package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDecideRecovery(t *testing.T) {
	cases := []struct {
		name   string
		total  int
		passed int
		want   recoveryDecision
	}{
		{"no paused feeds", 0, 0, recoverNone},
		{"all probes failed", 10, 0, recoverNone},
		{"majority passed resumes all", 10, 5, recoverAll},
		{"all passed resumes all", 4, 4, recoverAll},
		{"minority passed resumes partial", 10, 3, recoverPartial},
		{"single feed passed", 1, 1, recoverAll},
		{"single feed failed", 1, 0, recoverNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := decideRecovery(c.total, c.passed); got != c.want {
				t.Errorf("decideRecovery(%d, %d) = %v, want %v", c.total, c.passed, got, c.want)
			}
		})
	}
}

func TestProbeURL(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer okSrv.Close()

	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failSrv.Close()

	svc := NewRSSFetcherService(RSSFetcherConfig{Timeout: 5}, nil)
	ctx := context.Background()

	if !svc.probeURL(ctx, okSrv.URL) {
		t.Errorf("probeURL(%s) = false, want true for 200 response", okSrv.URL)
	}
	if svc.probeURL(ctx, failSrv.URL) {
		t.Errorf("probeURL(%s) = true, want false for 503 response", failSrv.URL)
	}
	if svc.probeURL(ctx, "http://127.0.0.1:1/unreachable") {
		t.Error("probeURL(unreachable) = true, want false for connection error")
	}
}

// setupRecoveryTestDB 复用 repository 测试同一 Postgres 实例，独立 schema 隔离。
// SetMaxOpenConns(1) 保证 SET search_path 在整个连接生命周期内稳定生效。
func setupRecoveryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(enqueueTestDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Skipf("测试 Postgres 不可用，跳过: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	db.Exec("CREATE SCHEMA IF NOT EXISTS svc_rss_recovery_test")
	db.Exec("SET search_path TO svc_rss_recovery_test, public")
	if err := db.AutoMigrate(&model.RSSFeed{}); err != nil {
		t.Fatalf("auto migrate rss_feeds: %v", err)
	}
	db.Exec("TRUNCATE TABLE rss_feeds")
	return db
}

// TestProbePausedFeeds_ResumesReachableFeed 是「死代码接线」的端到端回归护栏：
// 当上游其实可达时，一轮探测必须把被熔断暂停的 feed 自动恢复（is_paused=false、
// consecutive_failures 归零、health_score 回半血）。若 runLoop 未接线 probePausedFeeds，
// 自动恢复整条链路失效（2026-07-15 起 4 个英文源停摆 12 天的根因即此）。
func TestProbePausedFeeds_ResumesReachableFeed(t *testing.T) {
	db := setupRecoveryTestDB(t)
	repo := repository.NewRSSRepository(db)

	// httptest 同时充当 RSSHub base 预检目标与 feed 抓取探测目标（均返回 200）。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// 造一个已熔断暂停、但上游其实可达的 feed（URL 为绝对地址，resolveFeedURL 原样返回）。
	pausedAt := time.Now().Add(-time.Hour)
	feed := &model.RSSFeed{
		Name:                "recovery-probe-target",
		URL:                 srv.URL,
		IsActive:            true,
		IsPaused:            true,
		PausedAt:            &pausedAt,
		ConsecutiveFailures: 6,
		HealthScore:         10,
	}
	if err := repo.Create(feed); err != nil {
		t.Fatalf("create paused feed: %v", err)
	}

	svc := NewRSSFetcherService(RSSFetcherConfig{
		Timeout:       5,
		RSSHubBaseURL: srv.URL, // 网络预检指向可达 server
	}, repo)

	svc.probePausedFeeds(context.Background())

	got, err := repo.GetByID(feed.ID)
	if err != nil {
		t.Fatalf("reload feed: %v", err)
	}
	if got.IsPaused {
		t.Error("feed 应被自动恢复 (is_paused=false)，实得 is_paused=true")
	}
	if got.ConsecutiveFailures != 0 {
		t.Errorf("恢复后 consecutive_failures 应归零，实得 %d", got.ConsecutiveFailures)
	}
	if got.HealthScore < RecoveryHealthScore {
		t.Errorf("恢复后 health_score 应 >= %d，实得 %d", RecoveryHealthScore, got.HealthScore)
	}
}

// TestProbePausedFeeds_SkipsWhenUpstreamDown 验证网络预检门控：当 RSSHub base
// 不可达（整体网络仍未恢复）时，本轮探测必须整体放弃，不得误恢复任何 feed。
func TestProbePausedFeeds_SkipsWhenUpstreamDown(t *testing.T) {
	db := setupRecoveryTestDB(t)
	repo := repository.NewRSSRepository(db)

	pausedAt := time.Now().Add(-time.Hour)
	feed := &model.RSSFeed{
		Name:                "recovery-probe-down",
		URL:                 "http://127.0.0.1:1/whatever",
		IsActive:            true,
		IsPaused:            true,
		PausedAt:            &pausedAt,
		ConsecutiveFailures: 6,
		HealthScore:         10,
	}
	if err := repo.Create(feed); err != nil {
		t.Fatalf("create paused feed: %v", err)
	}

	svc := NewRSSFetcherService(RSSFetcherConfig{
		Timeout:       2,
		RSSHubBaseURL: "http://127.0.0.1:1", // 预检不可达
	}, repo)

	svc.probePausedFeeds(context.Background())

	got, err := repo.GetByID(feed.ID)
	if err != nil {
		t.Fatalf("reload feed: %v", err)
	}
	if !got.IsPaused {
		t.Error("预检失败时 feed 不应被恢复，实得 is_paused=false")
	}
	if got.ConsecutiveFailures != 6 {
		t.Errorf("预检失败时 consecutive_failures 不应改变，期望 6 实得 %d", got.ConsecutiveFailures)
	}
}

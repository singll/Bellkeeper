package repository

import (
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/datatypes"
)

func TestCrawlDomainProfileRepository_FindOrCreate(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)

	p, err := repo.FindOrCreate("example.com", 30, 2)
	assertNoError(t, err, "FindOrCreate new")
	assertEqual(t, p.Domain, "example.com")
	assertEqual(t, p.DefaultDelaySeconds, 30)
	assertEqual(t, p.MaxConcurrency, 2)

	p2, err := repo.FindOrCreate("example.com", 60, 5)
	assertNoError(t, err, "FindOrCreate existing")
	assertEqual(t, p2.ID, p.ID)
	assertEqual(t, p2.DefaultDelaySeconds, 30)
}

func TestCrawlDomainProfileRepository_FindOrCreateDefaults(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)

	p, err := repo.FindOrCreate("new.com", 0, 0)
	assertNoError(t, err, "FindOrCreate with zero defaults")
	assertEqual(t, p.DefaultDelaySeconds, 60)
	assertEqual(t, p.MaxConcurrency, 1)
}

func TestCrawlDomainProfileRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)

	_, err := repo.FindOrCreate("a.com", 30, 1)
	assertNoError(t, err, "FindOrCreate a")
	_, err = repo.FindOrCreate("b.com", 60, 2)
	assertNoError(t, err, "FindOrCreate b")

	profiles, total, err := repo.List(1, 10)
	assertNoError(t, err, "List")
	assertEqual(t, total, int64(2))
	assertEqual(t, len(profiles), 2)
}

func TestCrawlDomainProfileRepository_RecordStart(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)

	_, err := repo.FindOrCreate("example.com", 30, 2)
	assertNoError(t, err, "FindOrCreate")

	nextAllowed := time.Now().Add(30 * time.Second)
	assertNoError(t, repo.RecordStart("example.com", nextAllowed), "RecordStart")

	var p model.CrawlDomainProfile
	assertNoError(t, db.Where("domain = ?", "example.com").First(&p).Error, "Find")
	assertEqual(t, p.LastStatus, "running")
	assertEqual(t, p.NextAllowedAt != nil, true)
}

func TestCrawlDomainProfileRepository_RecordOutcome(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)

	_, err := repo.FindOrCreate("example.com", 30, 2)
	assertNoError(t, err, "FindOrCreate")

	nextAllowed := time.Now().Add(60 * time.Second)
	assertNoError(t, repo.RecordOutcome("example.com", "success", "ok", &nextAllowed), "RecordOutcome")

	var p model.CrawlDomainProfile
	assertNoError(t, db.Where("domain = ?", "example.com").First(&p).Error, "Find")
	assertEqual(t, p.LastStatus, "success")
	assertEqual(t, p.Notes, "ok")
}

func TestCrawlDomainProfileRepository_RefreshRates(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)
	crawlRepo := NewCrawlJobRepository(db)

	_, err := repo.FindOrCreate("example.com", 30, 2)
	assertNoError(t, err, "FindOrCreate")

	assertNoError(t, crawlRepo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://example.com/1", Status: model.CrawlJobSuccess, SourceDomain: "example.com"}), "Create success")
	assertNoError(t, crawlRepo.Enqueue(&model.CrawlJob{SourceID: 1, URL: "https://example.com/2", Status: model.CrawlJobBlocked, SourceDomain: "example.com"}), "Create blocked")

	since := time.Now().Add(-1 * time.Hour)
	assertNoError(t, repo.RefreshRates("example.com", since), "RefreshRates")

	var p model.CrawlDomainProfile
	assertNoError(t, db.Where("domain = ?", "example.com").First(&p).Error, "Find")
	assertEqual(t, p.SuccessRate24h > 0, true)
	assertEqual(t, p.BlockRate24h > 0, true)
}

func TestCrawlDomainProfileRepository_CoolingExponential(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)

	base := 1 * time.Minute
	max := 1 * time.Hour

	// First failure: failure_count=1, next_allowed_at ~ now+base.
	assertNoError(t, repo.EnterCooling("example.com", base, max, true), "EnterCooling 1")
	cooling, err := repo.IsCooling("example.com")
	assertNoError(t, err, "IsCooling")
	assertEqual(t, cooling, true)

	p, err := repo.FindOrCreate("example.com", 0, 0)
	assertNoError(t, err, "FindOrCreate")
	assertEqual(t, p.FailureCount, 1)
	if p.NextAllowedAt == nil {
		t.Fatalf("expected next_allowed_at set, got nil")
	}
	assertWithin(t, *p.NextAllowedAt, time.Now().Add(base), 15*time.Second)

	// Second failure: failure_count=2, duration doubles to base*2.
	assertNoError(t, repo.EnterCooling("example.com", base, max, true), "EnterCooling 2")
	p, _ = repo.FindOrCreate("example.com", 0, 0)
	assertEqual(t, p.FailureCount, 2)
	assertWithin(t, *p.NextAllowedAt, time.Now().Add(2*base), 15*time.Second)

	// Success clears cooling: failure_count back to 0, next_allowed_at NULL.
	assertNoError(t, repo.ClearCooling("example.com"), "ClearCooling")
	cooling, _ = repo.IsCooling("example.com")
	assertEqual(t, cooling, false)
	p, _ = repo.FindOrCreate("example.com", 0, 0)
	assertEqual(t, p.FailureCount, 0)
	assertEqual(t, p.NextAllowedAt == nil, true)
}

func TestCrawlDomainProfileRepository_CoolingCap(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)

	base := 1 * time.Minute
	max := 5 * time.Minute

	// After many failures the exponential duration must be capped at max.
	for i := 0; i < 10; i++ {
		assertNoError(t, repo.EnterCooling("capped.com", base, max, true), "EnterCooling")
	}
	p, _ := repo.FindOrCreate("capped.com", 0, 0)
	assertEqual(t, p.FailureCount, 10)
	assertWithin(t, *p.NextAllowedAt, time.Now().Add(max), 15*time.Second)
}

func TestCrawlDomainProfileRepository_UpdateOverrides(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)

	_, err := repo.FindOrCreate("ov.com", 0, 0)
	assertNoError(t, err, "FindOrCreate")

	assertNoError(t, repo.UpdateOverrides("ov.com", datatypes.JSON(`{"user_agent":"Mozilla/5.0"}`), "needs UA"), "UpdateOverrides")

	p, _ := repo.FindOrCreate("ov.com", 0, 0)
	assertJSONEq(t, string(p.RequestOverrides), `{"user_agent":"Mozilla/5.0"}`)
	assertEqual(t, p.AnalysisResult, "needs UA")
}

func assertWithin(t *testing.T, got, want time.Time, tolerance time.Duration) {
	t.Helper()
	diff := got.Sub(want)
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		t.Fatalf("time %v not within %s of %v (diff %s)", got, tolerance, want, diff)
	}
}

// 修复①：内容级/抓取器级失败（countsTowardHealth=false）只做调度冷却，
// 不得累加 ConsecutiveFailures / 扣 HealthScore（否则误暂停健康大站）。
func TestEnterCooling_ContentFailureDoesNotCountHealth(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)
	base, max := time.Minute, time.Hour

	for i := 0; i < 10; i++ {
		assertNoError(t, repo.EnterCooling("content.com", base, max, false), "EnterCooling content")
	}
	p, _ := repo.FindOrCreate("content.com", 0, 0)
	assertEqual(t, p.FailureCount, 10)       // 调度冷却仍生效
	assertEqual(t, p.ConsecutiveFailures, 0) // 健康度不受污染
	assertEqual(t, p.HealthScore, 100)       // 未扣分
}

// 域名级失败（countsTowardHealth=true）正常累加连续失败并扣健康度。
func TestEnterCooling_DomainFailureCountsHealth(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)
	base, max := time.Minute, time.Hour

	for i := 0; i < 5; i++ {
		assertNoError(t, repo.EnterCooling("down.com", base, max, true), "EnterCooling domain")
	}
	p, _ := repo.FindOrCreate("down.com", 0, 0)
	assertEqual(t, p.ConsecutiveFailures, 5)
	assertEqual(t, p.HealthScore, 50) // 100 - 5*10
}

// 修复：EvaluateDomainHealth 边缘触发——首次越阈值返回 paused，紧接稳态返回 none。
func TestEvaluateDomainHealth_EdgeTriggeredPause(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)
	base, max := time.Minute, time.Hour

	for i := 0; i < 5; i++ {
		assertNoError(t, repo.EnterCooling("edge.com", base, max, true), "EnterCooling")
	}
	res, err := repo.EvaluateDomainHealth("edge.com", 5, 30)
	assertNoError(t, err, "Evaluate 1")
	assertEqual(t, res.Action, "paused")
	assertEqual(t, res.HealthScore, 50)

	// 稳态：已暂停，不重复返回 paused，也不再从此函数恢复（恢复交给 half-open）。
	res2, err := repo.EvaluateDomainHealth("edge.com", 5, 30)
	assertNoError(t, err, "Evaluate 2")
	assertEqual(t, res2.Action, "none")
}

// 修复③：half-open 时间驱动恢复——暂停超冷静期的域名被解除、清零连续失败、抬健康度。
func TestHalfOpenRecoverDomains(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)
	base, max := time.Minute, time.Hour

	// 造两个暂停域名：old.com 暂停已久应恢复；fresh.com 刚暂停不应恢复。
	for i := 0; i < 5; i++ {
		assertNoError(t, repo.EnterCooling("old.com", base, max, true), "cooling old")
		assertNoError(t, repo.EnterCooling("fresh.com", base, max, true), "cooling fresh")
	}
	_, _ = repo.EvaluateDomainHealth("old.com", 5, 30)
	_, _ = repo.EvaluateDomainHealth("fresh.com", 5, 30)
	// 把 old.com 的 paused_at 拨早 1 小时。
	old := time.Now().Add(-time.Hour)
	assertNoError(t, db.Model(&model.CrawlDomainProfile{}).
		Where("domain = ?", "old.com").Update("paused_at", old).Error, "backdate")

	cutoff := time.Now().Add(-30 * time.Minute)
	recovered, err := repo.HalfOpenRecoverDomains(cutoff)
	assertNoError(t, err, "HalfOpenRecover")
	assertEqual(t, len(recovered), 1)
	assertEqual(t, recovered[0], "old.com")

	po, _ := repo.FindOrCreate("old.com", 0, 0)
	assertEqual(t, po.IsPaused, false)
	assertEqual(t, po.ConsecutiveFailures, 0)
	if po.HealthScore < 50 {
		t.Fatalf("expected health_score raised to >=50, got %d", po.HealthScore)
	}
	pf, _ := repo.FindOrCreate("fresh.com", 0, 0)
	assertEqual(t, pf.IsPaused, true) // 未过冷静期，保持暂停
}

func TestCrawlDomainProfileRepository_FindDomainsWithWaitForOverride(t *testing.T) {
	db := setupTestDB(t)
	repo := NewCrawlDomainProfileRepository(db)

	for _, d := range []string{"waitfor.com", "plainua.com", "noovr.com"} {
		_, err := repo.FindOrCreate(d, 30, 1)
		assertNoError(t, err, "FindOrCreate "+d)
	}
	// 带 firecrawl_wait_for → 应被选中
	assertNoError(t, repo.UpdateOverrides("waitfor.com",
		datatypes.JSON(`{"strategy":"firecrawl","firecrawl_wait_for":3000}`), "test"), "update waitfor")
	// 只有 UA、无 waitFor → 不应被选中
	assertNoError(t, repo.UpdateOverrides("plainua.com",
		datatypes.JSON(`{"user_agent":"Mozilla"}`), "test"), "update plainua")
	// noovr.com 无 override（request_overrides 为 NULL）→ 不应被选中

	domains, err := repo.FindDomainsWithWaitForOverride(10)
	assertNoError(t, err, "FindDomainsWithWaitForOverride")
	if len(domains) != 1 || domains[0] != "waitfor.com" {
		t.Fatalf("expected [waitfor.com], got %v", domains)
	}
}

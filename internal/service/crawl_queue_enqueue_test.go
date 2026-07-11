package service

import (
	"testing"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 与 repository 测试同一实例，独立 schema 隔离，避免跨包串数据。
const enqueueTestDSN = "host=localhost port=15432 user=bellkeeper password=testpass dbname=bellkeeper_test sslmode=disable"

func setupEnqueueTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(enqueueTestDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Skipf("测试 Postgres 不可用，跳过: %v", err)
	}
	db.Exec("CREATE SCHEMA IF NOT EXISTS svc_enqueue_test")
	db.Exec("SET search_path TO svc_enqueue_test, public")
	if err := db.AutoMigrate(&model.CrawlJob{}); err != nil {
		t.Fatalf("auto migrate crawl_jobs: %v", err)
	}
	db.Exec("TRUNCATE TABLE crawl_jobs")
	return db
}

// Enqueue 命中 recrawl-cooldown 时应显式跳过（返回 id=0, err=nil），不再重复入队。
func TestCrawlQueueService_Enqueue_RecrawlCooldownDedup(t *testing.T) {
	db := setupEnqueueTestDB(t)
	repo := repository.NewCrawlJobRepository(db)

	cfg := config.CrawlQueueConfig{
		MaxRetries:           4,
		DomainPendingCap:     0,   // 不测配额，只测去重
		RecrawlCooldownHours: 168, // 7 天冷却
	}
	// activityLog=nil 时 logActivity 是 nil 安全的；其余依赖此路径用不到。
	svc := NewCrawlQueueService(cfg, repo, nil, nil, nil, nil, nil)

	url := "https://openai.com/index/nvidia"

	// 首次入队成功
	id1, err := svc.Enqueue(1, url, "t", "auto", nil)
	if err != nil {
		t.Fatalf("首次 Enqueue 失败: %v", err)
	}
	if id1 == 0 {
		t.Fatalf("首次 Enqueue 应返回非零 id")
	}

	// 把该 job 标为 success，模拟已成功抓取
	if err := db.Model(&model.CrawlJob{}).Where("id = ?", id1).
		Update("status", string(model.CrawlJobSuccess)).Error; err != nil {
		t.Fatalf("标记 success 失败: %v", err)
	}

	// 再次入队同一 URL → 命中冷却，应跳过（id=0, err=nil）
	id2, err := svc.Enqueue(1, url, "t", "auto", nil)
	if err != nil {
		t.Fatalf("重复 Enqueue 不应报错: %v", err)
	}
	if id2 != 0 {
		t.Fatalf("重复 Enqueue 应被去重跳过（id=0），实得 id=%d", id2)
	}

	// 库中该 URL 仍只有 1 条记录（没有第二次入队）
	var count int64
	db.Model(&model.CrawlJob{}).Where("url = ?", url).Count(&count)
	if count != 1 {
		t.Fatalf("去重后应只有 1 条记录，实得 %d", count)
	}

	// 冷却关闭时（0）不去重：同 URL 可再次入队
	svc.cfg.RecrawlCooldownHours = 0
	id3, err := svc.Enqueue(1, url, "t", "auto", nil)
	if err != nil {
		t.Fatalf("关闭冷却后 Enqueue 失败: %v", err)
	}
	if id3 == 0 {
		t.Fatalf("关闭冷却后应正常入队（非零 id）")
	}
}

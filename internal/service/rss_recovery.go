package service

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/model"
)

// Recovery probe constants
const (
	// RecoveryMajorityRatio 探测通过率达到该比例时恢复全部暂停 feed，
	// 低于该比例（但 > 0）时仅恢复探测通过的 feed
	RecoveryMajorityRatio = 0.5

	// RecoveryHealthScore 恢复时重置的健康分（半血：再连续失败数次仍会重新熔断）
	RecoveryHealthScore = 50

	// RecoveryProbeConcurrency 探测请求并发上限
	RecoveryProbeConcurrency = 3

	// DefaultProbeIntervalMinutes 默认探测间隔（分钟），刻意取较长间隔避免频繁打扰上游
	DefaultProbeIntervalMinutes = 30
)

// recoveryDecision 表示一轮探测后的恢复决策
type recoveryDecision int

const (
	recoverNone    recoveryDecision = iota // 全部探测失败：不恢复，等下一轮
	recoverPartial                         // 通过率 < 多数比例：仅恢复探测通过的 feed
	recoverAll                             // 通过率 >= 多数比例：恢复全部暂停 feed
)

// decideRecovery 根据探测总数与通过数决定恢复策略
func decideRecovery(total, passed int) recoveryDecision {
	if total <= 0 || passed <= 0 {
		return recoverNone
	}
	if float64(passed)/float64(total) >= RecoveryMajorityRatio {
		return recoverAll
	}
	return recoverPartial
}

// probePausedFeeds 对处于暂停状态的 feed 执行一轮恢复探测：
//  1. 网络预检：探测 RSSHub 基础地址是否可达（未配置则跳过）
//  2. 抽样测试：对每个暂停 feed 的实际抓取 URL 发探测请求
//  3. 按通过率分级恢复（全部 / 部分 / 不恢复）
func (s *RSSFetcherService) probePausedFeeds(ctx context.Context) {
	paused, err := s.rssRepo.GetPaused()
	if err != nil {
		log.Printf("[RSSRecovery] failed to get paused feeds: %v", err)
		return
	}
	if len(paused) == 0 {
		return
	}

	log.Printf("[RSSRecovery] probing %d paused feeds", len(paused))

	// 网络预检：RSSHub 基础地址不可达视为网络仍未恢复，本轮直接放弃
	if s.cfg.RSSHubBaseURL != "" && !s.probeURL(ctx, s.cfg.RSSHubBaseURL) {
		log.Printf("[RSSRecovery] network precheck failed (rsshub base unreachable), skip this round")
		s.logActivity("rss_fetch", "recovery", "precheck_failed",
			fmt.Sprintf("Recovery probe skipped: RSSHub base %s unreachable, %d feeds remain paused", s.cfg.RSSHubBaseURL, len(paused)),
			0, 0)
		return
	}

	// 并发抽样探测每个暂停 feed 的实际抓取地址
	passedSet := make(map[uint]bool, len(paused))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, RecoveryProbeConcurrency)
	for _, feed := range paused {
		wg.Add(1)
		go func(f model.RSSFeed) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if s.probeURL(ctx, s.resolveFeedURL(&f)) {
				mu.Lock()
				passedSet[f.ID] = true
				mu.Unlock()
			}
		}(feed)
	}
	wg.Wait()

	decision := decideRecovery(len(paused), len(passedSet))
	switch decision {
	case recoverNone:
		log.Printf("[RSSRecovery] 0/%d probes passed, all feeds remain paused until next round", len(paused))
		s.logActivity("rss_fetch", "recovery", "still_down",
			fmt.Sprintf("Recovery probe: 0/%d passed, retry next round", len(paused)),
			0, 0)
		return
	case recoverAll:
		log.Printf("[RSSRecovery] %d/%d probes passed (>= majority), resuming ALL paused feeds", len(passedSet), len(paused))
	case recoverPartial:
		log.Printf("[RSSRecovery] %d/%d probes passed (< majority), resuming passed feeds only", len(passedSet), len(paused))
	}

	resumed := 0
	for i := range paused {
		feed := &paused[i]
		if decision == recoverPartial && !passedSet[feed.ID] {
			continue
		}
		if err := s.resumeFeedAfterProbe(feed); err != nil {
			log.Printf("[RSSRecovery] failed to resume feed %d (%s): %v", feed.ID, feed.Name, err)
			continue
		}
		resumed++
	}

	s.logActivity("rss_fetch", "recovery", "auto_resume",
		fmt.Sprintf("Recovery probe: %d/%d passed, resumed %d feeds", len(passedSet), len(paused), resumed),
		0, 0)
}

// probeURL 对目标地址发一次轻量 GET 探测，HTTP 2xx/3xx 视为通过
func (s *RSSFetcherService) probeURL(ctx context.Context, url string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, time.Duration(s.cfg.Timeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Printf("[RSSRecovery] failed to close probe response body: %v", cerr)
		}
	}()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

// resumeFeedAfterProbe 将暂停 feed 恢复为可调度状态：
// 清零失败计数、健康分恢复到半血、清空 last_fetched_at 让调度器立即重新抓取
func (s *RSSFetcherService) resumeFeedAfterProbe(feed *model.RSSFeed) error {
	feed.IsPaused = false
	feed.PausedAt = nil
	feed.ConsecutiveFailures = 0
	if feed.HealthScore < RecoveryHealthScore {
		feed.HealthScore = RecoveryHealthScore
	}
	feed.LastFetchedAt = nil
	if err := s.rssRepo.Update(feed); err != nil {
		return fmt.Errorf("failed to update feed %d: %w", feed.ID, err)
	}
	log.Printf("[RSSRecovery] auto-resumed feed %d (%s), health_score=%d", feed.ID, feed.Name, feed.HealthScore)
	return nil
}

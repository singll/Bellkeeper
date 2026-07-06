import { Component, createResource, For, Show, createSignal } from 'solid-js'
import { crawlQueueApi } from '@/api'
import { useToast } from '@/components/Toast'
import { formatDateShort } from '@/utils/format'

// CrawlQueue 爬取队列可视化页（1.0 §4 [fe]）。
// 后端 API 完整（/api/crawl/queue/*），前端此前缺失，本页补齐：
// 统计概览 + 域名健康度 + 任务列表（含 crawled 中间态）。
const CrawlQueue: Component = () => {
  const toast = useToast()
  const [statusFilter, setStatusFilter] = createSignal('')

  const [stats] = createResource(() => crawlQueueApi.getStats())
  const [domains] = createResource(() => crawlQueueApi.listDomains())
  const [jobs, { refetch }] = createResource(() =>
    crawlQueueApi.listJobs({ page: 1, page_size: 50, status: statusFilter() || undefined }),
  )

  const handleRetry = async (id: number) => {
    try {
      await crawlQueueApi.retryJob(id)
      toast.success('已重新入队')
      refetch()
    } catch (err) {
      toast.error('重试失败: ' + (err as Error).message)
    }
  }

  const statCards = () => {
    const s = stats()?.data
    if (!s) return []
    return [
      { label: '待处理', value: s.pending ?? 0, color: 'text-yellow-400' },
      { label: '运行中', value: s.running ?? 0, color: 'text-blue-400' },
      { label: '待提取', value: s.crawled ?? 0, color: 'text-purple-400' },
      { label: '成功', value: s.success ?? 0, color: 'text-green-400' },
      { label: '失败', value: s.failed ?? 0, color: 'text-red-400' },
      { label: '已跳过', value: s.skipped ?? 0, color: 'text-gray-400' },
    ]
  }

  const statusOptions = ['', 'pending', 'running', 'crawled', 'success', 'failed', 'skipped', 'blocked', 'dead']

  return (
    <div class="animate-fade-in">
      <div class="mb-6">
        <h1 class="text-2xl font-bold text-white">爬取队列</h1>
        <p class="text-sm text-dark-400 mt-1">任务调度 / 域名健康度 / 状态机</p>
      </div>

      {/* Stats */}
      <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4 mb-6">
        <For each={statCards()}>
          {(card) => (
            <div class="card p-4">
              <div class="text-xs text-dark-400 mb-1">{card.label}</div>
              <div class={`text-2xl font-bold ${card.color}`}>{card.value}</div>
            </div>
          )}
        </For>
      </div>

      {/* Domain health */}
      <Show when={domains()?.data?.length}>
        <div class="mb-6">
          <h2 class="text-lg font-semibold text-white mb-3">域名健康度</h2>
          <div class="card overflow-x-auto">
            <table class="min-w-full divide-y divide-dark-700">
              <thead class="bg-dark-800">
                <tr>
                  <th class="th">域名</th>
                  <th class="th">健康分</th>
                  <th class="th">连续失败</th>
                  <th class="th">状态</th>
                  <th class="th">24h 成功率</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-dark-700">
                <For each={domains()?.data ?? []}>
                  {(d) => (
                    <tr class="hover:bg-dark-800/50">
                      <td class="td font-medium text-white">{d.domain}</td>
                      <td class="td">
                        <span class={`font-bold ${d.health_score >= 60 ? 'text-green-400' : d.health_score >= 30 ? 'text-yellow-400' : 'text-red-400'}`}>
                          {d.health_score ?? 100}
                        </span>
                      </td>
                      <td class="td text-dark-300">{d.consecutive_failures ?? 0}</td>
                      <td class="td">
                        <Show when={d.is_paused}>
                          <span class="badge badge-red">已暂停</span>
                        </Show>
                        <Show when={!d.is_paused}>
                          <span class="badge badge-green">活跃</span>
                        </Show>
                      </td>
                      <td class="td text-dark-300">{((d.success_rate_24h ?? 0) * 100).toFixed(1)}%</td>
                    </tr>
                  )}
                </For>
              </tbody>
            </table>
          </div>
        </div>
      </Show>

      {/* Jobs */}
      <div>
        <div class="flex items-center justify-between mb-3">
          <h2 class="text-lg font-semibold text-white">任务列表</h2>
          <select
            class="input-sm"
            value={statusFilter()}
            onChange={(e) => { setStatusFilter(e.currentTarget.value); refetch() }}
          >
            <For each={statusOptions}>
              {(s) => <option value={s}>{s || '全部状态'}</option>}
            </For>
          </select>
        </div>
        <div class="card overflow-x-auto">
          <table class="min-w-full divide-y divide-dark-700">
            <thead class="bg-dark-800">
              <tr>
                <th class="th">ID</th>
                <th class="th">URL</th>
                <th class="th">域名</th>
                <th class="th">状态</th>
                <th class="th">重试</th>
                <th class="th">时间</th>
                <th class="th">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-dark-700">
              <For each={jobs()?.data?.items ?? []}>
                {(job) => (
                  <tr class="hover:bg-dark-800/50">
                    <td class="td text-dark-400">{job.id}</td>
                    <td class="td text-dark-300 max-w-xs truncate" title={job.url}>{job.url}</td>
                    <td class="td text-dark-400 text-xs">{job.source_domain}</td>
                    <td class="td">
                      <span class={`badge ${
                        job.status === 'success' ? 'badge-green'
                        : job.status === 'failed' || job.status === 'dead' ? 'badge-red'
                        : job.status === 'crawled' ? 'badge-blue'
                        : 'badge-gray'
                      }`}>{job.status}</span>
                    </td>
                    <td class="td text-dark-400">{job.retry_count}/{job.max_retries}</td>
                    <td class="td text-dark-400 text-xs">{formatDateShort(job.created_at)}</td>
                    <td class="td">
                      <Show when={job.status === 'failed' || job.status === 'skipped' || job.status === 'dead'}>
                        <button class="btn-sm btn-violet" onClick={() => handleRetry(job.id)}>重试</button>
                      </Show>
                    </td>
                  </tr>
                )}
              </For>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}

export default CrawlQueue

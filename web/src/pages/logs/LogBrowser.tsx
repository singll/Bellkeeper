import { Component, For, Show, createResource, createSignal, onCleanup } from 'solid-js'
import { logCenterApi } from '@/api'
import {
  formatDateTime,
  formatDuration,
  levelColor,
  levelLabel,
  statusColor,
  statusLabel,
  moduleLabel,
} from './shared'
import type { LogEntry } from './shared'

const LogBrowser: Component = () => {
  const [moduleFilter, setModuleFilter] = createSignal('')
  const [levelFilter, setLevelFilter] = createSignal('')
  const [statusFilter, setStatusFilter] = createSignal('')
  const [keyword, setKeyword] = createSignal('')
  const [page, setPage] = createSignal(1)
  const [autoRefresh, setAutoRefresh] = createSignal(false)
  const [expandedId, setExpandedId] = createSignal<number | null>(null)
  let refreshTimer: ReturnType<typeof setInterval> | undefined

  const [logsData, { refetch: refetchLogs }] = createResource(
    () => ({
      module: moduleFilter(),
      level: levelFilter(),
      status: statusFilter(),
      keyword: keyword(),
      page: page(),
      limit: 50,
    }),
    (params) => {
      const qs = Object.entries(params)
        .filter(([, v]) => v !== '' && v !== undefined)
        .map(([k, v]) => `${k}=${encodeURIComponent(String(v))}`)
        .join('&')
      return logCenterApi.listEntries(qs ? Object.fromEntries(new URLSearchParams(qs).entries()) : {})
    }
  )

  const startAutoRefresh = () => {
    stopAutoRefresh()
    refreshTimer = setInterval(() => refetchLogs(), 5000)
  }
  const stopAutoRefresh = () => {
    if (refreshTimer) { clearInterval(refreshTimer); refreshTimer = undefined }
  }
  onCleanup(stopAutoRefresh)

  const toggleAutoRefresh = () => {
    const next = !autoRefresh()
    setAutoRefresh(next)
    if (next) startAutoRefresh(); else stopAutoRefresh()
  }

  const totalPages = () => {
    const data = logsData()?.data
    if (!data) return 1
    return Math.max(1, Math.ceil(data.total / data.limit))
  }

  const applyFilter = () => setPage(1)

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">日志浏览器</h1>
          <p class="text-sm text-dark-400 mt-1">统一查看和搜索系统日志</p>
        </div>
      </div>

      <div class="space-y-4">
        {/* Filters */}
        <div class="flex flex-wrap gap-3 items-center">
          <select
            class="input w-40"
            value={moduleFilter()}
            onChange={(e) => { setModuleFilter(e.currentTarget.value); applyFilter() }}
          >
            <option value="">全部模块</option>
            <option value="classify">文章分类</option>
            <option value="llm_proxy">LLM Proxy</option>
            <option value="rss_fetch">RSS采集</option>
            <option value="file_ingest">文件入库</option>
            <option value="crawler">爬虫</option>
            <option value="matrix_notify">Matrix通知</option>
            <option value="bellkeeper-core">核心系统</option>
          </select>

          <div class="flex gap-1">
            <For each={['debug', 'info', 'warn', 'error']}>
              {(lvl) => (
                <button
                  class={`px-2.5 py-1 rounded-md text-xs font-medium border transition-colors ${
                    levelFilter() === lvl ? levelColor(lvl) : 'text-dark-500 border-dark-600 hover:border-dark-400'
                  }`}
                  onClick={() => { setLevelFilter(levelFilter() === lvl ? '' : lvl); applyFilter() }}
                >
                  {levelLabel(lvl)}
                </button>
              )}
            </For>
          </div>

          <select
            class="input w-32"
            value={statusFilter()}
            onChange={(e) => { setStatusFilter(e.currentTarget.value); applyFilter() }}
          >
            <option value="">全部状态</option>
            <option value="success">成功</option>
            <option value="failed">失败</option>
            <option value="skipped">跳过</option>
          </select>

          <input
            type="text"
            class="input w-48"
            placeholder="关键词搜索..."
            value={keyword()}
            onInput={(e) => setKeyword(e.currentTarget.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') applyFilter() }}
          />

          <button class="btn btn-secondary btn-sm" onClick={() => refetchLogs()}>
            刷新
          </button>

          <button
            class={`btn btn-sm ${autoRefresh() ? 'btn-primary' : 'btn-secondary'}`}
            onClick={toggleAutoRefresh}
          >
            {autoRefresh() ? '自动: 开' : '自动: 关'}
          </button>

          <Show when={logsData()?.data}>
            <span class="text-xs text-dark-500 ml-auto">
              共 {logsData()!.data!.total} 条
            </span>
          </Show>
        </div>

        {/* Log list */}
        <Show when={!logsData.loading} fallback={<div class="text-center text-dark-400 py-8">加载中...</div>}>
          <div class="space-y-2">
            <For each={logsData()?.data?.items || []} fallback={<div class="text-center text-dark-500 py-8">暂无日志</div>}>
              {(entry: LogEntry) => (
                <div class="card p-3 hover:bg-dark-700/50 transition-colors">
                  <div
                    class="flex items-start gap-3 cursor-pointer"
                    onClick={() => setExpandedId(expandedId() === entry.id ? null : entry.id)}
                  >
                    {/* Level badge */}
                    <div class="flex-shrink-0 mt-0.5">
                      <span class={`text-xs font-mono px-1.5 py-0.5 rounded border ${levelColor(entry.level)}`}>
                        {levelLabel(entry.level)}
                      </span>
                    </div>

                    <div class="flex-1 min-w-0">
                      <div class="flex items-center gap-2">
                        <span class="text-xs font-medium text-dark-300">{moduleLabel(entry.module)}</span>
                        <span class={`text-xs px-1.5 py-0.5 rounded border ${statusColor(entry.status)}`}>
                          {statusLabel(entry.status)}
                        </span>
                        <span class="text-xs text-dark-500">{entry.action}</span>
                        <Show when={entry.duration_ms}>
                          <span class="text-xs text-dark-500">{formatDuration(entry.duration_ms)}</span>
                        </Show>
                        <span class="text-xs text-dark-600 ml-auto">{formatDateTime(entry.created_at)}</span>
                      </div>
                      <p class="text-sm text-dark-200 break-words mt-1">{entry.summary}</p>
                      <Show when={entry.trace_id}>
                        <span class="text-xs text-dark-600 font-mono">trace: {entry.trace_id}</span>
                      </Show>
                    </div>
                  </div>

                  {/* Expanded detail */}
                  <Show when={expandedId() === entry.id && entry.detail}>
                    <div class="mt-3 bg-dark-900 rounded-lg p-3 max-h-80 overflow-y-auto">
                      <pre class="text-xs text-dark-300 font-mono whitespace-pre-wrap">{JSON.stringify(entry.detail, null, 2)}</pre>
                    </div>
                  </Show>
                </div>
              )}
            </For>
          </div>

          {/* Pagination */}
          <Show when={totalPages() > 1}>
            <div class="flex justify-center items-center gap-2 mt-4">
              <button class="btn btn-secondary btn-sm" disabled={page() <= 1} onClick={() => setPage(p => Math.max(1, p - 1))}>
                上一页
              </button>
              <span class="text-sm text-dark-400">{page()} / {totalPages()}</span>
              <button class="btn btn-secondary btn-sm" disabled={page() >= totalPages()} onClick={() => setPage(p => p + 1)}>
                下一页
              </button>
            </div>
          </Show>
        </Show>
      </div>
    </div>
  )
}

export default LogBrowser

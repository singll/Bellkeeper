import { Component, createSignal, createResource, For, Show, onCleanup } from 'solid-js'
import { matrixApi } from '@/api'
import type { MatrixCommandLog } from '@/types'

const MatrixCommandLogs: Component = () => {
  const [page, setPage] = createSignal(1)
  const [perPage] = createSignal(20)
  const [commandFilter, setCommandFilter] = createSignal('')
  const [statusFilter, setStatusFilter] = createSignal('')
  const [refreshKey, setRefreshKey] = createSignal(0)

  // Auto-refresh every 10 seconds
  const [autoRefresh, setAutoRefresh] = createSignal(true)
  const interval = setInterval(() => {
    if (autoRefresh()) {
      setRefreshKey(k => k + 1)
    }
  }, 10000)
  onCleanup(() => clearInterval(interval))

  const [logs, { refetch }] = createResource(
    () => ({ page: page(), perPage: perPage(), command: commandFilter(), status: statusFilter(), key: refreshKey() }),
    (params) => matrixApi.listCommandLogs({ page: params.page, per_page: params.perPage, command: params.command || undefined, status: params.status || undefined })
  )

  const totalPages = () => Math.ceil((logs()?.total || 0) / perPage())

  const formatTime = (time: string) => {
    const d = new Date(time)
    return d.toLocaleString('zh-CN', {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  }

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'success':
        return <span class="badge badge-green">成功</span>
      case 'failed':
        return <span class="badge badge-red">失败</span>
      case 'pending':
        return <span class="badge badge-yellow">处理中</span>
      default:
        return <span class="badge badge-gray">{status}</span>
    }
  }

  const formatDuration = (ms?: number) => {
    if (ms === undefined) return '-'
    if (ms < 1000) return `${ms}ms`
    return `${(ms / 1000).toFixed(1)}s`
  }

  const handleFilterChange = () => {
    setPage(1)
    setRefreshKey(k => k + 1)
  }

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">命令日志</h1>
          <p class="text-sm text-dark-400 mt-1">查看 Matrix 命令执行记录</p>
        </div>
        <div class="flex items-center gap-3">
          <label class="flex items-center gap-2 text-sm text-dark-400 cursor-pointer">
            <input
              type="checkbox"
              checked={autoRefresh()}
              onChange={(e) => setAutoRefresh(e.currentTarget.checked)}
              class="w-4 h-4 rounded border-dark-600 bg-dark-800 text-primary focus:ring-primary"
            />
            10秒自动刷新
          </label>
          <button
            onClick={() => setRefreshKey(k => k + 1)}
            class="btn btn-secondary btn-sm"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
            刷新
          </button>
        </div>
      </div>

      {/* Filters */}
      <div class="card p-4 mb-6">
        <div class="flex flex-wrap gap-4">
          <div class="flex-1 min-w-[150px]">
            <label class="block text-sm text-dark-400 mb-1">命令</label>
            <input
              type="text"
              value={commandFilter()}
              onInput={(e) => { setCommandFilter(e.currentTarget.value); handleFilterChange() }}
              placeholder="筛选命令名"
              class="input input-sm"
            />
          </div>
          <div class="flex-1 min-w-[150px]">
            <label class="block text-sm text-dark-400 mb-1">状态</label>
            <select
              value={statusFilter()}
              onChange={(e) => { setStatusFilter(e.currentTarget.value); handleFilterChange() }}
              class="input input-sm"
            >
              <option value="">全部</option>
              <option value="success">成功</option>
              <option value="failed">失败</option>
              <option value="pending">处理中</option>
            </select>
          </div>
        </div>
      </div>

      {/* Table */}
      <div class="card overflow-hidden p-0">
        <div class="overflow-x-auto">
          <table class="table">
            <thead>
              <tr>
                <th>时间</th>
                <th>命令</th>
                <th>用户</th>
                <th>房间</th>
                <th>状态</th>
                <th>耗时</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              <Show
                when={!logs.loading && (logs()?.data?.length ?? 0) > 0}
                fallback={
                  <tr>
                    <td colspan="7" class="text-center py-12">
                      <Show when={logs.loading}>
                        <div class="loading-spinner mx-auto" />
                        <p class="mt-3 text-dark-400">加载中...</p>
                      </Show>
                      <Show when={!logs.loading}>
                        <div class="empty-state">
                          <svg class="empty-state-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                          </svg>
                          <p class="empty-state-title">暂无命令日志</p>
                          <p class="empty-state-description">命令执行后将在此显示记录</p>
                        </div>
                      </Show>
                    </td>
                  </tr>
                }
              >
                <For each={logs()?.data ?? []}>
                  {(log: MatrixCommandLog) => (
                    <tr>
                      <td>
                        <span class="text-dark-400 text-sm">{formatTime(log.created_at)}</span>
                      </td>
                      <td>
                        <span class="font-mono text-white">{log.command_name}</span>
                      </td>
                      <td>
                        <span class="text-dark-300">{log.sender}</span>
                      </td>
                      <td>
                        <span class="font-mono text-sm text-dark-400 truncate max-w-[150px] block" title={log.room_id}>
                          {log.room_id}
                        </span>
                      </td>
                      <td>{getStatusBadge(log.execution_status || 'success')}</td>
                      <td>
                        <span class="text-dark-400">{formatDuration(log.execution_time_ms)}</span>
                      </td>
                      <td>
                        <Show when={log.error_message}>
                          <span class="text-red-400 text-sm" title={log.error_message}>查看错误</span>
                        </Show>
                      </td>
                    </tr>
                  )}
                </For>
              </Show>
            </tbody>
          </table>
        </div>

        {/* Pagination */}
        <Show when={(logs()?.total ?? 0) > 0}>
          <div class="flex items-center justify-between p-4 border-t border-dark-700">
            <span class="text-sm text-dark-400">
              共 {logs()?.total} 条，第 {page()} / {totalPages()} 页
            </span>
            <div class="flex gap-2">
              <button
                class="btn btn-secondary btn-sm"
                disabled={page() <= 1}
                onClick={() => setPage(p => p - 1)}
              >
                上一页
              </button>
              <button
                class="btn btn-secondary btn-sm"
                disabled={page() >= totalPages()}
                onClick={() => setPage(p => p + 1)}
              >
                下一页
              </button>
            </div>
          </div>
        </Show>
      </div>
    </div>
  )
}

export default MatrixCommandLogs

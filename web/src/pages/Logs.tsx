import { Component, For, Show, createResource, createSignal, onCleanup } from 'solid-js'
import { logsApi, ragflowApi } from '@/api'
import type { ActivityLog, ParseTask } from '@/types'

type TabKey = 'logs' | 'parse-tasks'

const formatDateTime = (value?: string) => {
  if (!value) return '--'
  return new Date(value).toLocaleString('zh-CN')
}

const formatDuration = (ms?: number) => {
  if (!ms) return '--'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

const statusBadge = (status: string) => {
  switch (status) {
    case 'success': return 'badge-success'
    case 'error': return 'badge-danger'
    case 'info': return 'badge-primary'
    default: return 'badge-gray'
  }
}

const statusLabel = (status: string) => {
  switch (status) {
    case 'success': return '成功'
    case 'error': return '失败'
    case 'info': return '信息'
    default: return status
  }
}

const moduleLabel = (module: string) => {
  switch (module) {
    case 'ragflow_upload': return '文件上传'
    case 'ragflow_parse': return '智能解析'
    case 'classify': return '文章分类'
    case 'llm_proxy': return 'LLM Proxy'
    default: return module
  }
}

const moduleColor = (module: string) => {
  switch (module) {
    case 'ragflow_upload': return 'text-sky-400'
    case 'ragflow_parse': return 'text-violet-400'
    case 'classify': return 'text-amber-400'
    case 'llm_proxy': return 'text-emerald-400'
    default: return 'text-dark-300'
  }
}

const parseTaskStatusBadge = (status: string) => {
  switch (status) {
    case 'running': return 'badge-primary'
    case 'completed': return 'badge-success'
    default: return 'badge-gray'
  }
}

const parseResultBadge = (result?: string) => {
  switch (result) {
    case 'success': return 'badge-success'
    case 'partial': return 'badge-warning'
    case 'failed': return 'badge-danger'
    default: return 'badge-gray'
  }
}

const Logs: Component = () => {
  const [activeTab, setActiveTab] = createSignal<TabKey>('logs')
  const [moduleFilter, setModuleFilter] = createSignal('')
  const [statusFilter, setStatusFilter] = createSignal('')
  const [page, setPage] = createSignal(1)

  // Auto-refresh
  const [autoRefresh, setAutoRefresh] = createSignal(false)
  let refreshTimer: ReturnType<typeof setInterval> | undefined

  const startAutoRefresh = () => {
    stopAutoRefresh()
    refreshTimer = setInterval(() => {
      refetchLogs()
      if (activeTab() === 'parse-tasks') refetchTasks()
    }, 5000)
  }

  const stopAutoRefresh = () => {
    if (refreshTimer) {
      clearInterval(refreshTimer)
      refreshTimer = undefined
    }
  }

  onCleanup(stopAutoRefresh)

  const toggleAutoRefresh = () => {
    const next = !autoRefresh()
    setAutoRefresh(next)
    if (next) startAutoRefresh()
    else stopAutoRefresh()
  }

  // Logs data
  const [logsData, { refetch: refetchLogs }] = createResource(
    () => ({ module: moduleFilter(), status: statusFilter(), page: page(), limit: 30 }),
    (params) => logsApi.list(params).then(r => r.data)
  )

  const [modules] = createResource(() => logsApi.modules().then(r => r.data))

  // Parse tasks data
  const [parseTasks, { refetch: refetchTasks }] = createResource(
    () => activeTab() === 'parse-tasks',
    (active) => active ? ragflowApi.listParseTasks().then(r => r.data || []) : Promise.resolve([])
  )

  const totalPages = () => {
    const data = logsData()
    if (!data) return 1
    return Math.max(1, Math.ceil(data.total / data.limit))
  }

  // --- Tab rendering ---

  const renderLogsTab = () => (
    <div class="space-y-4">
      {/* Filters */}
      <div class="flex flex-wrap gap-3 items-center">
        <select
          class="input w-40"
          value={moduleFilter()}
          onInput={(e) => { setModuleFilter(e.currentTarget.value); setPage(1) }}
        >
          <option value="">全部模块</option>
          <For each={modules() || []}>
            {(m) => <option value={m}>{moduleLabel(m)}</option>}
          </For>
        </select>

        <select
          class="input w-32"
          value={statusFilter()}
          onInput={(e) => { setStatusFilter(e.currentTarget.value); setPage(1) }}
        >
          <option value="">全部状态</option>
          <option value="success">成功</option>
          <option value="error">失败</option>
          <option value="info">信息</option>
        </select>

        <button class="btn btn-secondary btn-sm" onClick={() => refetchLogs()}>
          <svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          刷新
        </button>

        <Show when={logsData()}>
          <span class="text-xs text-dark-500 ml-auto">
            共 {logsData()!.total} 条
          </span>
        </Show>
      </div>

      {/* Log entries */}
      <Show when={!logsData.loading} fallback={<div class="text-center text-dark-400 py-8">加载中...</div>}>
        <div class="space-y-2">
          <For each={logsData()?.items || []} fallback={<div class="text-center text-dark-500 py-8">暂无日志</div>}>
            {(log: ActivityLog) => (
              <div class="card p-3 hover:bg-dark-700/50 transition-colors">
                <div class="flex items-start gap-3">
                  <div class="flex-shrink-0 mt-0.5">
                    <span class={`text-xs font-mono ${moduleColor(log.module)}`}>
                      {moduleLabel(log.module)}
                    </span>
                  </div>
                  <div class="flex-1 min-w-0">
                    <p class="text-sm text-dark-200 break-words">{log.summary}</p>
                    <div class="flex items-center gap-2 mt-1">
                      <span class={`badge badge-sm ${statusBadge(log.status)}`}>
                        {statusLabel(log.status)}
                      </span>
                      <span class="text-xs text-dark-500">{log.action}</span>
                      <Show when={log.duration_ms}>
                        <span class="text-xs text-dark-500">{formatDuration(log.duration_ms)}</span>
                      </Show>
                      <span class="text-xs text-dark-600 ml-auto">{formatDateTime(log.created_at)}</span>
                    </div>
                  </div>
                </div>
              </div>
            )}
          </For>
        </div>

        {/* Pagination */}
        <Show when={totalPages() > 1}>
          <div class="flex justify-center items-center gap-2 mt-4">
            <button
              class="btn btn-secondary btn-sm"
              disabled={page() <= 1}
              onClick={() => setPage(p => Math.max(1, p - 1))}
            >
              上一页
            </button>
            <span class="text-sm text-dark-400">
              {page()} / {totalPages()}
            </span>
            <button
              class="btn btn-secondary btn-sm"
              disabled={page() >= totalPages()}
              onClick={() => setPage(p => p + 1)}
            >
              下一页
            </button>
          </div>
        </Show>
      </Show>
    </div>
  )

  const renderParseTasksTab = () => (
    <div class="space-y-4">
      <div class="flex items-center gap-3">
        <button class="btn btn-secondary btn-sm" onClick={() => refetchTasks()}>
          <svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          刷新
        </button>
        <span class="text-xs text-dark-500">
          显示内存中的解析任务（重启后清空）
        </span>
      </div>

      <Show when={!parseTasks.loading} fallback={<div class="text-center text-dark-400 py-8">加载中...</div>}>
        <For each={parseTasks() || []} fallback={<div class="text-center text-dark-500 py-8">暂无解析任务</div>}>
          {(task: ParseTask) => (
            <div class="card p-4 space-y-3">
              {/* Task header */}
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-2">
                  <span class="font-mono text-sm text-dark-200">{task.id}</span>
                  <span class={`badge badge-sm ${parseTaskStatusBadge(task.status)}`}>
                    {task.status === 'running' ? '运行中' : '已完成'}
                  </span>
                  <Show when={task.result_status}>
                    <span class={`badge badge-sm ${parseResultBadge(task.result_status)}`}>
                      {task.result_status}
                    </span>
                  </Show>
                </div>
                <span class="text-xs text-dark-500">{formatDateTime(task.started_at)}</span>
              </div>

              {/* Progress bar */}
              <div>
                <div class="flex justify-between text-xs text-dark-400 mb-1">
                  <span>进度: {task.completed + task.failed} / {task.total}</span>
                  <span>成功 {task.completed} | 失败 {task.failed} | 待处理 {task.pending}</span>
                </div>
                <div class="w-full bg-dark-700 rounded-full h-2.5">
                  <div class="flex h-2.5 rounded-full overflow-hidden">
                    <div
                      class="bg-emerald-500 transition-all duration-300"
                      style={{ width: `${task.total > 0 ? (task.completed / task.total) * 100 : 0}%` }}
                    />
                    <div
                      class="bg-red-500 transition-all duration-300"
                      style={{ width: `${task.total > 0 ? (task.failed / task.total) * 100 : 0}%` }}
                    />
                  </div>
                </div>
              </div>

              {/* Current stage */}
              <Show when={task.current_stage && task.status === 'running'}>
                <div class="flex items-center gap-2 text-xs text-dark-400">
                  <div class="status-dot status-dot-primary animate-pulse" />
                  <span>阶段: {task.current_stage}</span>
                  <Show when={task.current_dataset_id}>
                    <span>| 当前 dataset: {task.current_dataset_id}</span>
                  </Show>
                  <Show when={task.current_batch_index}>
                    <span>| 批次 #{task.current_batch_index}</span>
                  </Show>
                </div>
              </Show>

              {/* Stats row */}
              <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs">
                <div class="bg-dark-800 rounded-lg p-2 text-center">
                  <div class="text-dark-500">运行中</div>
                  <div class="text-lg font-semibold text-sky-400">{task.running_count}</div>
                </div>
                <div class="bg-dark-800 rounded-lg p-2 text-center">
                  <div class="text-dark-500">恢复中</div>
                  <div class="text-lg font-semibold text-amber-400">{task.recovering_count}</div>
                </div>
                <div class="bg-dark-800 rounded-lg p-2 text-center">
                  <div class="text-dark-500">已完成</div>
                  <div class="text-lg font-semibold text-emerald-400">{task.succeeded_count}</div>
                </div>
                <div class="bg-dark-800 rounded-lg p-2 text-center">
                  <div class="text-dark-500">最终失败</div>
                  <div class="text-lg font-semibold text-red-400">{task.final_failed_count}</div>
                </div>
              </div>

              {/* Failed docs */}
              <Show when={task.failed_docs && task.failed_docs.length > 0}>
                <details class="text-xs">
                  <summary class="text-red-400 cursor-pointer hover:text-red-300">
                    失败文档 ({task.failed_docs!.length})
                  </summary>
                  <div class="mt-2 space-y-1 max-h-40 overflow-y-auto">
                    <For each={task.failed_docs!}>
                      {(doc) => (
                        <div class="bg-dark-800 rounded p-2 font-mono text-dark-400">
                          <span class="text-dark-500">{doc.document_id.slice(0, 12)}...</span>
                          <span class="text-red-400 ml-2">{doc.error}</span>
                          <span class="text-dark-600 ml-2">({doc.retries} retries)</span>
                        </div>
                      )}
                    </For>
                  </div>
                </details>
              </Show>

              {/* Logs */}
              <Show when={task.log && task.log.length > 0}>
                <details class="text-xs">
                  <summary class="text-dark-400 cursor-pointer hover:text-dark-300">
                    任务日志 ({task.log!.length} 条)
                  </summary>
                  <div class="mt-2 bg-dark-900 rounded-lg p-3 max-h-60 overflow-y-auto font-mono text-dark-400 space-y-0.5">
                    <For each={task.log!}>
                      {(line) => <div class="whitespace-pre-wrap break-all">{line}</div>}
                    </For>
                  </div>
                </details>
              </Show>

              {/* Duration */}
              <Show when={task.completed_at}>
                <div class="text-xs text-dark-500">
                  完成于 {formatDateTime(task.completed_at)} |
                  耗时 {formatDuration(new Date(task.completed_at!).getTime() - new Date(task.started_at).getTime())}
                </div>
              </Show>
            </div>
          )}
        </For>
      </Show>
    </div>
  )

  return (
    <div class="space-y-6">
      {/* Header */}
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-2xl font-bold text-dark-100">操作日志</h1>
          <p class="text-dark-500 text-sm mt-1">查看系统操作记录和解析任务进度</p>
        </div>
        <button
          class={`btn btn-sm ${autoRefresh() ? 'btn-primary' : 'btn-secondary'}`}
          onClick={toggleAutoRefresh}
        >
          <Show when={autoRefresh()} fallback="自动刷新: 关">
            自动刷新: 开 (5s)
          </Show>
        </button>
      </div>

      {/* Tabs */}
      <div class="flex gap-1 bg-dark-800/50 p-1 rounded-xl w-fit">
        <button
          class={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
            activeTab() === 'logs'
              ? 'bg-dark-700 text-dark-100 shadow-sm'
              : 'text-dark-400 hover:text-dark-200'
          }`}
          onClick={() => setActiveTab('logs')}
        >
          操作日志
        </button>
        <button
          class={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
            activeTab() === 'parse-tasks'
              ? 'bg-dark-700 text-dark-100 shadow-sm'
              : 'text-dark-400 hover:text-dark-200'
          }`}
          onClick={() => setActiveTab('parse-tasks')}
        >
          解析任务
        </button>
      </div>

      {/* Tab content */}
      <Show when={activeTab() === 'logs'}>
        {renderLogsTab()}
      </Show>
      <Show when={activeTab() === 'parse-tasks'}>
        {renderParseTasksTab()}
      </Show>
    </div>
  )
}

export default Logs
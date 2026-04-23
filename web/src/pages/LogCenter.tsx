import { Component, For, Show, createResource, createSignal, onCleanup } from 'solid-js'
import { logCenterApi } from '@/api'
import type { LogEntry, LogSource, LogAlertRule, DashboardData, ParseTask } from '@/types'

type TabKey = 'browser' | 'dashboard' | 'sources' | 'alerts' | 'parse-tasks'

// --- Utilities ---

const formatDateTime = (value?: string) => {
  if (!value) return '--'
  return new Date(value).toLocaleString('zh-CN')
}

const formatDuration = (ms?: number) => {
  if (!ms) return '--'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

const levelColor = (level: string) => {
  switch (level) {
    case 'error': return 'text-red-400 bg-red-400/10 border-red-400/30'
    case 'warn': return 'text-amber-400 bg-amber-400/10 border-amber-400/30'
    case 'info': return 'text-sky-400 bg-sky-400/10 border-sky-400/30'
    case 'debug': return 'text-dark-400 bg-dark-400/10 border-dark-400/30'
    default: return 'text-dark-300 bg-dark-300/10 border-dark-300/30'
  }
}

const levelLabel = (level: string) => {
  switch (level) {
    case 'error': return '错误'
    case 'warn': return '警告'
    case 'info': return '信息'
    case 'debug': return '调试'
    default: return level
  }
}

const statusColor = (status: string) => {
  switch (status) {
    case 'success': return 'text-emerald-400 bg-emerald-400/10 border-emerald-400/30'
    case 'failed': return 'text-red-400 bg-red-400/10 border-red-400/30'
    case 'skipped': return 'text-dark-400 bg-dark-400/10 border-dark-400/30'
    default: return 'text-dark-300 bg-dark-300/10 border-dark-300/30'
  }
}

const statusLabel = (status: string) => {
  switch (status) {
    case 'success': return '成功'
    case 'failed': return '失败'
    case 'skipped': return '跳过'
    default: return status
  }
}

const moduleLabel = (module: string) => {
  const map: Record<string, string> = {
    ragflow_upload: '文件上传',
    ragflow_parse: '智能解析',
    classify: '文章分类',
    llm_proxy: 'LLM Proxy',
    rss_fetch: 'RSS采集',
    file_ingest: '文件入库',
    crawler: '爬虫',
    matrix_notify: 'Matrix通知',
    'bellkeeper-core': '核心系统',
  }
  return map[module] || module
}

const sourceTypeLabel = (t: string) => {
  const map: Record<string, string> = { internal: '内部', n8n: 'n8n', external: '外部' }
  return map[t] || t
}

const parseTaskStatusBadge = (status: string) => {
  switch (status) {
    case 'running': return 'badge-primary'
    case 'recovering': return 'badge-warning'
    case 'completed': return 'badge-success'
    default: return 'badge-gray'
  }
}

const parseResultBadge = (result?: string) => {
  switch (result) {
    case 'success': return 'badge-success'
    case 'partial_failed': return 'badge-warning'
    case 'failed': return 'badge-danger'
    default: return 'badge-gray'
  }
}

// --- Tab 1: Log Browser ---

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
    <div class="space-y-4">
      {/* Filters */}
      <div class="flex flex-wrap gap-3 items-center">
        <select
          class="input w-40"
          value={moduleFilter()}
          onChange={(e) => { setModuleFilter(e.currentTarget.value); applyFilter() }}
        >
          <option value="">全部模块</option>
          <option value="ragflow_upload">文件上传</option>
          <option value="ragflow_parse">智能解析</option>
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
  )
}

// --- Tab 2: Dashboard ---

const DashboardTab: Component = () => {
  const [period, setPeriod] = createSignal<'24h' | '7d' | '30d'>('24h')

  const [data, { refetch }] = createResource(
    () => period(),
    (p) => logCenterApi.getDashboard(p)
  )

  const totals = () => {
    const d = data()?.data
    if (!d) return { total: 0, errors: 0, warnings: 0 }
    const total = d.by_level.reduce((s, x) => s + x.count, 0)
    const errors = d.by_level.find(x => x.level === 'error')?.count || 0
    const warnings = d.by_level.find(x => x.level === 'warn')?.count || 0
    return { total, errors, warnings }
  }

  // CSS bar chart helpers
  const barWidth = (count: number, max: number) => {
    if (max === 0) return '0%'
    return `${Math.max(4, (count / max) * 100)}%`
  }

  // Group by_hour data into hours array
  const hourData = () => {
    const d = data()?.data
    if (!d) return [] as { hour: string; levels: { level: string; count: number }[]; total: number }[]
    const hours = new Map<string, Map<string, number>>()
    for (const h of d.by_hour) {
      if (!hours.has(h.hour)) hours.set(h.hour, new Map())
      hours.get(h.hour)!.set(h.level, h.count)
    }
    const sorted = Array.from(hours.entries()).sort((a, b) => a[0].localeCompare(b[0]))
    return sorted.map(([hour, levels]) => {
      let total = 0
      const arr: { level: string; count: number }[] = []
      levels.forEach((count, level) => { arr.push({ level, count }); total += count })
      return { hour, levels: arr, total }
    })
  }

  const hourMax = () => Math.max(1, ...hourData().map(h => h.total))

  // conic-gradient for module pie
  const pieGradient = () => {
    const d = data()?.data
    if (!d || d.by_module.length === 0) return ''
    const colors = ['#3b82f6', '#8b5cf6', '#f59e0b', '#10b981', '#ef4444', '#06b6d4', '#ec4899', '#6366f1']
    const total = d.by_module.reduce((s, x) => s + x.count, 0)
    let pct = 0
    return d.by_module.map((m, i) => {
      const p = (m.count / total) * 100
      const start = pct
      pct += p
      return `${colors[i % colors.length]} ${start}% ${pct}%`
    }).join(', ')
  }

  const periodLabel = (p: string) => {
    if (p === '24h') return '24小时'
    if (p === '7d') return '7天'
    return '30天'
  }

  return (
    <div class="space-y-6">
      {/* Period selector */}
      <div class="flex items-center justify-between">
        <div class="flex gap-1 bg-dark-800/50 p-1 rounded-xl">
          <For each={['24h', '7d', '30d'] as const}>
            {(p) => (
              <button
                class={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
                  period() === p ? 'bg-dark-700 text-dark-100 shadow-sm' : 'text-dark-400 hover:text-dark-200'
                }`}
                onClick={() => setPeriod(p)}
              >
                {periodLabel(p)}
              </button>
            )}
          </For>
        </div>
        <button class="btn btn-secondary btn-sm" onClick={() => refetch()}>刷新</button>
      </div>

      {/* Summary cards */}
      <div class="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <div class="card p-4 text-center">
          <div class="text-dark-500 text-sm">总日志量</div>
          <div class="text-3xl font-bold text-dark-100 mt-1">{totals().total.toLocaleString()}</div>
        </div>
        <div class="card p-4 text-center">
          <div class="text-dark-500 text-sm">错误数</div>
          <div class="text-3xl font-bold text-red-400 mt-1">{totals().errors.toLocaleString()}</div>
        </div>
        <div class="card p-4 text-center">
          <div class="text-dark-500 text-sm">警告数</div>
          <div class="text-3xl font-bold text-amber-400 mt-1">{totals().warnings.toLocaleString()}</div>
        </div>
        <div class="card p-4 text-center">
          <div class="text-dark-500 text-sm">错误率</div>
          <div class="text-3xl font-bold text-red-400 mt-1">
            {totals().total > 0 ? ((totals().errors / totals().total) * 100).toFixed(1) : 0}%
          </div>
        </div>
      </div>

      <Show when={!data.loading} fallback={<div class="text-center text-dark-400 py-8">加载中...</div>}>
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
          {/* Hour timeline - bar chart */}
          <div class="card p-4">
            <h3 class="text-sm font-semibold text-dark-200 mb-4">日志量时序</h3>
            <Show when={hourData().length > 0} fallback={<div class="text-dark-500 text-sm">暂无数据</div>}>
              <div class="flex items-end gap-0.5 h-40">
                <For each={hourData()}>
                  {(h) => (
                    <div class="flex-1 flex flex-col items-center justify-end h-full">
                      <div
                        class="w-full bg-sky-500 rounded-t hover:bg-sky-400 transition-colors"
                        style={{ height: `${(h.total / hourMax()) * 100}%` }}
                        title={`${h.hour}: ${h.total} 条`}
                      />
                    </div>
                  )}
                </For>
              </div>
              <div class="flex gap-0.5 mt-1">
                <For each={hourData()}>
                  {(_, i) => (
                    <div class="flex-1 text-center">
                      <Show when={i() % Math.max(1, Math.floor(hourData().length / 6)) === 0}>
                        <span class="text-xs text-dark-600">{hourData()[i()]?.hour.slice(5)}</span>
                      </Show>
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </div>

          {/* Module distribution - pie chart */}
          <div class="card p-4">
            <h3 class="text-sm font-semibold text-dark-200 mb-4">模块分布</h3>
            <Show when={data()?.data?.by_module && data()!.data!.by_module.length > 0} fallback={<div class="text-dark-500 text-sm">暂无数据</div>}>
              <div class="flex items-center gap-6">
                <div
                  class="w-32 h-32 rounded-full flex-shrink-0"
                  style={{ 'background': `conic-gradient(${pieGradient()})` }}
                />
                <div class="space-y-1.5 flex-1 min-w-0">
                  <For each={data()?.data?.by_module || []}>
                    {(m) => (
                      <div class="flex items-center justify-between text-sm">
                        <span class="text-dark-300 truncate">{moduleLabel(m.module)}</span>
                        <span class="text-dark-100 font-mono ml-2">{m.count}</span>
                      </div>
                    )}
                  </For>
                </div>
              </div>
            </Show>
          </div>

          {/* Level distribution - horizontal bars */}
          <div class="card p-4">
            <h3 class="text-sm font-semibold text-dark-200 mb-4">级别分布</h3>
            <Show when={data()?.data?.by_level && data()!.data!.by_level.length > 0} fallback={<div class="text-dark-500 text-sm">暂无数据</div>}>
              <div class="space-y-3">
                <For each={data()?.data?.by_level || []}>
                  {(l) => {
                    const max = Math.max(1, ...(data()?.data?.by_level || []).map(x => x.count))
                    return (
                      <div>
                        <div class="flex justify-between text-sm mb-1">
                          <span class={`font-medium ${levelColor(l.level).split(' ')[0]}`}>{levelLabel(l.level)}</span>
                          <span class="text-dark-400 font-mono">{l.count}</span>
                        </div>
                        <div class="w-full bg-dark-700 rounded-full h-2.5">
                          <div
                            class="h-2.5 rounded-full transition-all"
                            style={{
                              width: barWidth(l.count, max),
                              'background-color': l.level === 'error' ? '#ef4444' : l.level === 'warn' ? '#f59e0b' : l.level === 'info' ? '#3b82f6' : '#6b7280',
                            }}
                          />
                        </div>
                      </div>
                    )
                  }}
                </For>
              </div>
            </Show>
          </div>

          {/* Top error modules */}
          <div class="card p-4">
            <h3 class="text-sm font-semibold text-dark-200 mb-4">Top 失败模块</h3>
            <Show when={data()?.data?.top_errors && data()!.data!.top_errors.length > 0} fallback={<div class="text-dark-500 text-sm">暂无错误</div>}>
              <div class="space-y-2">
                <For each={data()?.data?.top_errors || []}>
                  {(m, i) => {
                    const max = Math.max(1, ...(data()?.data?.top_errors || []).map(x => x.count))
                    return (
                      <div class="flex items-center gap-3">
                        <span class="text-dark-500 text-sm w-6 text-right">#{i() + 1}</span>
                        <span class="text-dark-300 text-sm w-28 truncate">{moduleLabel(m.module)}</span>
                        <div class="flex-1 bg-dark-700 rounded-full h-2">
                          <div
                            class="h-2 rounded-full bg-red-500 transition-all"
                            style={{ width: barWidth(m.count, max) }}
                          />
                        </div>
                        <span class="text-red-400 font-mono text-sm w-10 text-right">{m.count}</span>
                      </div>
                    )
                  }}
                </For>
              </div>
            </Show>
          </div>
        </div>
      </Show>
    </div>
  )
}

// --- Tab 3: Log Sources ---

const SourcesTab: Component = () => {
  const [sources, { refetch }] = createResource(
    () => true,
    () => logCenterApi.listSources()
  )
  const [showCreate, setShowCreate] = createSignal(false)
  const [editingId, setEditingId] = createSignal<number | null>(null)
  const [newApiKey, setNewApiKey] = createSignal<string | null>(null)

  const createForm = { name: '', source_type: 'internal', description: '' }
  const [form, setForm] = createSignal({ ...createForm })
  const [editForm, setEditForm] = createSignal<{ name: string; description: string; is_active: boolean }>({ name: '', description: '', is_active: true })

  const handleCreate = async () => {
    const f = form()
    if (!f.name) return
    const res = await logCenterApi.registerSource(f as any)
    setNewApiKey((res.data as any).api_key || null)
    refetch()
  }

  const startEdit = (s: LogSource) => {
    setEditingId(s.id)
    setEditForm({ name: s.name, description: s.description, is_active: s.is_active })
  }

  const saveEdit = async (id: number) => {
    const f = editForm()
    await logCenterApi.updateSource(id, f)
    setEditingId(null)
    refetch()
  }

  const handleDelete = async (id: number) => {
    if (!confirm('确认删除该日志源？')) return
    await logCenterApi.deleteSource(id)
    refetch()
  }

  return (
    <div class="space-y-4">
      <div class="flex justify-between items-center">
        <span class="text-sm text-dark-400">管理日志写入源的 API Key 和配置</span>
        <button class="btn btn-primary btn-sm" onClick={() => { setShowCreate(true); setNewApiKey(null); setForm(createForm) }}>
          注册新源
        </button>
      </div>

      <Show when={showCreate()}>
        <div class="card p-4 space-y-3">
          <h3 class="font-semibold text-dark-200">注册日志源</h3>
          <div class="grid grid-cols-3 gap-3">
            <input class="input" placeholder="名称 (如 n8n-k02)" value={form().name}
              onInput={(e) => setForm({ ...form(), name: e.currentTarget.value })} />
            <select class="input" value={form().source_type}
              onChange={(e) => setForm({ ...form(), source_type: e.currentTarget.value })}>
              <option value="internal">内部</option>
              <option value="n8n">n8n</option>
              <option value="external">外部</option>
            </select>
            <input class="input" placeholder="描述" value={form().description}
              onInput={(e) => setForm({ ...form(), description: e.currentTarget.value })} />
          </div>
          <div class="flex gap-2">
            <button class="btn btn-primary btn-sm" onClick={handleCreate}>创建</button>
            <button class="btn btn-secondary btn-sm" onClick={() => setShowCreate(false)}>取消</button>
          </div>
          <Show when={newApiKey()}>
            <div class="bg-amber-500/10 border border-amber-500/30 rounded-lg p-3 text-sm">
              <p class="text-amber-300 font-medium">API Key（仅显示一次，请保存）：</p>
              <code class="text-amber-200 font-mono mt-1 block select-all">{newApiKey()}</code>
            </div>
          </Show>
        </div>
      </Show>

      <Show when={!sources.loading} fallback={<div class="text-center text-dark-400 py-8">加载中...</div>}>
        <div class="card overflow-hidden">
          <table class="w-full text-sm">
            <thead class="bg-dark-800/80 text-dark-400">
              <tr>
                <th class="text-left px-4 py-2">名称</th>
                <th class="text-left px-4 py-2">类型</th>
                <th class="text-left px-4 py-2">描述</th>
                <th class="text-left px-4 py-2">状态</th>
                <th class="text-left px-4 py-2">创建时间</th>
                <th class="text-left px-4 py-2 w-32">操作</th>
              </tr>
            </thead>
            <tbody>
              <For each={sources()?.data || []}>
                {(s) => (
                  <tr class="border-t border-dark-700/50">
                    <td class="px-4 py-2">
                      <Show when={editingId() === s.id} fallback={<span class="text-dark-200">{s.name}</span>}>
                        <input class="input" value={editForm().name}
                          onInput={(e) => setEditForm({ ...editForm(), name: e.currentTarget.value })} />
                      </Show>
                    </td>
                    <td class="px-4 py-2 text-dark-400">{sourceTypeLabel(s.source_type)}</td>
                    <td class="px-4 py-2 text-dark-400">
                      <Show when={editingId() === s.id} fallback={s.description || '--'}>
                        <input class="input" value={editForm().description}
                          onInput={(e) => setEditForm({ ...editForm(), description: e.currentTarget.value })} />
                      </Show>
                    </td>
                    <td class="px-4 py-2">
                      <Show when={editingId() === s.id} fallback={
                        <span class={`text-xs px-1.5 py-0.5 rounded ${s.is_active ? 'text-emerald-400 bg-emerald-400/10' : 'text-dark-500 bg-dark-500/10'}`}>
                          {s.is_active ? '活跃' : '停用'}
                        </span>
                      }>
                        <label class="flex items-center gap-2">
                          <input type="checkbox" checked={editForm().is_active}
                            onChange={(e) => setEditForm({ ...editForm(), is_active: e.currentTarget.checked })} />
                          <span class="text-dark-400">活跃</span>
                        </label>
                      </Show>
                    </td>
                    <td class="px-4 py-2 text-dark-500 text-xs">{formatDateTime(s.created_at)}</td>
                    <td class="px-4 py-2">
                      <Show when={editingId() === s.id} fallback={
                        <div class="flex gap-1">
                          <button class="text-sky-400 hover:text-sky-300 text-xs" onClick={() => startEdit(s)}>编辑</button>
                          <button class="text-red-400 hover:text-red-300 text-xs" onClick={() => handleDelete(s.id)}>删除</button>
                        </div>
                      }>
                        <div class="flex gap-1">
                          <button class="text-emerald-400 hover:text-emerald-300 text-xs" onClick={() => saveEdit(s.id)}>保存</button>
                          <button class="text-dark-400 hover:text-dark-300 text-xs" onClick={() => setEditingId(null)}>取消</button>
                        </div>
                      </Show>
                    </td>
                  </tr>
                )}
              </For>
            </tbody>
          </table>
          <Show when={(sources()?.data || []).length === 0}>
            <div class="text-center text-dark-500 py-8">暂无日志源</div>
          </Show>
        </div>
      </Show>
    </div>
  )
}

// --- Tab 4: Alert Rules ---

const AlertsTab: Component = () => {
  const [alerts, { refetch }] = createResource(
    () => true,
    () => logCenterApi.listAlerts()
  )
  const [showCreate, setShowCreate] = createSignal(false)
  const [editingId, setEditingId] = createSignal<number | null>(null)
  const [alertResult, setAlertResult] = createSignal<string | null>(null)

  const alertForm = { name: '', module: 'rss_fetch', level: 'error', threshold: 5, window_minutes: 30, notify_channel: 'daily' }
  const [form, setForm] = createSignal({ ...alertForm })

  const handleCreate = async () => {
    const f = form()
    if (!f.name) return
    await logCenterApi.createAlert({
      name: f.name,
      condition: { module: f.module, level: f.level, threshold: f.threshold, window_minutes: f.window_minutes },
      notify_channel: f.notify_channel,
    })
    setShowCreate(false)
    setForm(alertForm)
    refetch()
  }

  const toggleActive = async (rule: LogAlertRule) => {
    await logCenterApi.updateAlert(rule.id, { is_active: !rule.is_active })
    refetch()
  }

  const handleDelete = async (id: number) => {
    if (!confirm('确认删除该告警规则？')) return
    await logCenterApi.deleteAlert(id)
    refetch()
  }

  const conditionLabel = (cond: LogAlertRule['condition']) => (
    <span class="text-xs text-dark-300">
      {moduleLabel(cond.module)} / {levelLabel(cond.level)} / {cond.threshold}次 / {cond.window_minutes}分钟
    </span>
  )

  return (
    <div class="space-y-4">
      <div class="flex justify-between items-center">
        <span class="text-sm text-dark-400">设置日志告警规则，当满足条件时触发通知</span>
        <button class="btn btn-primary btn-sm" onClick={() => setShowCreate(true)}>
          创建规则
        </button>
      </div>

      <Show when={showCreate()}>
        <div class="card p-4 space-y-3">
          <h3 class="font-semibold text-dark-200">创建告警规则</h3>
          <div class="grid grid-cols-2 gap-3">
            <input class="input" placeholder="规则名称" value={form().name}
              onInput={(e) => setForm({ ...form(), name: e.currentTarget.value })} />
            <select class="input" value={form().module}
              onChange={(e) => setForm({ ...form(), module: e.currentTarget.value })}>
              <option value="rss_fetch">RSS采集</option>
              <option value="ragflow_parse">智能解析</option>
              <option value="classify">文章分类</option>
              <option value="llm_proxy">LLM Proxy</option>
              <option value="file_ingest">文件入库</option>
              <option value="crawler">爬虫</option>
              <option value="matrix_notify">Matrix通知</option>
            </select>
            <select class="input" value={form().level}
              onChange={(e) => setForm({ ...form(), level: e.currentTarget.value })}>
              <option value="error">错误</option>
              <option value="warn">警告</option>
            </select>
            <div class="flex gap-2">
              <input class="input flex-1" type="number" placeholder="阈值" value={form().threshold}
                onInput={(e) => setForm({ ...form(), threshold: parseInt(e.currentTarget.value) || 0 })} />
              <input class="input flex-1" type="number" placeholder="窗口(分)" value={form().window_minutes}
                onInput={(e) => setForm({ ...form(), window_minutes: parseInt(e.currentTarget.value) || 0 })} />
            </div>
            <input class="input" placeholder="通知渠道 (如 daily)" value={form().notify_channel}
              onInput={(e) => setForm({ ...form(), notify_channel: e.currentTarget.value })} />
          </div>
          <div class="flex gap-2">
            <button class="btn btn-primary btn-sm" onClick={handleCreate}>创建</button>
            <button class="btn btn-secondary btn-sm" onClick={() => { setShowCreate(false); setForm(alertForm) }}>取消</button>
          </div>
        </div>
      </Show>

      <Show when={alertResult()}>
        <div class="card p-3 bg-sky-500/10 border border-sky-500/30">
          <pre class="text-xs text-sky-300 font-mono whitespace-pre-wrap">{alertResult()}</pre>
        </div>
      </Show>

      <Show when={!alerts.loading} fallback={<div class="text-center text-dark-400 py-8">加载中...</div>}>
        <div class="space-y-2">
          <For each={alerts()?.data || []} fallback={<div class="text-center text-dark-500 py-8">暂无告警规则</div>}>
            {(rule) => (
              <div class="card p-4 flex items-center gap-4">
                {/* Toggle */}
                <button
                  class={`w-10 h-5 rounded-full relative transition-colors ${rule.is_active ? 'bg-emerald-500' : 'bg-dark-600'}`}
                  onClick={() => toggleActive(rule)}
                >
                  <div class={`w-4 h-4 rounded-full bg-white absolute top-0.5 transition-all ${rule.is_active ? 'left-5' : 'left-0.5'}`} />
                </button>

                <div class="flex-1 min-w-0">
                  <div class="flex items-center gap-2">
                    <span class="font-medium text-dark-200">{rule.name}</span>
                    {!rule.is_active && <span class="text-xs text-dark-500">(已停用)</span>}
                  </div>
                  <div class="mt-1">{conditionLabel(rule.condition)}</div>
                  <div class="text-xs text-dark-500 mt-0.5">通知: {rule.notify_channel}</div>
                </div>

                <div class="flex gap-1">
                  <button class="text-red-400 hover:text-red-300 text-xs" onClick={() => handleDelete(rule.id)}>删除</button>
                </div>
              </div>
            )}
          </For>
        </div>
      </Show>
    </div>
  )
}

// --- Tab 5: Parse Tasks (migrated from old Logs.tsx) ---

const ParseTasksTab: Component = () => {
  const [parseTasks, { refetch }] = createResource(
    () => true,
    () => logCenterApi.listParseTasks().then(r => r.data || [])
  )

  const formatDuration = (ms: number) => {
    if (ms < 1000) return `${ms}ms`
    return `${(ms / 1000).toFixed(1)}s`
  }

  return (
    <div class="space-y-4">
      <div class="flex items-center gap-3">
        <button class="btn btn-secondary btn-sm" onClick={() => refetch()}>刷新</button>
        <span class="text-xs text-dark-500">显示内存中的解析任务（重启后清空）</span>
      </div>

      <Show when={!parseTasks.loading} fallback={<div class="text-center text-dark-400 py-8">加载中...</div>}>
        <For each={parseTasks() || []} fallback={<div class="text-center text-dark-500 py-8">暂无解析任务</div>}>
          {(task: ParseTask) => (
            <div class="card p-4 space-y-3">
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-2">
                  <span class="font-mono text-sm text-dark-200">{task.id}</span>
                  <span class={`badge badge-sm ${parseTaskStatusBadge(task.status)}`}>
                    {task.status === 'running' ? '运行中' : task.status === 'recovering' ? '恢复中' : '已完成'}
                  </span>
                  <Show when={task.result_status}>
                    <span class={`badge badge-sm ${parseResultBadge(task.result_status)}`}>
                      {task.result_status === 'success' ? '全部成功' : task.result_status === 'partial_failed' ? '部分失败' : task.result_status === 'failed' ? '全部失败' : task.result_status}
                    </span>
                  </Show>
                </div>
                <span class="text-xs text-dark-500">{formatDateTime(task.started_at)}</span>
              </div>

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

              <Show when={task.current_stage && task.status !== 'completed'}>
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
}

// --- Main LogCenter Component ---

const LogCenter: Component = () => {
  const [activeTab, setActiveTab] = createSignal<TabKey>('browser')

  const tabs: { key: TabKey; label: string }[] = [
    { key: 'browser', label: '日志浏览器' },
    { key: 'dashboard', label: '仪表盘' },
    { key: 'sources', label: '日志源' },
    { key: 'alerts', label: '告警规则' },
    { key: 'parse-tasks', label: '解析任务' },
  ]

  return (
    <div class="space-y-6">
      {/* Header */}
      <div>
        <h1 class="text-2xl font-bold text-dark-100">日志中心</h1>
        <p class="text-dark-500 text-sm mt-1">统一查看、管理和监控系统日志</p>
      </div>

      {/* Tabs */}
      <div class="flex gap-1 bg-dark-800/50 p-1 rounded-xl w-fit">
        <For each={tabs}>
          {(tab) => (
            <button
              class={`px-4 py-2 rounded-lg text-sm font-medium transition-all ${
                activeTab() === tab.key
                  ? 'bg-dark-700 text-dark-100 shadow-sm'
                  : 'text-dark-400 hover:text-dark-200'
              }`}
              onClick={() => setActiveTab(tab.key)}
            >
              {tab.label}
            </button>
          )}
        </For>
      </div>

      {/* Tab content */}
      <Show when={activeTab() === 'browser'}><LogBrowser /></Show>
      <Show when={activeTab() === 'dashboard'}><DashboardTab /></Show>
      <Show when={activeTab() === 'sources'}><SourcesTab /></Show>
      <Show when={activeTab() === 'alerts'}><AlertsTab /></Show>
      <Show when={activeTab() === 'parse-tasks'}><ParseTasksTab /></Show>
    </div>
  )
}

export default LogCenter

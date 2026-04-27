import { Component, For, Show, createResource, createSignal } from 'solid-js'
import { logCenterApi } from '@/api'
import { moduleLabel, levelColor, levelLabel } from './shared'

const LogDashboard: Component = () => {
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
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">日志仪表盘</h1>
          <p class="text-sm text-dark-400 mt-1">日志统计分析与可视化</p>
        </div>
      </div>

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
    </div>
  )
}

export default LogDashboard

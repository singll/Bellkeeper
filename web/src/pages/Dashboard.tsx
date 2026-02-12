import { Component, createSignal, createResource, Show, For } from 'solid-js'
import { A } from '@solidjs/router'
import { healthApi, workflowsApi } from '@/api'
import { useToast } from '@/components/Toast'

const Dashboard: Component = () => {
  const toast = useToast()
  const [health, { refetch: refetchHealth }] = createResource(() => healthApi.detailed())
  const [workflows] = createResource(() => workflowsApi.list())
  const [triggeringWorkflow, setTriggeringWorkflow] = createSignal<string | null>(null)

  const getMetric = (key: string): number | string => {
    const metrics = health()?.metrics
    if (!metrics) return '--'
    const value = metrics[key]
    return typeof value === 'number' ? value : '--'
  }

  const handleTriggerWorkflow = async (name: string) => {
    setTriggeringWorkflow(name)
    try {
      await workflowsApi.trigger(name, {})
      toast.success(`工作流 "${name}" 已触发`)
    } catch (err) {
      toast.error('触发失败: ' + (err as Error).message)
    } finally {
      setTriggeringWorkflow(null)
    }
  }

  const stats = [
    { label: '标签数量', key: 'tags_count', icon: 'M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z', color: 'text-primary-400' },
    { label: 'RSS 订阅', key: 'rss_feeds_count', icon: 'M6 5c7.18 0 13 5.82 13 13M6 11a7 7 0 017 7m-6 0a1 1 0 11-2 0 1 1 0 012 0z', color: 'text-orange-400' },
    { label: '数据源', key: 'datasources_count', icon: 'M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4', color: 'text-emerald-400' },
    { label: '知识库', key: 'datasets_count', icon: 'M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10', color: 'text-purple-400' },
  ]

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">仪表盘</h1>
          <p class="text-sm text-dark-400 mt-1">系统状态概览</p>
        </div>
        <button class="btn btn-secondary" onClick={() => refetchHealth()}>
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          刷新状态
        </button>
      </div>

      {/* System Status */}
      <div class="card mb-6">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-4">
            <div class={`w-12 h-12 rounded-xl flex items-center justify-center ${
              health()?.status === 'healthy'
                ? 'bg-emerald-500/20'
                : 'bg-amber-500/20'
            }`}>
              <svg class={`w-6 h-6 ${
                health()?.status === 'healthy' ? 'text-emerald-400' : 'text-amber-400'
              }`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            </div>
            <div>
              <div class="text-sm text-dark-400">系统状态</div>
              <div class={`text-xl font-bold ${
                health()?.status === 'healthy' ? 'text-emerald-400' : 'text-amber-400'
              }`}>
                <Show when={health()} fallback="加载中...">
                  {health()?.status === 'healthy' ? '运行正常' : '部分降级'}
                </Show>
              </div>
            </div>
          </div>
          <Show when={health()?.version}>
            <div class="text-right">
              <div class="text-sm text-dark-400">版本</div>
              <div class="text-lg font-mono text-dark-200">{health()?.version}</div>
            </div>
          </Show>
        </div>
      </div>

      {/* Stats Grid */}
      <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <For each={stats}>
          {(stat) => (
            <div class="card card-hover">
              <div class="flex items-center gap-3">
                <div class={`w-10 h-10 rounded-lg flex items-center justify-center bg-dark-800/50 ${stat.color}`}>
                  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d={stat.icon} />
                  </svg>
                </div>
                <div>
                  <div class="text-sm text-dark-400">{stat.label}</div>
                  <div class={`text-2xl font-bold ${stat.color}`}>{getMetric(stat.key)}</div>
                </div>
              </div>
            </div>
          )}
        </For>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Service Status */}
        <div class="card">
          <h2 class="text-lg font-semibold text-white mb-4">服务状态</h2>
          <Show when={health()} fallback={
            <div class="flex items-center justify-center py-8">
              <div class="loading-spinner" />
              <span class="ml-3 text-dark-400">加载中...</span>
            </div>
          }>
            {(h) => (
              <div class="space-y-3">
                <For each={Object.entries(h().services || {})}>
                  {([name, status]) => (
                    <div class="flex items-center justify-between p-3 bg-dark-800/50 rounded-xl">
                      <div class="flex items-center gap-3">
                        <span class={`status-dot ${
                          status.status === 'up' ? 'status-dot-success' : 'status-dot-danger'
                        }`} />
                        <span class="font-medium text-white capitalize">{name}</span>
                      </div>
                      <span class={`badge ${
                        status.status === 'up' ? 'badge-success' : 'badge-danger'
                      }`}>
                        {status.status === 'up' ? '在线' : '离线'}
                        {status.latency_ms && ` (${status.latency_ms}ms)`}
                      </span>
                    </div>
                  )}
                </For>
              </div>
            )}
          </Show>
        </div>

        {/* Quick Actions */}
        <div class="card">
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold text-white">快捷操作</h2>
            <A href="/workflows" class="text-sm text-primary-400 hover:text-primary-300">
              查看全部 →
            </A>
          </div>
          <Show
            when={workflows()?.data && workflows()!.data.length > 0}
            fallback={
              <div class="empty-state py-8">
                <svg class="empty-state-icon w-12 h-12" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
                </svg>
                <p class="empty-state-title">暂无可用工作流</p>
                <p class="empty-state-description">配置 n8n API Key 后可查看更多工作流</p>
              </div>
            }
          >
            <div class="space-y-3">
              <For each={workflows()?.data.slice(0, 5)}>
                {(workflow) => (
                  <div class="flex items-center justify-between p-3 bg-dark-800/50 rounded-xl group hover:bg-dark-800/70 transition-colors">
                    <div class="flex items-center gap-3">
                      <span class={`status-dot ${
                        workflow.active ? 'status-dot-success' : 'status-dot-gray'
                      }`} />
                      <span class="font-medium text-white">{workflow.name}</span>
                    </div>
                    <button
                      class="btn btn-primary btn-sm opacity-0 group-hover:opacity-100 transition-opacity"
                      disabled={triggeringWorkflow() === workflow.name}
                      onClick={() => handleTriggerWorkflow(workflow.name)}
                    >
                      {triggeringWorkflow() === workflow.name ? (
                        <div class="loading-spinner w-4 h-4" />
                      ) : (
                        <>
                          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                          </svg>
                          触发
                        </>
                      )}
                    </button>
                  </div>
                )}
              </For>
            </div>
          </Show>
        </div>
      </div>

      {/* Last Update */}
      <div class="mt-6 text-center text-sm text-dark-500">
        <Show when={health()?.metrics?.timestamp}>
          最后更新: {new Date(health()!.metrics!.timestamp as string).toLocaleString('zh-CN')}
        </Show>
      </div>
    </div>
  )
}

export default Dashboard

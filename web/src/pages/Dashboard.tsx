import { Component, createSignal, createResource, Show, For } from 'solid-js'
import { healthApi, workflowsApi } from '@/api'

const Dashboard: Component = () => {
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
      alert(`工作流 "${name}" 已触发`)
    } catch (err) {
      alert('触发失败: ' + (err as Error).message)
    } finally {
      setTriggeringWorkflow(null)
    }
  }

  return (
    <div>
      <div class="flex items-center justify-between mb-6">
        <h1 class="text-2xl font-bold">仪表盘</h1>
        <button
          class="btn btn-secondary text-sm"
          onClick={() => refetchHealth()}
        >
          刷新状态
        </button>
      </div>

      {/* Stats Cards */}
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
        <div class="card">
          <div class="text-dark-muted text-sm">系统状态</div>
          <div class="text-2xl font-bold mt-1">
            <Show when={health()} fallback="加载中...">
              {(h) => (
                <span class={h().status === 'healthy' ? 'text-green-500' : 'text-yellow-500'}>
                  {h().status === 'healthy' ? '正常' : '降级'}
                </span>
              )}
            </Show>
          </div>
        </div>

        <div class="card">
          <div class="text-dark-muted text-sm">标签数量</div>
          <div class="text-2xl font-bold mt-1 text-primary-400">{getMetric('tags_count')}</div>
        </div>

        <div class="card">
          <div class="text-dark-muted text-sm">活跃订阅</div>
          <div class="text-2xl font-bold mt-1 text-primary-400">{getMetric('rss_feeds_count')}</div>
        </div>

        <div class="card">
          <div class="text-dark-muted text-sm">知识库映射</div>
          <div class="text-2xl font-bold mt-1 text-primary-400">{getMetric('datasets_count')}</div>
        </div>
      </div>

      <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-8">
        {/* Service Status */}
        <div class="card">
          <h2 class="text-lg font-semibold mb-4">服务状态</h2>
          <Show when={health()} fallback={<div class="text-dark-muted">加载中...</div>}>
            {(h) => (
              <div class="space-y-3">
                <For each={Object.entries(h().services || {})}>
                  {([name, status]) => (
                    <div class="flex items-center justify-between p-3 bg-dark-bg rounded-lg">
                      <span class="font-medium capitalize">{name}</span>
                      <span
                        class={`px-2 py-1 rounded text-sm ${
                          status.status === 'up'
                            ? 'bg-green-500/20 text-green-400'
                            : 'bg-red-500/20 text-red-400'
                        }`}
                      >
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
          <h2 class="text-lg font-semibold mb-4">快捷操作</h2>
          <Show
            when={workflows()?.data && workflows()!.data.length > 0}
            fallback={
              <div class="text-dark-muted text-sm">
                <p>暂无可用工作流</p>
                <p class="mt-2 text-xs">配置 n8n API Key 后可查看更多工作流</p>
              </div>
            }
          >
            <div class="space-y-3">
              <For each={workflows()?.data.slice(0, 5)}>
                {(workflow) => (
                  <div class="flex items-center justify-between p-3 bg-dark-bg rounded-lg">
                    <div class="flex items-center gap-2">
                      <span
                        class={`w-2 h-2 rounded-full ${
                          workflow.active ? 'bg-green-500' : 'bg-dark-muted'
                        }`}
                      />
                      <span class="font-medium">{workflow.name}</span>
                    </div>
                    <button
                      class="px-3 py-1 text-sm bg-primary-600 hover:bg-primary-500 rounded transition-colors disabled:opacity-50"
                      disabled={triggeringWorkflow() === workflow.name}
                      onClick={() => handleTriggerWorkflow(workflow.name)}
                    >
                      {triggeringWorkflow() === workflow.name ? '触发中...' : '触发'}
                    </button>
                  </div>
                )}
              </For>
              <Show when={workflows()?.data && workflows()!.data.length > 5}>
                <a
                  href="/workflows"
                  class="block text-center text-sm text-primary-500 hover:text-primary-400 py-2"
                >
                  查看全部工作流 →
                </a>
              </Show>
            </div>
          </Show>
        </div>
      </div>

      {/* Additional Stats Row */}
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div class="card">
          <div class="text-dark-muted text-sm">数据源</div>
          <div class="text-2xl font-bold mt-1 text-primary-400">{getMetric('datasources_count')}</div>
        </div>
        <div class="card">
          <div class="text-dark-muted text-sm">最后更新</div>
          <div class="text-lg font-medium mt-1 text-dark-muted">
            <Show when={health()?.metrics?.timestamp} fallback="--">
              {(ts) => new Date(ts() as string).toLocaleString()}
            </Show>
          </div>
        </div>
      </div>
    </div>
  )
}

export default Dashboard

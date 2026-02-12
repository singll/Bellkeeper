import { Component, createResource, Show, For } from 'solid-js'
import { healthApi } from '@/api'

const Dashboard: Component = () => {
  const [health] = createResource(() => healthApi.detailed())

  const getMetric = (key: string): number | string => {
    const metrics = health()?.metrics
    if (!metrics) return '--'
    const value = metrics[key]
    return typeof value === 'number' ? value : '--'
  }

  return (
    <div>
      <h1 class="text-2xl font-bold mb-6">仪表盘</h1>

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

      {/* Additional Stats Row */}
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mb-8">
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

      {/* Service Status */}
      <div class="card">
        <h2 class="text-lg font-semibold mb-4">服务状态</h2>
        <Show when={health()} fallback={<div class="text-dark-muted">加载中...</div>}>
          {(h) => (
            <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
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
    </div>
  )
}

export default Dashboard

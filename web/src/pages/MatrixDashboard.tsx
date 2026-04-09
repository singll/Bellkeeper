import { Component, createSignal, onMount, For, Show } from 'solid-js'
import { matrixApi } from '@/api'
import type { MatrixStats, MatrixEvent } from '@/types'

const MatrixDashboard: Component = () => {
  const [stats, setStats] = createSignal<MatrixStats | null>(null)
  const [events, setEvents] = createSignal<MatrixEvent[]>([])
  const [loading, setLoading] = createSignal(true)
  const [error, setError] = createSignal<string | null>(null)

  const loadData = async () => {
    setLoading(true)
    setError(null)
    try {
      const [statsRes, eventsRes] = await Promise.all([
        matrixApi.getStats(),
        matrixApi.listEvents({ page: 1, page_size: 10 }),
      ])
      setStats(statsRes)
      setEvents((eventsRes as any).data?.data || [])
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load data')
    } finally {
      setLoading(false)
    }
  }

  onMount(loadData)

  const formatTime = (time: string) => {
    const d = new Date(time)
    return d.toLocaleString('zh-CN', {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })
  }

  return (
    <div class="p-6">
      <h1 class="text-2xl font-bold mb-6">Matrix 平台总览</h1>

      <Show when={error()}>
        <div class="bg-red-500/10 border border-red-500 text-red-500 rounded p-4 mb-4">
          {error()}
        </div>
      </Show>

      <Show when={loading()}>
        <div class="flex items-center justify-center py-12">
          <div class="animate-spin w-8 h-8 border-2 border-primary border-t-transparent rounded-full"></div>
        </div>
      </Show>

      <Show when={!loading() && stats()}>
        {/* Stats Cards */}
        <div class="grid grid-cols-1 md:grid-cols-4 gap-4 mb-8">
          <div class="bg-card rounded-lg p-6 border border-border">
            <div class="text-sm text-muted-foreground mb-1">房间总数</div>
            <div class="text-3xl font-bold">{stats()?.rooms ?? 0}</div>
          </div>
          <div class="bg-card rounded-lg p-6 border border-border">
            <div class="text-sm text-muted-foreground mb-1">活跃频道</div>
            <div class="text-3xl font-bold">{stats()?.channels ?? 0}</div>
          </div>
          <div class="bg-card rounded-lg p-6 border border-border">
            <div class="text-sm text-muted-foreground mb-1">注册命令</div>
            <div class="text-3xl font-bold">{stats()?.commands ?? 0}</div>
          </div>
          <div class="bg-card rounded-lg p-6 border border-border">
            <div class="text-sm text-muted-foreground mb-1">24h 消息</div>
            <div class="text-3xl font-bold">{stats()?.notifications_24h ?? 0}</div>
          </div>
        </div>

        {/* Recent Events */}
        <div class="bg-card rounded-lg border border-border">
          <div class="px-6 py-4 border-b border-border">
            <h2 class="text-lg font-semibold">最近事件</h2>
          </div>
          <div class="divide-y divide-border">
            <For each={events()} fallback={
              <div class="px-6 py-8 text-center text-muted-foreground">
                暂无事件记录
              </div>
            }>
              {(event) => (
                <div class="px-6 py-3 flex items-center justify-between">
                  <div class="flex items-center gap-4">
                    <span class="text-sm text-muted-foreground">{formatTime(event.created_at)}</span>
                    <span class={`px-2 py-0.5 text-xs rounded ${
                      event.event_type === 'command'
                        ? 'bg-blue-500/20 text-blue-400'
                        : 'bg-green-500/20 text-green-400'
                    }`}>
                      {event.event_type}
                    </span>
                    <span class="text-sm">{event.content || event.sender}</span>
                  </div>
                  <span class="text-xs text-muted-foreground">{event.room_id}</span>
                </div>
              )}
            </For>
          </div>
        </div>

        {/* Quick Stats */}
        <div class="grid grid-cols-1 md:grid-cols-2 gap-4 mt-6">
          <div class="bg-card rounded-lg p-6 border border-border">
            <div class="text-sm text-muted-foreground mb-1">24h 事件数</div>
            <div class="text-2xl font-bold">{stats()?.events_24h ?? 0}</div>
          </div>
          <div class="bg-card rounded-lg p-6 border border-border">
            <div class="text-sm text-muted-foreground mb-1">活跃房间</div>
            <div class="text-2xl font-bold">{stats()?.active_rooms ?? 0}</div>
          </div>
        </div>
      </Show>
    </div>
  )
}

export default MatrixDashboard

import { Component, createSignal, onMount, For, Show } from 'solid-js'
import { matrixApi } from '@/api'
import type { MatrixEvent } from '@/types'
import Modal from '@/components/Modal'

const MatrixEvents: Component = () => {
  const [events, setEvents] = createSignal<MatrixEvent[]>([])
  const [loading, setLoading] = createSignal(true)
  const [error, setError] = createSignal<string | null>(null)
  const [page, setPage] = createSignal(1)
  const [total, setTotal] = createSignal(0)
  const [selected, setSelected] = createSignal<MatrixEvent | null>(null)

  const [filters, setFilters] = createSignal({
    event_type: '',
    room_id: '',
  })

  const loadData = async () => {
    setLoading(true)
    try {
      const res = await matrixApi.listEvents({ page: page(), page_size: 20, ...filters() })
      setEvents(res.data.data || [])
      setTotal(res.data.total || 0)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load data')
    } finally {
      setLoading(false)
    }
  }

  onMount(loadData)

  const formatTime = (time: string) => {
    return new Date(time).toLocaleString('zh-CN')
  }

  const totalPages = () => Math.ceil(total() / 20)

  return (
    <div class="p-6">
      <div class="flex items-center justify-between mb-6">
        <h1 class="text-2xl font-bold">事件日志</h1>
      </div>

      {/* Filters */}
      <div class="bg-card rounded-lg border border-border p-4 mb-6">
        <div class="flex items-center gap-4">
          <div>
            <label class="block text-sm mb-1">事件类型</label>
            <select
              value={filters().event_type}
              onChange={(e) => { setFilters({ ...filters(), event_type: e.currentTarget.value }); setPage(1) }}
              class="px-3 py-2 bg-background border border-border rounded"
            >
              <option value="">全部</option>
              <option value="command">命令</option>
              <option value="message">消息</option>
              <option value="member">成员</option>
            </select>
          </div>
          <div>
            <label class="block text-sm mb-1">房间 ID</label>
            <input
              type="text"
              value={filters().room_id}
              onInput={(e) => { setFilters({ ...filters(), room_id: e.currentTarget.value }); setPage(1) }}
              class="px-3 py-2 bg-background border border-border rounded"
              placeholder="!xxx:matrix.example.com"
            />
          </div>
          <button
            onClick={() => loadData()}
            class="px-4 py-2 bg-primary text-white rounded hover:bg-primary/80 mt-5"
          >
            搜索
          </button>
        </div>
      </div>

      <Show when={error()}>
        <div class="bg-red-500/10 border border-red-500 text-red-500 rounded p-4 mb-4">
          {error()}
        </div>
      </Show>

      <div class="bg-card rounded-lg border border-border overflow-hidden">
        <table class="w-full">
          <thead class="bg-muted">
            <tr>
              <th class="px-4 py-3 text-left text-sm font-medium">时间</th>
              <th class="px-4 py-3 text-left text-sm font-medium">类型</th>
              <th class="px-4 py-3 text-left text-sm font-medium">房间</th>
              <th class="px-4 py-3 text-left text-sm font-medium">用户</th>
              <th class="px-4 py-3 text-left text-sm font-medium">详情</th>
              <th class="px-4 py-3 text-right text-sm font-medium">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border">
            <Show when={loading()}>
              <tr>
                <td colspan="6" class="px-4 py-8 text-center text-muted-foreground">
                  加载中...
                </td>
              </tr>
            </Show>
            <For each={events()} fallback={
              <tr>
                <td colspan="6" class="px-4 py-8 text-center text-muted-foreground">
                  暂无事件记录
                </td>
              </tr>
            }>
              {(event) => (
                <tr class="hover:bg-muted/50">
                  <td class="px-4 py-3 text-sm text-muted-foreground">
                    {formatTime(event.created_at)}
                  </td>
                  <td class="px-4 py-3 text-sm">
                    <span class={`px-2 py-0.5 text-xs rounded ${
                      event.event_type === 'command'
                        ? 'bg-blue-500/20 text-blue-400'
                        : event.event_type === 'member'
                          ? 'bg-purple-500/20 text-purple-400'
                          : 'bg-green-500/20 text-green-400'
                    }`}>
                      {event.event_type}
                    </span>
                  </td>
                  <td class="px-4 py-3 text-sm font-mono">{event.room_id.slice(0, 20)}...</td>
                  <td class="px-4 py-3 text-sm">{event.sender}</td>
                  <td class="px-4 py-3 text-sm truncate max-w-xs">
                    {event.content || '-'}
                  </td>
                  <td class="px-4 py-3 text-right">
                    <button
                      onClick={() => setSelected(event)}
                      class="text-primary hover:underline"
                    >
                      查看
                    </button>
                  </td>
                </tr>
              )}
            </For>
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      <Show when={totalPages() > 1}>
        <div class="flex items-center justify-center gap-2 mt-4">
          <button
            onClick={() => { setPage(Math.max(1, page() - 1)); loadData() }}
            disabled={page() === 1}
            class="px-3 py-1 border border-border rounded disabled:opacity-50"
          >
            上一页
          </button>
          <span class="px-3 py-1">
            第 {page()} / {totalPages()} 页
          </span>
          <button
            onClick={() => { setPage(page() + 1); loadData() }}
            disabled={page() >= totalPages()}
            class="px-3 py-1 border border-border rounded disabled:opacity-50"
          >
            下一页
          </button>
        </div>
      </Show>

      {/* Detail Modal */}
      <Modal
        open={!!selected()}
        onClose={() => setSelected(null)}
        title="事件详情"
      >
        <Show when={selected()}>
          <div class="space-y-4">
            <div class="grid grid-cols-2 gap-4">
              <div>
                <div class="text-sm text-muted-foreground">ID</div>
                <div class="font-mono text-sm">{selected()!.id}</div>
              </div>
              <div>
                <div class="text-sm text-muted-foreground">类型</div>
                <div>{selected()!.event_type}</div>
              </div>
              <div>
                <div class="text-sm text-muted-foreground">房间</div>
                <div class="font-mono text-sm">{selected()!.room_id}</div>
              </div>
              <div>
                <div class="text-sm text-muted-foreground">发送者</div>
                <div>{selected()!.sender}</div>
              </div>
              <div>
                <div class="text-sm text-muted-foreground">处理状态</div>
                <div>{selected()!.processing_status || '-'}</div>
              </div>
              <div>
                <div class="text-sm text-muted-foreground">创建时间</div>
                <div class="text-sm">{formatTime(selected()!.created_at)}</div>
              </div>
            </div>
            <Show when={selected()!.content}>
              <div>
                <div class="text-sm text-muted-foreground mb-1">消息内容</div>
                <div class="bg-muted rounded p-3 text-sm whitespace-pre-wrap">
                  {selected()!.content}
                </div>
              </div>
            </Show>
            <Show when={selected()!.error_message}>
              <div>
                <div class="text-sm text-muted-foreground mb-1">错误信息</div>
                <div class="bg-red-500/10 border border-red-500 rounded p-3 text-sm text-red-400">
                  {selected()!.error_message}
                </div>
              </div>
            </Show>
          </div>
        </Show>
      </Modal>
    </div>
  )
}

export default MatrixEvents

import { Component, createSignal, onMount, For, Show } from 'solid-js'
import { matrixApi } from '@/api'
import type { MatrixNotification, MatrixChannel } from '@/types'
import Modal from '@/components/Modal'
import { useToast } from '@/components/Toast'

const MatrixNotifications: Component = () => {
  const { success: showSuccess, error: showError } = useToast()
  const [notifications, setNotifications] = createSignal<MatrixNotification[]>([])
  const [channels, setChannels] = createSignal<MatrixChannel[]>([])
  const [loading, setLoading] = createSignal(true)
  const [error, setError] = createSignal<string | null>(null)
  const [page, setPage] = createSignal(1)
  const [total, setTotal] = createSignal(0)
  const [selected, setSelected] = createSignal<MatrixNotification | null>(null)

  const [filters, setFilters] = createSignal({
    channel: '',
    status: '',
  })

  const loadData = async () => {
    setLoading(true)
    try {
      const [notifRes, channelRes] = await Promise.all([
        matrixApi.listNotifications({ page: page(), page_size: 20, ...filters() }),
        matrixApi.listChannels(),
      ])
      setNotifications(notifRes.data.data || [])
      setTotal(notifRes.data.total || 0)
      setChannels(channelRes.data || [])
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load data')
    } finally {
      setLoading(false)
    }
  }

  onMount(loadData)

  const handleRetry = async (id: number) => {
    try {
      await matrixApi.retryNotification(id)
      showSuccess('重试成功')
      loadData()
    } catch (e) {
      showError(e instanceof Error ? e.message : '重试失败')
    }
  }

  const formatTime = (time: string) => {
    return new Date(time).toLocaleString('zh-CN')
  }

  const totalPages = () => Math.ceil(total() / 20)

  return (
    <div class="p-6">
      <div class="flex items-center justify-between mb-6">
        <h1 class="text-2xl font-bold">通知管理</h1>
      </div>

      {/* Filters */}
      <div class="bg-card rounded-lg border border-border p-4 mb-6">
        <div class="flex items-center gap-4">
          <div>
            <label class="block text-sm mb-1">频道</label>
            <select
              value={filters().channel}
              onChange={(e) => { setFilters({ ...filters(), channel: e.currentTarget.value }); setPage(1) }}
              class="px-3 py-2 bg-background border border-border rounded"
            >
              <option value="">全部</option>
              <For each={channels()}>
                {(ch) => <option value={ch.name}>{ch.display_name}</option>}
              </For>
            </select>
          </div>
          <div>
            <label class="block text-sm mb-1">状态</label>
            <select
              value={filters().status}
              onChange={(e) => { setFilters({ ...filters(), status: e.currentTarget.value }); setPage(1) }}
              class="px-3 py-2 bg-background border border-border rounded"
            >
              <option value="">全部</option>
              <option value="pending">待发送</option>
              <option value="sent">已发送</option>
              <option value="failed">失败</option>
            </select>
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
              <th class="px-4 py-3 text-left text-sm font-medium">频道</th>
              <th class="px-4 py-3 text-left text-sm font-medium">类型</th>
              <th class="px-4 py-3 text-left text-sm font-medium">消息预览</th>
              <th class="px-4 py-3 text-left text-sm font-medium">状态</th>
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
            <For each={notifications()} fallback={
              <tr>
                <td colspan="6" class="px-4 py-8 text-center text-muted-foreground">
                  暂无通知记录
                </td>
              </tr>
            }>
              {(notif) => (
                <tr class="hover:bg-muted/50">
                  <td class="px-4 py-3 text-sm text-muted-foreground">
                    {formatTime(notif.created_at)}
                  </td>
                  <td class="px-4 py-3 text-sm">{notif.channel_name}</td>
                  <td class="px-4 py-3 text-sm">
                    <span class="px-2 py-0.5 text-xs rounded bg-blue-500/20 text-blue-400">
                      {notif.message_type}
                    </span>
                  </td>
                  <td class="px-4 py-3 text-sm truncate max-w-xs">
                    {notif.message_content.slice(0, 50)}...
                  </td>
                  <td class="px-4 py-3 text-sm">
                    <span class={`px-2 py-0.5 text-xs rounded ${
                      notif.status === 'sent'
                        ? 'bg-green-500/20 text-green-400'
                        : notif.status === 'failed'
                          ? 'bg-red-500/20 text-red-400'
                          : 'bg-yellow-500/20 text-yellow-400'
                    }`}>
                      {notif.status === 'sent' ? '成功' : notif.status === 'failed' ? '失败' : '待发送'}
                    </span>
                  </td>
                  <td class="px-4 py-3 text-right">
                    <button
                      onClick={() => setSelected(notif)}
                      class="text-primary hover:underline mr-3"
                    >
                      查看
                    </button>
                    <Show when={notif.status === 'failed'}>
                      <button
                        onClick={() => handleRetry(notif.id)}
                        class="text-blue-400 hover:underline"
                      >
                        重试
                      </button>
                    </Show>
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
        title="通知详情"
      >
        <Show when={selected()}>
          <div class="space-y-4">
            <div class="grid grid-cols-2 gap-4">
              <div>
                <div class="text-sm text-muted-foreground">ID</div>
                <div class="font-mono text-sm">{selected()!.id}</div>
              </div>
              <div>
                <div class="text-sm text-muted-foreground">频道</div>
                <div>{selected()!.channel_name}</div>
              </div>
              <div>
                <div class="text-sm text-muted-foreground">类型</div>
                <div>{selected()!.message_type}</div>
              </div>
              <div>
                <div class="text-sm text-muted-foreground">状态</div>
                <div>{selected()!.status === 'sent' ? '成功' : selected()!.status === 'failed' ? '失败' : '待发送'}</div>
              </div>
              <div>
                <div class="text-sm text-muted-foreground">创建时间</div>
                <div class="text-sm">{formatTime(selected()!.created_at)}</div>
              </div>
              <div>
                <div class="text-sm text-muted-foreground">重试次数</div>
                <div>{selected()!.retry_count}</div>
              </div>
            </div>
            <div>
              <div class="text-sm text-muted-foreground mb-1">消息内容</div>
              <div class="bg-muted rounded p-3 text-sm whitespace-pre-wrap">
                {selected()!.message_content}
              </div>
            </div>
            <Show when={selected()!.error_message}>
              <div>
                <div class="text-sm text-muted-foreground mb-1">错误信息</div>
                <div class="bg-red-500/10 border border-red-500 rounded p-3 text-sm text-red-400">
                  {selected()!.error_message}
                </div>
              </div>
            </Show>
            <Show when={selected()!.status === 'failed'}>
              <button
                onClick={() => { handleRetry(selected()!.id); setSelected(null) }}
                class="w-full px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600"
              >
                重试发送
              </button>
            </Show>
          </div>
        </Show>
      </Modal>
    </div>
  )
}

export default MatrixNotifications

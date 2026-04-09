import { Component, createSignal, onMount, For, Show } from 'solid-js'
import { matrixApi } from '@/api'
import type { MatrixCommandLog } from '@/types'
import Modal from '@/components/Modal'

const MatrixCommandLogs: Component = () => {
  const [logs, setLogs] = createSignal<MatrixCommandLog[]>([])
  const [loading, setLoading] = createSignal(true)
  const [error, setError] = createSignal<string | null>(null)
  const [page, setPage] = createSignal(1)
  const [total, setTotal] = createSignal(0)
  const [selected, setSelected] = createSignal<MatrixCommandLog | null>(null)

  const [filters, setFilters] = createSignal({
    command: '',
    status: '',
  })

  const loadData = async () => {
    setLoading(true)
    try {
      const res = await matrixApi.listCommandLogs({ page: page(), page_size: 20, ...filters() })
      setLogs(res.data.data || [])
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
        <h1 class="text-2xl font-bold">命令日志</h1>
      </div>

      {/* Filters */}
      <div class="bg-card rounded-lg border border-border p-4 mb-6">
        <div class="flex items-center gap-4">
          <div>
            <label class="block text-sm mb-1">命令</label>
            <input
              type="text"
              value={filters().command}
              onInput={(e) => { setFilters({ ...filters(), command: e.currentTarget.value }); setPage(1) }}
              class="px-3 py-2 bg-background border border-border rounded"
              placeholder="!help"
            />
          </div>
          <div>
            <label class="block text-sm mb-1">状态</label>
            <select
              value={filters().status}
              onChange={(e) => { setFilters({ ...filters(), status: e.currentTarget.value }); setPage(1) }}
              class="px-3 py-2 bg-background border border-border rounded"
            >
              <option value="">全部</option>
              <option value="success">成功</option>
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
              <th class="px-4 py-3 text-left text-sm font-medium">命令</th>
              <th class="px-4 py-3 text-left text-sm font-medium">用户</th>
              <th class="px-4 py-3 text-left text-sm font-medium">房间</th>
              <th class="px-4 py-3 text-left text-sm font-medium">状态</th>
              <th class="px-4 py-3 text-left text-sm font-medium">耗时</th>
              <th class="px-4 py-3 text-right text-sm font-medium">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border">
            <Show when={loading()}>
              <tr>
                <td colspan="7" class="px-4 py-8 text-center text-muted-foreground">
                  加载中...
                </td>
              </tr>
            </Show>
            <For each={logs()} fallback={
              <tr>
                <td colspan="7" class="px-4 py-8 text-center text-muted-foreground">
                  暂无命令日志
                </td>
              </tr>
            }>
              {(log) => (
                <tr class="hover:bg-muted/50">
                  <td class="px-4 py-3 text-sm text-muted-foreground">
                    {formatTime(log.created_at)}
                  </td>
                  <td class="px-4 py-3 text-sm font-mono">{log.command_name}</td>
                  <td class="px-4 py-3 text-sm">{log.user_id}</td>
                  <td class="px-4 py-3 text-sm font-mono">{log.room_id.slice(0, 20)}...</td>
                  <td class="px-4 py-3 text-sm">
                    <span class={`px-2 py-0.5 text-xs rounded ${
                      log.status === 'success'
                        ? 'bg-green-500/20 text-green-400'
                        : 'bg-red-500/20 text-red-400'
                    }`}>
                      {log.status === 'success' ? '成功' : '失败'}
                    </span>
                  </td>
                  <td class="px-4 py-3 text-sm">
                    {log.duration_ms < 1000
                      ? `${log.duration_ms}ms`
                      : `${(log.duration_ms / 1000).toFixed(1)}s`}
                  </td>
                  <td class="px-4 py-3 text-right">
                    <button
                      onClick={() => setSelected(log)}
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
        title="命令执行详情"
      >
        <Show when={selected()}>
          <div class="space-y-4">
            <div class="grid grid-cols-2 gap-4">
              <div>
                <div class="text-sm text-muted-foreground">ID</div>
                <div class="font-mono text-sm">{selected()!.id}</div>
              </div>
              <div>
                <div class="text-sm text-muted-foreground">命令</div>
                <div class="font-mono">{selected()!.command_name}</div>
              </div>
              <div>
                <div class="text-sm text-muted-foreground">用户</div>
                <div>{selected()!.user_id}</div>
              </div>
              <div>
                <div class="text-sm text-muted-foreground">房间</div>
                <div class="font-mono text-sm">{selected()!.room_id}</div>
              </div>
              <div>
                <div class="text-sm text-muted-foreground">状态</div>
                <div>
                  <span class={`px-2 py-0.5 text-xs rounded ${
                    selected()!.status === 'success'
                      ? 'bg-green-500/20 text-green-400'
                      : 'bg-red-500/20 text-red-400'
                  }`}>
                    {selected()!.status === 'success' ? '成功' : '失败'}
                  </span>
                </div>
              </div>
              <div>
                <div class="text-sm text-muted-foreground">耗时</div>
                <div>
                  {selected()!.duration_ms < 1000
                    ? `${selected()!.duration_ms}ms`
                    : `${(selected()!.duration_ms / 1000).toFixed(1)}s`}
                </div>
              </div>
              <div class="col-span-2">
                <div class="text-sm text-muted-foreground">执行时间</div>
                <div class="text-sm">{formatTime(selected()!.created_at)}</div>
              </div>
            </div>
            <Show when={selected()!.args}>
              <div>
                <div class="text-sm text-muted-foreground mb-1">参数</div>
                <div class="bg-muted rounded p-3 text-sm font-mono">
                  {selected()!.args}
                </div>
              </div>
            </Show>
            <Show when={selected()!.response}>
              <div>
                <div class="text-sm text-muted-foreground mb-1">响应内容</div>
                <div class="bg-muted rounded p-3 text-sm whitespace-pre-wrap">
                  {selected()!.response}
                </div>
              </div>
            </Show>
            <Show when={selected()!.error_message}>
              <div>
                <div class="text-sm text-muted-foreground mb-1">错误信息</div>
                <div class="bg-red-500/10 border border-red-500 rounded p-3 text-sm text-red-400 whitespace-pre-wrap">
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

export default MatrixCommandLogs

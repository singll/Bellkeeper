import { Component, createSignal, onMount, For, Show } from 'solid-js'
import { matrixApi } from '@/api'
import type { MatrixChannel, MatrixRoom } from '@/types'
import Modal from '@/components/Modal'
import { useToast } from '@/components/Toast'

const MatrixChannels: Component = () => {
  const { success: showSuccess, error: showError } = useToast()
  const [channels, setChannels] = createSignal<MatrixChannel[]>([])
  const [rooms, setRooms] = createSignal<MatrixRoom[]>([])
  const [loading, setLoading] = createSignal(true)
  const [error, setError] = createSignal<string | null>(null)
  const [showCreate, setShowCreate] = createSignal(false)
  const [editing, setEditing] = createSignal<MatrixChannel | null>(null)

  const [form, setForm] = createSignal({
    name: '',
    display_name: '',
    room_id: '',
    description: '',
  })

  const loadData = async () => {
    setLoading(true)
    try {
      const [channelsRes, roomsRes] = await Promise.all([
        matrixApi.listChannels(),
        matrixApi.listRooms({ page: 1, page_size: 100 }),
      ])
      setChannels(channelsRes.data || [])
      setRooms(roomsRes.data.data || [])
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load data')
    } finally {
      setLoading(false)
    }
  }

  onMount(loadData)

  const handleCreate = async () => {
    try {
      await matrixApi.createChannel(form())
      showSuccess('频道创建成功')
      setShowCreate(false)
      setForm({ name: '', display_name: '', room_id: '', description: '' })
      loadData()
    } catch (e) {
      showError(e instanceof Error ? e.message : '创建失败')
    }
  }

  const handleUpdate = async () => {
    const channel = editing()
    if (!channel) return
    try {
      await matrixApi.updateChannel(channel.id, form())
      showSuccess('频道更新成功')
      setEditing(null)
      loadData()
    } catch (e) {
      showError(e instanceof Error ? e.message : '更新失败')
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('确定要删除这个频道吗？')) return
    try {
      await matrixApi.deleteChannel(id)
      showSuccess('频道已删除')
      loadData()
    } catch (e) {
      showError(e instanceof Error ? e.message : '删除失败')
    }
  }

  const openEdit = (channel: MatrixChannel) => {
    setForm({
      name: channel.name,
      display_name: channel.display_name,
      room_id: channel.room_id,
      description: channel.description || '',
    })
    setEditing(channel)
  }

  return (
    <div class="p-6">
      <div class="flex items-center justify-between mb-6">
        <h1 class="text-2xl font-bold">频道管理</h1>
        <button
          onClick={() => setShowCreate(true)}
          class="px-4 py-2 bg-primary text-white rounded hover:bg-primary/80"
        >
          创建频道
        </button>
      </div>

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

      <Show when={!loading()}>
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          <For each={channels()} fallback={
            <div class="col-span-full text-center py-12 text-muted-foreground">
              暂无频道
            </div>
          }>
            {(channel) => (
              <div class="bg-card rounded-lg border border-border p-6">
                <div class="flex items-start justify-between mb-4">
                  <div>
                    <h3 class="font-semibold text-lg">{channel.display_name}</h3>
                    <p class="text-sm text-muted-foreground">{channel.name}</p>
                  </div>
                  <div class="flex gap-2">
                    <button
                      onClick={() => openEdit(channel)}
                      class="text-sm text-primary hover:underline"
                    >
                      编辑
                    </button>
                    <button
                      onClick={() => handleDelete(channel.id)}
                      class="text-sm text-red-500 hover:underline"
                    >
                      删除
                    </button>
                  </div>
                </div>
                <div class="space-y-2 text-sm">
                  <div class="flex items-center justify-between">
                    <span class="text-muted-foreground">绑定房间</span>
                    <span class="font-mono text-xs">{channel.room_id.slice(0, 15)}...</span>
                  </div>
                  <div class="flex items-center justify-between">
                    <span class="text-muted-foreground">消息数</span>
                    <span>{channel.message_count ?? 0}</span>
                  </div>
                </div>
                <Show when={channel.description}>
                  <p class="mt-4 text-sm text-muted-foreground">{channel.description}</p>
                </Show>
              </div>
            )}
          </For>
        </div>
      </Show>

      {/* Create Modal */}
      <Modal
        open={showCreate()}
        onClose={() => setShowCreate(false)}
        title="创建频道"
        onConfirm={handleCreate}
      >
        <div class="space-y-4">
          <div>
            <label class="block text-sm mb-1">频道名称</label>
            <input
              type="text"
              value={form().name}
              onInput={(e) => setForm({ ...form(), name: e.currentTarget.value })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
              placeholder="alerts"
            />
          </div>
          <div>
            <label class="block text-sm mb-1">显示名称</label>
            <input
              type="text"
              value={form().display_name}
              onInput={(e) => setForm({ ...form(), display_name: e.currentTarget.value })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
              placeholder="告警通知频道"
            />
          </div>
          <div>
            <label class="block text-sm mb-1">绑定房间</label>
            <select
              value={form().room_id}
              onChange={(e) => setForm({ ...form(), room_id: e.currentTarget.value })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
            >
              <option value="">选择房间</option>
              <For each={rooms()}>
                {(room) => (
                  <option value={room.room_id}>{room.name} ({room.room_id.slice(0, 15)}...)</option>
                )}
              </For>
            </select>
          </div>
          <div>
            <label class="block text-sm mb-1">描述</label>
            <textarea
              value={form().description}
              onInput={(e) => setForm({ ...form(), description: e.currentTarget.value })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
              rows={3}
            />
          </div>
        </div>
      </Modal>

      {/* Edit Modal */}
      <Modal
        open={!!editing()}
        onClose={() => setEditing(null)}
        title={`编辑频道: ${editing()?.name}`}
        onConfirm={handleUpdate}
      >
        <div class="space-y-4">
          <div>
            <label class="block text-sm mb-1">显示名称</label>
            <input
              type="text"
              value={form().display_name}
              onInput={(e) => setForm({ ...form(), display_name: e.currentTarget.value })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
            />
          </div>
          <div>
            <label class="block text-sm mb-1">绑定房间</label>
            <select
              value={form().room_id}
              onChange={(e) => setForm({ ...form(), room_id: e.currentTarget.value })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
            >
              <option value="">选择房间</option>
              <For each={rooms()}>
                {(room) => (
                  <option value={room.room_id}>{room.name}</option>
                )}
              </For>
            </select>
          </div>
          <div>
            <label class="block text-sm mb-1">描述</label>
            <textarea
              value={form().description}
              onInput={(e) => setForm({ ...form(), description: e.currentTarget.value })}
              class="w-full px-3 py-2 bg-background border border-border rounded"
              rows={3}
            />
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default MatrixChannels

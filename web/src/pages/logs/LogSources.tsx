import { Component, For, Show, createResource, createSignal } from 'solid-js'
import { logCenterApi } from '@/api'
import { formatDateTime, sourceTypeLabel } from './shared'
import type { LogSource } from './shared'

const LogSources: Component = () => {
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
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">日志源</h1>
          <p class="text-sm text-dark-400 mt-1">管理日志写入源的 API Key 和配置</p>
        </div>
        <button class="btn btn-primary btn-sm" onClick={() => { setShowCreate(true); setNewApiKey(null); setForm(createForm) }}>
          注册新源
        </button>
      </div>

      <div class="space-y-4">
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
    </div>
  )
}

export default LogSources

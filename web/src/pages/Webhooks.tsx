import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { webhooksApi } from '@/api'
import type { WebhookConfig, WebhookHistory } from '@/types'

const Webhooks: Component = () => {
  const [page, setPage] = createSignal(1)
  const [showModal, setShowModal] = createSignal(false)
  const [showHistoryModal, setShowHistoryModal] = createSignal(false)
  const [editing, setEditing] = createSignal<WebhookConfig | null>(null)
  const [selectedWebhook, setSelectedWebhook] = createSignal<number | null>(null)

  const [webhooks, { refetch }] = createResource(
    () => page(),
    (page) => webhooksApi.list(page, 20)
  )

  const [history] = createResource(
    () => selectedWebhook(),
    (id) => (id ? webhooksApi.history(id) : null)
  )

  const [form, setForm] = createSignal({
    name: '',
    url: '',
    method: 'POST',
    content_type: 'application/json',
    body_template: '',
    timeout_seconds: 30,
    description: '',
    is_active: true,
  })

  const openCreateModal = () => {
    setEditing(null)
    setForm({
      name: '',
      url: '',
      method: 'POST',
      content_type: 'application/json',
      body_template: '',
      timeout_seconds: 30,
      description: '',
      is_active: true,
    })
    setShowModal(true)
  }

  const openEditModal = (webhook: WebhookConfig) => {
    setEditing(webhook)
    setForm({
      name: webhook.name,
      url: webhook.url,
      method: webhook.method,
      content_type: webhook.content_type,
      body_template: webhook.body_template,
      timeout_seconds: webhook.timeout_seconds,
      description: webhook.description,
      is_active: webhook.is_active,
    })
    setShowModal(true)
  }

  const openHistoryModal = (id: number) => {
    setSelectedWebhook(id)
    setShowHistoryModal(true)
  }

  const handleSubmit = async (e: Event) => {
    e.preventDefault()
    try {
      if (editing()) {
        await webhooksApi.update(editing()!.id, form())
      } else {
        await webhooksApi.create(form())
      }
      setShowModal(false)
      refetch()
    } catch (err) {
      alert('操作失败: ' + (err as Error).message)
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('确定要删除这个 Webhook 吗？')) return
    try {
      await webhooksApi.delete(id)
      refetch()
    } catch (err) {
      alert('删除失败: ' + (err as Error).message)
    }
  }

  const handleTrigger = async (id: number) => {
    try {
      const result = await webhooksApi.trigger(id)
      alert(`触发成功！状态: ${result.data.status}`)
    } catch (err) {
      alert('触发失败: ' + (err as Error).message)
    }
  }

  return (
    <div>
      <div class="flex items-center justify-between mb-6">
        <h1 class="text-2xl font-bold">Webhook 管理</h1>
        <button class="btn btn-primary" onClick={openCreateModal}>
          新建 Webhook
        </button>
      </div>

      {/* Table */}
      <div class="card overflow-hidden">
        <table class="table">
          <thead>
            <tr>
              <th>名称</th>
              <th>URL</th>
              <th>方法</th>
              <th>超时</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <Show
              when={!webhooks.loading}
              fallback={
                <tr>
                  <td colspan="6" class="text-center py-8 text-dark-muted">加载中...</td>
                </tr>
              }
            >
              <For each={webhooks()?.data}>
                {(webhook) => (
                  <tr>
                    <td class="font-medium">{webhook.name}</td>
                    <td class="text-dark-muted truncate max-w-xs font-mono text-sm">{webhook.url}</td>
                    <td>
                      <span class="px-2 py-0.5 bg-primary-500/20 text-primary-400 rounded text-sm">
                        {webhook.method}
                      </span>
                    </td>
                    <td>{webhook.timeout_seconds}s</td>
                    <td>
                      <span class={webhook.is_active ? 'text-green-500' : 'text-dark-muted'}>
                        {webhook.is_active ? '启用' : '禁用'}
                      </span>
                    </td>
                    <td class="space-x-2">
                      <button
                        class="text-green-500 hover:text-green-400"
                        onClick={() => handleTrigger(webhook.id)}
                      >
                        触发
                      </button>
                      <button
                        class="text-dark-muted hover:text-dark-text"
                        onClick={() => openHistoryModal(webhook.id)}
                      >
                        历史
                      </button>
                      <button
                        class="text-primary-500 hover:text-primary-400"
                        onClick={() => openEditModal(webhook)}
                      >
                        编辑
                      </button>
                      <button
                        class="text-red-500 hover:text-red-400"
                        onClick={() => handleDelete(webhook.id)}
                      >
                        删除
                      </button>
                    </td>
                  </tr>
                )}
              </For>
            </Show>
          </tbody>
        </table>
      </div>

      {/* Create/Edit Modal */}
      <Show when={showModal()}>
        <div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div class="card w-full max-w-lg max-h-[90vh] overflow-y-auto">
            <h2 class="text-lg font-semibold mb-4">
              {editing() ? '编辑 Webhook' : '新建 Webhook'}
            </h2>
            <form onSubmit={handleSubmit}>
              <div class="space-y-4">
                <div>
                  <label class="label">名称</label>
                  <input
                    type="text"
                    class="input"
                    required
                    value={form().name}
                    onInput={(e) => setForm({ ...form(), name: e.currentTarget.value })}
                  />
                </div>
                <div>
                  <label class="label">URL</label>
                  <input
                    type="url"
                    class="input font-mono"
                    required
                    value={form().url}
                    onInput={(e) => setForm({ ...form(), url: e.currentTarget.value })}
                  />
                </div>
                <div class="grid grid-cols-3 gap-4">
                  <div>
                    <label class="label">方法</label>
                    <select
                      class="input"
                      value={form().method}
                      onChange={(e) => setForm({ ...form(), method: e.currentTarget.value })}
                    >
                      <option value="GET">GET</option>
                      <option value="POST">POST</option>
                      <option value="PUT">PUT</option>
                      <option value="DELETE">DELETE</option>
                    </select>
                  </div>
                  <div>
                    <label class="label">Content-Type</label>
                    <select
                      class="input"
                      value={form().content_type}
                      onChange={(e) => setForm({ ...form(), content_type: e.currentTarget.value })}
                    >
                      <option value="application/json">JSON</option>
                      <option value="application/x-www-form-urlencoded">Form</option>
                    </select>
                  </div>
                  <div>
                    <label class="label">超时 (秒)</label>
                    <input
                      type="number"
                      class="input"
                      min="1"
                      max="300"
                      value={form().timeout_seconds}
                      onInput={(e) => setForm({ ...form(), timeout_seconds: parseInt(e.currentTarget.value) })}
                    />
                  </div>
                </div>
                <div>
                  <label class="label">Body 模板 (JSON)</label>
                  <textarea
                    class="input font-mono text-sm"
                    rows="4"
                    placeholder='{"key": "value"}'
                    value={form().body_template}
                    onInput={(e) => setForm({ ...form(), body_template: e.currentTarget.value })}
                  />
                </div>
                <div>
                  <label class="label">描述</label>
                  <textarea
                    class="input"
                    rows="2"
                    value={form().description}
                    onInput={(e) => setForm({ ...form(), description: e.currentTarget.value })}
                  />
                </div>
                <div class="flex items-center gap-2">
                  <input
                    type="checkbox"
                    id="is_active"
                    checked={form().is_active}
                    onChange={(e) => setForm({ ...form(), is_active: e.currentTarget.checked })}
                  />
                  <label for="is_active">启用</label>
                </div>
              </div>
              <div class="flex justify-end gap-3 mt-6">
                <button type="button" class="btn btn-secondary" onClick={() => setShowModal(false)}>
                  取消
                </button>
                <button type="submit" class="btn btn-primary">
                  {editing() ? '保存' : '创建'}
                </button>
              </div>
            </form>
          </div>
        </div>
      </Show>

      {/* History Modal */}
      <Show when={showHistoryModal()}>
        <div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div class="card w-full max-w-2xl max-h-[90vh] overflow-y-auto">
            <div class="flex items-center justify-between mb-4">
              <h2 class="text-lg font-semibold">调用历史</h2>
              <button
                class="text-dark-muted hover:text-dark-text"
                onClick={() => setShowHistoryModal(false)}
              >
                关闭
              </button>
            </div>
            <Show when={history()} fallback={<div class="text-dark-muted">加载中...</div>}>
              <div class="space-y-3">
                <For each={history()?.data}>
                  {(h) => (
                    <div class="p-3 bg-dark-bg rounded-lg">
                      <div class="flex items-center justify-between mb-2">
                        <span class={h.status === 'success' ? 'text-green-500' : 'text-red-500'}>
                          {h.status === 'success' ? '成功' : '失败'}
                        </span>
                        <span class="text-dark-muted text-sm">
                          {new Date(h.created_at).toLocaleString()}
                        </span>
                      </div>
                      <div class="text-sm text-dark-muted">
                        {h.request_method} {h.request_url}
                      </div>
                      <div class="text-sm">
                        状态码: {h.response_code} | 耗时: {h.duration_ms}ms
                      </div>
                      {h.error_message && (
                        <div class="text-sm text-red-400 mt-1">{h.error_message}</div>
                      )}
                    </div>
                  )}
                </For>
              </div>
            </Show>
          </div>
        </div>
      </Show>
    </div>
  )
}

export default Webhooks

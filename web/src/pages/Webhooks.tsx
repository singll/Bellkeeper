import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { webhooksApi } from '@/api'
import { useToast } from '@/components/Toast'
import Modal from '@/components/Modal'
import type { WebhookConfig, WebhookHistory } from '@/types'

const Webhooks: Component = () => {
  const toast = useToast()
  const [page, setPage] = createSignal(1)
  const [showModal, setShowModal] = createSignal(false)
  const [showHistoryModal, setShowHistoryModal] = createSignal(false)
  const [editing, setEditing] = createSignal<WebhookConfig | null>(null)
  const [selectedWebhook, setSelectedWebhook] = createSignal<WebhookConfig | null>(null)
  const [submitting, setSubmitting] = createSignal(false)
  const [triggering, setTriggering] = createSignal<number | null>(null)

  const [webhooks, { refetch }] = createResource(
    () => page(),
    (page) => webhooksApi.list(page, 20)
  )

  const [history] = createResource(
    () => selectedWebhook()?.id,
    (id) => (id ? webhooksApi.history(id) : null)
  )

  const [form, setForm] = createSignal({
    name: '',
    url: '',
    method: 'POST',
    content_type: 'application/json',
    headers: '{}',
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
      headers: '{}',
      body_template: '',
      timeout_seconds: 30,
      description: '',
      is_active: true,
    })
    setShowModal(true)
  }

  const openEditModal = (webhook: WebhookConfig) => {
    setEditing(webhook)
    let headersStr = '{}'
    try {
      headersStr = webhook.headers ? JSON.stringify(webhook.headers, null, 2) : '{}'
    } catch { headersStr = '{}' }
    setForm({
      name: webhook.name,
      url: webhook.url,
      method: webhook.method,
      content_type: webhook.content_type,
      headers: headersStr,
      body_template: webhook.body_template,
      timeout_seconds: webhook.timeout_seconds,
      description: webhook.description,
      is_active: webhook.is_active,
    })
    setShowModal(true)
  }

  const openHistoryModal = (webhook: WebhookConfig) => {
    setSelectedWebhook(webhook)
    setShowHistoryModal(true)
  }

  const handleSubmit = async (e: Event) => {
    e.preventDefault()
    setSubmitting(true)
    try {
      let headers = {}
      try {
        headers = JSON.parse(form().headers || '{}')
      } catch {
        toast.error('Headers JSON 格式无效')
        setSubmitting(false)
        return
      }
      const payload = { ...form(), headers }
      if (editing()) {
        await webhooksApi.update(editing()!.id, payload)
        toast.success('Webhook 更新成功')
      } else {
        await webhooksApi.create(payload)
        toast.success('Webhook 创建成功')
      }
      setShowModal(false)
      refetch()
    } catch (err) {
      toast.error('操作失败: ' + (err as Error).message)
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (webhook: WebhookConfig) => {
    if (!confirm(`确定要删除 Webhook "${webhook.name}"吗？`)) return
    try {
      await webhooksApi.delete(webhook.id)
      toast.success('Webhook 删除成功')
      refetch()
    } catch (err) {
      toast.error('删除失败: ' + (err as Error).message)
    }
  }

  const handleTrigger = async (webhook: WebhookConfig) => {
    setTriggering(webhook.id)
    try {
      const result = await webhooksApi.trigger(webhook.id)
      if (result.data.status === 'success') {
        toast.success(`触发成功，响应码: ${result.data.response_code}`)
      } else {
        toast.warning(`触发完成但返回错误: ${result.data.error_message}`)
      }
    } catch (err) {
      toast.error('触发失败: ' + (err as Error).message)
    } finally {
      setTriggering(null)
    }
  }

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">Webhook 管理</h1>
          <p class="text-sm text-dark-400 mt-1">管理外部服务的 Webhook 调用配置</p>
        </div>
        <button class="btn btn-primary" onClick={openCreateModal}>
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
          </svg>
          新建 Webhook
        </button>
      </div>

      {/* Table */}
      <div class="card overflow-hidden p-0">
        <div class="overflow-x-auto">
          <table class="table">
            <thead>
              <tr>
                <th>名称</th>
                <th>URL</th>
                <th>方法</th>
                <th>超时</th>
                <th>状态</th>
                <th class="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              <Show
                when={!webhooks.loading}
                fallback={
                  <tr>
                    <td colspan="6" class="text-center py-12">
                      <div class="loading-spinner mx-auto" />
                      <p class="mt-3 text-dark-400">加载中...</p>
                    </td>
                  </tr>
                }
              >
                <Show
                  when={webhooks()?.data && webhooks()!.data.length > 0}
                  fallback={
                    <tr>
                      <td colspan="6">
                        <div class="empty-state">
                          <svg class="empty-state-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
                          </svg>
                          <p class="empty-state-title">暂无 Webhook</p>
                          <p class="empty-state-description">点击"新建 Webhook"添加第一个配置</p>
                        </div>
                      </td>
                    </tr>
                  }
                >
                  <For each={webhooks()?.data}>
                    {(webhook) => (
                      <tr class="group">
                        <td>
                          <div class="flex flex-col">
                            <span class="font-medium text-white">{webhook.name}</span>
                            <Show when={webhook.description}>
                              <span class="text-xs text-dark-500 mt-0.5">{webhook.description}</span>
                            </Show>
                          </div>
                        </td>
                        <td>
                          <code class="text-dark-400 font-mono text-sm truncate max-w-[250px] block">
                            {webhook.url}
                          </code>
                        </td>
                        <td>
                          <span class="badge badge-primary">{webhook.method}</span>
                        </td>
                        <td>
                          <span class="text-dark-400">{webhook.timeout_seconds}s</span>
                        </td>
                        <td>
                          <div class="flex items-center gap-2">
                            <span class={`status-dot ${webhook.is_active ? 'status-dot-success' : 'status-dot-gray'}`} />
                            <span class={webhook.is_active ? 'text-emerald-400' : 'text-dark-500'}>
                              {webhook.is_active ? '启用' : '禁用'}
                            </span>
                          </div>
                        </td>
                        <td class="text-right">
                          <div class="flex items-center justify-end gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                            <button
                              class="btn btn-ghost btn-sm text-emerald-400 hover:text-emerald-300 hover:bg-emerald-500/10"
                              onClick={() => handleTrigger(webhook)}
                              disabled={triggering() === webhook.id}
                            >
                              {triggering() === webhook.id ? (
                                <div class="loading-spinner w-4 h-4" />
                              ) : (
                                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                                </svg>
                              )}
                            </button>
                            <button
                              class="btn btn-ghost btn-sm"
                              onClick={() => openHistoryModal(webhook)}
                            >
                              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                              </svg>
                            </button>
                            <button
                              class="btn btn-ghost btn-sm"
                              onClick={() => openEditModal(webhook)}
                            >
                              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                              </svg>
                            </button>
                            <button
                              class="btn btn-ghost btn-sm text-red-400 hover:text-red-300 hover:bg-red-500/10"
                              onClick={() => handleDelete(webhook)}
                            >
                              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                              </svg>
                            </button>
                          </div>
                        </td>
                      </tr>
                    )}
                  </For>
                </Show>
              </Show>
            </tbody>
          </table>
        </div>
      </div>

      {/* Pagination */}
      <Show when={webhooks() && webhooks()!.total > 20}>
        <div class="flex items-center justify-between mt-4">
          <div class="text-sm text-dark-400">
            共 <span class="text-dark-200 font-medium">{webhooks()!.total}</span> 条记录
          </div>
          <div class="flex gap-2">
            <button
              class="btn btn-secondary btn-sm"
              disabled={page() === 1}
              onClick={() => setPage((p) => p - 1)}
            >
              上一页
            </button>
            <span class="btn btn-ghost btn-sm cursor-default">
              {page()} / {Math.ceil((webhooks()?.total || 0) / 20)}
            </span>
            <button
              class="btn btn-secondary btn-sm"
              disabled={page() * 20 >= (webhooks()?.total || 0)}
              onClick={() => setPage((p) => p + 1)}
            >
              下一页
            </button>
          </div>
        </div>
      </Show>

      {/* Create/Edit Modal */}
      <Modal
        open={showModal()}
        onClose={() => setShowModal(false)}
        title={editing() ? '编辑 Webhook' : '新建 Webhook'}
        size="lg"
        footer={
          <>
            <button type="button" class="btn btn-secondary" onClick={() => setShowModal(false)}>
              取消
            </button>
            <button
              type="submit"
              form="webhook-form"
              class="btn btn-primary"
              disabled={submitting()}
            >
              {submitting() ? (
                <>
                  <div class="loading-spinner" />
                  处理中...
                </>
              ) : editing() ? '保存' : '创建'}
            </button>
          </>
        }
      >
        <form id="webhook-form" onSubmit={handleSubmit} class="space-y-4">
          <div>
            <label class="label">名称 *</label>
            <input
              type="text"
              class="input"
              required
              placeholder="输入 Webhook 名称"
              value={form().name}
              onInput={(e) => setForm({ ...form(), name: e.currentTarget.value })}
            />
          </div>
          <div>
            <label class="label">URL *</label>
            <input
              type="url"
              class="input font-mono"
              required
              placeholder="https://api.example.com/webhook"
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
              <label class="label">超时</label>
              <div class="relative">
                <input
                  type="number"
                  class="input pr-8"
                  min="1"
                  max="300"
                  value={form().timeout_seconds}
                  onInput={(e) => setForm({ ...form(), timeout_seconds: parseInt(e.currentTarget.value) || 30 })}
                />
                <span class="absolute right-3 top-1/2 -translate-y-1/2 text-dark-500 text-sm">秒</span>
              </div>
            </div>
          </div>
          <div>
            <label class="label">Body 模板</label>
            <textarea
              class="input font-mono text-sm resize-none"
              rows="4"
              placeholder='{"key": "value"}'
              value={form().body_template}
              onInput={(e) => setForm({ ...form(), body_template: e.currentTarget.value })}
            />
            <p class="text-xs text-dark-500 mt-1">JSON 格式，留空使用默认值</p>
          </div>
          <div>
            <label class="label">自定义 Headers</label>
            <textarea
              class="input font-mono text-sm resize-none"
              rows="3"
              placeholder='{"Authorization": "Bearer xxx"}'
              value={form().headers}
              onInput={(e) => setForm({ ...form(), headers: e.currentTarget.value })}
            />
            <p class="text-xs text-dark-500 mt-1">JSON 格式的 HTTP 请求头</p>
          </div>
          <div>
            <label class="label">描述</label>
            <textarea
              class="input resize-none"
              rows="2"
              placeholder="输入描述（可选）"
              value={form().description}
              onInput={(e) => setForm({ ...form(), description: e.currentTarget.value })}
            />
          </div>
          <div class="flex items-center gap-3 pt-2">
            <label class="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                class="sr-only peer"
                checked={form().is_active}
                onChange={(e) => setForm({ ...form(), is_active: e.currentTarget.checked })}
              />
              <div class="w-11 h-6 bg-dark-700 peer-focus:outline-none peer-focus:ring-2 peer-focus:ring-primary-500 rounded-full peer peer-checked:after:translate-x-full rtl:peer-checked:after:-translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:start-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary-600"></div>
              <span class="ms-3 text-sm font-medium text-dark-300">启用</span>
            </label>
          </div>
        </form>
      </Modal>

      {/* History Modal */}
      <Modal
        open={showHistoryModal()}
        onClose={() => setShowHistoryModal(false)}
        title={`调用历史 - ${selectedWebhook()?.name || ''}`}
        size="xl"
      >
        <Show when={history()} fallback={
          <div class="flex items-center justify-center py-8">
            <div class="loading-spinner" />
            <span class="ml-3 text-dark-400">加载中...</span>
          </div>
        }>
          <Show
            when={history()?.data && history()!.data.length > 0}
            fallback={
              <div class="empty-state py-8">
                <svg class="empty-state-icon w-12 h-12" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                <p class="empty-state-title">暂无调用记录</p>
              </div>
            }
          >
            <div class="space-y-3 max-h-96 overflow-y-auto">
              <For each={history()?.data}>
                {(h) => (
                  <div class="p-4 bg-dark-700/50 rounded-xl border border-dark-600/50">
                    <div class="flex items-center justify-between mb-2">
                      <div class="flex items-center gap-2">
                        <span class={`status-dot ${h.status === 'success' ? 'status-dot-success' : 'status-dot-danger'}`} />
                        <span class={h.status === 'success' ? 'text-emerald-400 font-medium' : 'text-red-400 font-medium'}>
                          {h.status === 'success' ? '成功' : '失败'}
                        </span>
                        <span class="badge badge-gray">{h.response_code}</span>
                      </div>
                      <span class="text-dark-500 text-sm">
                        {new Date(h.created_at).toLocaleString('zh-CN')}
                      </span>
                    </div>
                    <div class="text-sm text-dark-400 font-mono mb-2">
                      {h.request_method} {h.request_url}
                    </div>
                    <div class="flex items-center gap-4 text-sm">
                      <span class="text-dark-500">耗时: <span class="text-dark-300">{h.duration_ms}ms</span></span>
                    </div>
                    <Show when={h.error_message}>
                      <div class="mt-2 text-sm text-red-400 bg-red-500/10 px-3 py-2 rounded-lg">
                        {h.error_message}
                      </div>
                    </Show>
                  </div>
                )}
              </For>
            </div>
          </Show>
        </Show>
      </Modal>
    </div>
  )
}

export default Webhooks

import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { datasetsApi, tagsApi } from '@/api'
import { useToast } from '@/components/Toast'
import Modal from '@/components/Modal'
import type { DatasetMapping } from '@/types'

const Datasets: Component = () => {
  const toast = useToast()
  const [page, setPage] = createSignal(1)
  const [showModal, setShowModal] = createSignal(false)
  const [editing, setEditing] = createSignal<DatasetMapping | null>(null)
  const [submitting, setSubmitting] = createSignal(false)

  const [datasets, { refetch }] = createResource(
    () => page(),
    (page) => datasetsApi.list(page, 20)
  )

  const [allTags] = createResource(() => tagsApi.list(1, 100))

  const [form, setForm] = createSignal({
    name: '',
    display_name: '',
    dataset_id: '',
    description: '',
    is_default: false,
    is_active: true,
    parser_id: 'naive',
    tag_ids: [] as number[],
  })

  const openCreateModal = () => {
    setEditing(null)
    setForm({
      name: '',
      display_name: '',
      dataset_id: '',
      description: '',
      is_default: false,
      is_active: true,
      parser_id: 'naive',
      tag_ids: [],
    })
    setShowModal(true)
  }

  const openEditModal = (dataset: DatasetMapping) => {
    setEditing(dataset)
    setForm({
      name: dataset.name,
      display_name: dataset.display_name,
      dataset_id: dataset.dataset_id,
      description: dataset.description,
      is_default: dataset.is_default,
      is_active: dataset.is_active,
      parser_id: dataset.parser_id,
      tag_ids: dataset.tags?.map((t) => t.id) || [],
    })
    setShowModal(true)
  }

  const handleSubmit = async (e: Event) => {
    e.preventDefault()
    setSubmitting(true)
    try {
      if (editing()) {
        await datasetsApi.update(editing()!.id, form())
        toast.success('知识库映射更新成功')
      } else {
        await datasetsApi.create(form())
        toast.success('知识库映射创建成功')
      }
      setShowModal(false)
      refetch()
    } catch (err) {
      toast.error('操作失败: ' + (err as Error).message)
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (dataset: DatasetMapping) => {
    if (!confirm(`确定要删除知识库映射"${dataset.name}"吗？`)) return
    try {
      await datasetsApi.delete(dataset.id)
      toast.success('知识库映射删除成功')
      refetch()
    } catch (err) {
      toast.error('删除失败: ' + (err as Error).message)
    }
  }

  const toggleTag = (tagId: number) => {
    const current = form().tag_ids
    if (current.includes(tagId)) {
      setForm({ ...form(), tag_ids: current.filter((id) => id !== tagId) })
    } else {
      setForm({ ...form(), tag_ids: [...current, tagId] })
    }
  }

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">知识库映射</h1>
          <p class="text-sm text-dark-400 mt-1">管理 RagFlow 知识库与标签的映射关系</p>
        </div>
        <button class="btn btn-primary" onClick={openCreateModal}>
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
          </svg>
          新建映射
        </button>
      </div>

      {/* Info Card */}
      <div class="card mb-6 bg-primary-500/10 border-primary-500/30">
        <div class="flex items-start gap-3">
          <svg class="w-5 h-5 text-primary-400 mt-0.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <div class="text-sm">
            <p class="text-primary-300 font-medium">知识库路由说明</p>
            <p class="text-dark-400 mt-1">文档上传时，系统会根据标签自动路由到对应的 RagFlow 知识库。默认知识库用于未匹配任何标签的文档。</p>
          </div>
        </div>
      </div>

      {/* Table */}
      <div class="card overflow-hidden p-0">
        <div class="overflow-x-auto">
          <table class="table">
            <thead>
              <tr>
                <th>名称</th>
                <th>Dataset ID</th>
                <th>关联标签</th>
                <th>解析器</th>
                <th>状态</th>
                <th class="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              <Show
                when={!datasets.loading}
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
                  when={datasets()?.data && datasets()!.data.length > 0}
                  fallback={
                    <tr>
                      <td colspan="6">
                        <div class="empty-state">
                          <svg class="empty-state-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
                          </svg>
                          <p class="empty-state-title">暂无知识库映射</p>
                          <p class="empty-state-description">点击"新建映射"添加第一个映射</p>
                        </div>
                      </td>
                    </tr>
                  }
                >
                  <For each={datasets()?.data}>
                    {(dataset) => (
                      <tr class="group">
                        <td>
                          <div class="flex items-center gap-2">
                            <span class="font-medium text-white">{dataset.display_name || dataset.name}</span>
                            <Show when={dataset.is_default}>
                              <span class="badge badge-primary">默认</span>
                            </Show>
                          </div>
                          <p class="text-xs text-dark-500 mt-0.5 font-mono">{dataset.name}</p>
                        </td>
                        <td>
                          <code class="text-dark-400 font-mono text-sm bg-dark-800/50 px-2 py-1 rounded">
                            {dataset.dataset_id.slice(0, 12)}...
                          </code>
                        </td>
                        <td>
                          <div class="flex flex-wrap gap-1">
                            <For each={dataset.tags}>
                              {(tag) => (
                                <span
                                  class="px-2 py-0.5 rounded-md text-xs font-medium"
                                  style={{ "background-color": tag.color + '20', color: tag.color }}
                                >
                                  {tag.name}
                                </span>
                              )}
                            </For>
                            <Show when={!dataset.tags || dataset.tags.length === 0}>
                              <span class="text-dark-600">-</span>
                            </Show>
                          </div>
                        </td>
                        <td>
                          <span class="badge badge-gray">{dataset.parser_id}</span>
                        </td>
                        <td>
                          <div class="flex items-center gap-2">
                            <span class={`status-dot ${dataset.is_active ? 'status-dot-success' : 'status-dot-gray'}`} />
                            <span class={dataset.is_active ? 'text-emerald-400' : 'text-dark-500'}>
                              {dataset.is_active ? '启用' : '禁用'}
                            </span>
                          </div>
                        </td>
                        <td class="text-right">
                          <div class="flex items-center justify-end gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                            <button
                              class="btn btn-ghost btn-sm"
                              onClick={() => openEditModal(dataset)}
                            >
                              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                              </svg>
                            </button>
                            <button
                              class="btn btn-ghost btn-sm text-red-400 hover:text-red-300 hover:bg-red-500/10"
                              onClick={() => handleDelete(dataset)}
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
      <Show when={datasets() && datasets()!.total > 20}>
        <div class="flex items-center justify-between mt-4">
          <div class="text-sm text-dark-400">
            共 <span class="text-dark-200 font-medium">{datasets()!.total}</span> 条记录
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
              {page()} / {Math.ceil((datasets()?.total || 0) / 20)}
            </span>
            <button
              class="btn btn-secondary btn-sm"
              disabled={page() * 20 >= (datasets()?.total || 0)}
              onClick={() => setPage((p) => p + 1)}
            >
              下一页
            </button>
          </div>
        </div>
      </Show>

      {/* Modal */}
      <Modal
        open={showModal()}
        onClose={() => setShowModal(false)}
        title={editing() ? '编辑知识库映射' : '新建知识库映射'}
        size="lg"
        footer={
          <>
            <button type="button" class="btn btn-secondary" onClick={() => setShowModal(false)}>
              取消
            </button>
            <button
              type="submit"
              form="dataset-form"
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
        <form id="dataset-form" onSubmit={handleSubmit} class="space-y-4">
          <div class="grid grid-cols-2 gap-4">
            <div>
              <label class="label">映射名称 *</label>
              <input
                type="text"
                class="input font-mono"
                required
                placeholder="security"
                value={form().name}
                onInput={(e) => setForm({ ...form(), name: e.currentTarget.value })}
              />
              <p class="text-xs text-dark-500 mt-1">用于 API 调用，建议使用英文</p>
            </div>
            <div>
              <label class="label">显示名称</label>
              <input
                type="text"
                class="input"
                placeholder="安全知识库"
                value={form().display_name}
                onInput={(e) => setForm({ ...form(), display_name: e.currentTarget.value })}
              />
            </div>
          </div>
          <div>
            <label class="label">RagFlow Dataset ID *</label>
            <input
              type="text"
              class="input font-mono"
              required
              placeholder="abc123def456..."
              value={form().dataset_id}
              onInput={(e) => setForm({ ...form(), dataset_id: e.currentTarget.value })}
            />
            <p class="text-xs text-dark-500 mt-1">从 RagFlow 控制台复制知识库 ID</p>
          </div>
          <div>
            <label class="label">解析器</label>
            <select
              class="input"
              value={form().parser_id}
              onChange={(e) => setForm({ ...form(), parser_id: e.currentTarget.value })}
            >
              <option value="naive">Naive - 通用解析</option>
              <option value="paper">Paper - 论文解析</option>
              <option value="laws">Laws - 法律文档</option>
              <option value="qa">QA - 问答对</option>
              <option value="table">Table - 表格数据</option>
            </select>
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
          <div>
            <label class="label">关联标签</label>
            <p class="text-xs text-dark-500 mb-2">选择标签后，带有这些标签的文档将自动路由到此知识库</p>
            <div class="flex flex-wrap gap-2 p-3 bg-dark-800/50 rounded-xl border border-dark-700/50">
              <Show when={allTags()?.data && allTags()!.data.length > 0} fallback={
                <span class="text-dark-500 text-sm">暂无标签，请先创建标签</span>
              }>
                <For each={allTags()?.data}>
                  {(tag) => (
                    <button
                      type="button"
                      class="px-3 py-1.5 rounded-lg text-sm font-medium transition-all"
                      classList={{
                        'ring-2 ring-white ring-offset-2 ring-offset-dark-900': form().tag_ids.includes(tag.id),
                      }}
                      style={{ "background-color": tag.color + '20', color: tag.color }}
                      onClick={() => toggleTag(tag.id)}
                    >
                      {tag.name}
                    </button>
                  )}
                </For>
              </Show>
            </div>
          </div>
          <div class="flex items-center gap-6 pt-2">
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
            <label class="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                class="sr-only peer"
                checked={form().is_default}
                onChange={(e) => setForm({ ...form(), is_default: e.currentTarget.checked })}
              />
              <div class="w-11 h-6 bg-dark-700 peer-focus:outline-none peer-focus:ring-2 peer-focus:ring-primary-500 rounded-full peer peer-checked:after:translate-x-full rtl:peer-checked:after:-translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:start-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary-600"></div>
              <span class="ms-3 text-sm font-medium text-dark-300">设为默认</span>
            </label>
          </div>
        </form>
      </Modal>
    </div>
  )
}

export default Datasets

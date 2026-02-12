import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { tagsApi } from '@/api'
import { useToast } from '@/components/Toast'
import Modal from '@/components/Modal'
import type { Tag } from '@/types'

const Tags: Component = () => {
  const toast = useToast()
  const [page, setPage] = createSignal(1)
  const [keyword, setKeyword] = createSignal('')
  const [showModal, setShowModal] = createSignal(false)
  const [editingTag, setEditingTag] = createSignal<Tag | null>(null)
  const [submitting, setSubmitting] = createSignal(false)

  const [tags, { refetch }] = createResource(
    () => ({ page: page(), keyword: keyword() }),
    ({ page, keyword }) => tagsApi.list(page, 20, keyword)
  )

  const [form, setForm] = createSignal({
    name: '',
    description: '',
    color: '#6366f1',
  })

  const openCreateModal = () => {
    setEditingTag(null)
    setForm({ name: '', description: '', color: '#6366f1' })
    setShowModal(true)
  }

  const openEditModal = (tag: Tag) => {
    setEditingTag(tag)
    setForm({ name: tag.name, description: tag.description, color: tag.color })
    setShowModal(true)
  }

  const handleSubmit = async (e: Event) => {
    e.preventDefault()
    setSubmitting(true)
    try {
      if (editingTag()) {
        await tagsApi.update(editingTag()!.id, form())
        toast.success('标签更新成功')
      } else {
        await tagsApi.create(form())
        toast.success('标签创建成功')
      }
      setShowModal(false)
      refetch()
    } catch (err) {
      toast.error('操作失败: ' + (err as Error).message)
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (tag: Tag) => {
    if (!confirm(`确定要删除标签"${tag.name}"吗？`)) return
    try {
      await tagsApi.delete(tag.id)
      toast.success('标签删除成功')
      refetch()
    } catch (err) {
      toast.error('删除失败: ' + (err as Error).message)
    }
  }

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">标签管理</h1>
          <p class="text-sm text-dark-400 mt-1">管理知识库文档的分类标签</p>
        </div>
        <button class="btn btn-primary" onClick={openCreateModal}>
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
          </svg>
          新建标签
        </button>
      </div>

      {/* Search */}
      <div class="card mb-6">
        <div class="flex items-center gap-3">
          <div class="relative flex-1">
            <svg class="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-dark-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
            <input
              type="text"
              class="input pl-10"
              placeholder="搜索标签名称..."
              value={keyword()}
              onInput={(e) => {
                setKeyword(e.currentTarget.value)
                setPage(1)
              }}
            />
          </div>
          <Show when={keyword()}>
            <button class="btn btn-ghost btn-sm" onClick={() => setKeyword('')}>
              清除
            </button>
          </Show>
        </div>
      </div>

      {/* Table */}
      <div class="card overflow-hidden p-0">
        <div class="overflow-x-auto">
          <table class="table">
            <thead>
              <tr>
                <th>标签</th>
                <th>描述</th>
                <th>创建时间</th>
                <th class="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              <Show
                when={!tags.loading}
                fallback={
                  <tr>
                    <td colspan="4" class="text-center py-12">
                      <div class="loading-spinner mx-auto" />
                      <p class="mt-3 text-dark-400">加载中...</p>
                    </td>
                  </tr>
                }
              >
                <Show
                  when={tags()?.data && tags()!.data.length > 0}
                  fallback={
                    <tr>
                      <td colspan="4">
                        <div class="empty-state">
                          <svg class="empty-state-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z" />
                          </svg>
                          <p class="empty-state-title">暂无标签</p>
                          <p class="empty-state-description">点击"新建标签"创建第一个标签</p>
                        </div>
                      </td>
                    </tr>
                  }
                >
                  <For each={tags()?.data}>
                    {(tag) => (
                      <tr class="group">
                        <td>
                          <div class="flex items-center gap-3">
                            <div
                              class="w-3 h-3 rounded-full shadow-sm"
                              style={{ "background-color": tag.color, "box-shadow": `0 0 8px ${tag.color}40` }}
                            />
                            <span
                              class="px-3 py-1.5 rounded-lg text-sm font-medium"
                              style={{ "background-color": tag.color + '20', color: tag.color }}
                            >
                              {tag.name}
                            </span>
                          </div>
                        </td>
                        <td class="text-dark-400 max-w-xs truncate">
                          {tag.description || <span class="text-dark-600">-</span>}
                        </td>
                        <td class="text-dark-400 text-sm">
                          {new Date(tag.created_at).toLocaleDateString('zh-CN')}
                        </td>
                        <td class="text-right">
                          <div class="flex items-center justify-end gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                            <button
                              class="btn btn-ghost btn-sm"
                              onClick={() => openEditModal(tag)}
                            >
                              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                              </svg>
                            </button>
                            <button
                              class="btn btn-ghost btn-sm text-red-400 hover:text-red-300 hover:bg-red-500/10"
                              onClick={() => handleDelete(tag)}
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
      <Show when={tags() && tags()!.total > 20}>
        <div class="flex items-center justify-between mt-4">
          <div class="text-sm text-dark-400">
            共 <span class="text-dark-200 font-medium">{tags()!.total}</span> 条记录
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
              {page()} / {Math.ceil((tags()?.total || 0) / 20)}
            </span>
            <button
              class="btn btn-secondary btn-sm"
              disabled={page() * 20 >= (tags()?.total || 0)}
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
        title={editingTag() ? '编辑标签' : '新建标签'}
        footer={
          <>
            <button type="button" class="btn btn-secondary" onClick={() => setShowModal(false)}>
              取消
            </button>
            <button
              type="submit"
              form="tag-form"
              class="btn btn-primary"
              disabled={submitting()}
            >
              {submitting() ? (
                <>
                  <div class="loading-spinner" />
                  处理中...
                </>
              ) : editingTag() ? '保存' : '创建'}
            </button>
          </>
        }
      >
        <form id="tag-form" onSubmit={handleSubmit} class="space-y-4">
          <div>
            <label class="label">标签名称 *</label>
            <input
              type="text"
              class="input"
              required
              placeholder="输入标签名称"
              value={form().name}
              onInput={(e) => setForm({ ...form(), name: e.currentTarget.value })}
            />
          </div>
          <div>
            <label class="label">颜色</label>
            <div class="flex items-center gap-3">
              <input
                type="color"
                class="w-12 h-12 rounded-xl cursor-pointer border-2 border-dark-700 bg-dark-800"
                value={form().color}
                onInput={(e) => setForm({ ...form(), color: e.currentTarget.value })}
              />
              <input
                type="text"
                class="input flex-1 font-mono"
                value={form().color}
                onInput={(e) => setForm({ ...form(), color: e.currentTarget.value })}
                pattern="^#[0-9A-Fa-f]{6}$"
                placeholder="#6366f1"
              />
            </div>
            <div class="flex gap-2 mt-2">
              {['#6366f1', '#8b5cf6', '#ec4899', '#ef4444', '#f97316', '#eab308', '#22c55e', '#14b8a6'].map((c) => (
                <button
                  type="button"
                  class="w-6 h-6 rounded-md transition-transform hover:scale-110"
                  style={{ "background-color": c }}
                  classList={{ 'ring-2 ring-white ring-offset-2 ring-offset-dark-900': form().color === c }}
                  onClick={() => setForm({ ...form(), color: c })}
                />
              ))}
            </div>
          </div>
          <div>
            <label class="label">描述</label>
            <textarea
              class="input resize-none"
              rows="3"
              placeholder="输入标签描述（可选）"
              value={form().description}
              onInput={(e) => setForm({ ...form(), description: e.currentTarget.value })}
            />
          </div>
        </form>
      </Modal>
    </div>
  )
}

export default Tags

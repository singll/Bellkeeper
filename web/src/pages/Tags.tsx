import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { tagsApi } from '@/api'
import type { Tag } from '@/types'

const Tags: Component = () => {
  const [page, setPage] = createSignal(1)
  const [keyword, setKeyword] = createSignal('')
  const [showModal, setShowModal] = createSignal(false)
  const [editingTag, setEditingTag] = createSignal<Tag | null>(null)

  const [tags, { refetch }] = createResource(
    () => ({ page: page(), keyword: keyword() }),
    ({ page, keyword }) => tagsApi.list(page, 20, keyword)
  )

  const [form, setForm] = createSignal({
    name: '',
    description: '',
    color: '#409EFF',
  })

  const openCreateModal = () => {
    setEditingTag(null)
    setForm({ name: '', description: '', color: '#409EFF' })
    setShowModal(true)
  }

  const openEditModal = (tag: Tag) => {
    setEditingTag(tag)
    setForm({ name: tag.name, description: tag.description, color: tag.color })
    setShowModal(true)
  }

  const handleSubmit = async (e: Event) => {
    e.preventDefault()
    try {
      if (editingTag()) {
        await tagsApi.update(editingTag()!.id, form())
      } else {
        await tagsApi.create(form())
      }
      setShowModal(false)
      refetch()
    } catch (err) {
      alert('操作失败: ' + (err as Error).message)
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('确定要删除这个标签吗？')) return
    try {
      await tagsApi.delete(id)
      refetch()
    } catch (err) {
      alert('删除失败: ' + (err as Error).message)
    }
  }

  return (
    <div>
      <div class="flex items-center justify-between mb-6">
        <h1 class="text-2xl font-bold">标签管理</h1>
        <button class="btn btn-primary" onClick={openCreateModal}>
          新建标签
        </button>
      </div>

      {/* Search */}
      <div class="card mb-6">
        <input
          type="text"
          class="input"
          placeholder="搜索标签..."
          value={keyword()}
          onInput={(e) => setKeyword(e.currentTarget.value)}
        />
      </div>

      {/* Table */}
      <div class="card overflow-hidden">
        <table class="table">
          <thead>
            <tr>
              <th>标签名</th>
              <th>颜色</th>
              <th>描述</th>
              <th>创建时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <Show
              when={!tags.loading}
              fallback={
                <tr>
                  <td colspan="5" class="text-center py-8 text-dark-muted">
                    加载中...
                  </td>
                </tr>
              }
            >
              <For each={tags()?.data}>
                {(tag) => (
                  <tr>
                    <td>
                      <span
                        class="px-2 py-1 rounded text-sm"
                        style={{ "background-color": tag.color + '30', color: tag.color }}
                      >
                        {tag.name}
                      </span>
                    </td>
                    <td>
                      <div
                        class="w-6 h-6 rounded"
                        style={{ "background-color": tag.color }}
                      />
                    </td>
                    <td class="text-dark-muted">{tag.description || '-'}</td>
                    <td class="text-dark-muted">
                      {new Date(tag.created_at).toLocaleDateString()}
                    </td>
                    <td>
                      <button
                        class="text-primary-500 hover:text-primary-400 mr-3"
                        onClick={() => openEditModal(tag)}
                      >
                        编辑
                      </button>
                      <button
                        class="text-red-500 hover:text-red-400"
                        onClick={() => handleDelete(tag.id)}
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

      {/* Pagination */}
      <Show when={tags()}>
        <div class="flex items-center justify-between mt-4">
          <div class="text-dark-muted text-sm">
            共 {tags()!.total} 条
          </div>
          <div class="flex gap-2">
            <button
              class="btn btn-secondary"
              disabled={page() === 1}
              onClick={() => setPage((p) => p - 1)}
            >
              上一页
            </button>
            <button
              class="btn btn-secondary"
              disabled={page() * 20 >= (tags()?.total || 0)}
              onClick={() => setPage((p) => p + 1)}
            >
              下一页
            </button>
          </div>
        </div>
      </Show>

      {/* Modal */}
      <Show when={showModal()}>
        <div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div class="card w-full max-w-md">
            <h2 class="text-lg font-semibold mb-4">
              {editingTag() ? '编辑标签' : '新建标签'}
            </h2>
            <form onSubmit={handleSubmit}>
              <div class="space-y-4">
                <div>
                  <label class="label">标签名</label>
                  <input
                    type="text"
                    class="input"
                    required
                    value={form().name}
                    onInput={(e) => setForm({ ...form(), name: e.currentTarget.value })}
                  />
                </div>
                <div>
                  <label class="label">颜色</label>
                  <input
                    type="color"
                    class="w-full h-10 rounded cursor-pointer"
                    value={form().color}
                    onInput={(e) => setForm({ ...form(), color: e.currentTarget.value })}
                  />
                </div>
                <div>
                  <label class="label">描述</label>
                  <textarea
                    class="input"
                    rows="3"
                    value={form().description}
                    onInput={(e) => setForm({ ...form(), description: e.currentTarget.value })}
                  />
                </div>
              </div>
              <div class="flex justify-end gap-3 mt-6">
                <button type="button" class="btn btn-secondary" onClick={() => setShowModal(false)}>
                  取消
                </button>
                <button type="submit" class="btn btn-primary">
                  {editingTag() ? '保存' : '创建'}
                </button>
              </div>
            </form>
          </div>
        </div>
      </Show>
    </div>
  )
}

export default Tags

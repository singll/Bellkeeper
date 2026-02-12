import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { dataSourcesApi, tagsApi } from '@/api'
import type { DataSource, Tag } from '@/types'

const DataSources: Component = () => {
  const [page, setPage] = createSignal(1)
  const [keyword, setKeyword] = createSignal('')
  const [showModal, setShowModal] = createSignal(false)
  const [editing, setEditing] = createSignal<DataSource | null>(null)

  const [sources, { refetch }] = createResource(
    () => ({ page: page(), keyword: keyword() }),
    ({ page, keyword }) => dataSourcesApi.list(page, 20, '', keyword)
  )

  const [allTags] = createResource(() => tagsApi.list(1, 100))

  const [form, setForm] = createSignal({
    name: '',
    url: '',
    type: 'website',
    category: '',
    description: '',
    is_active: true,
    tag_ids: [] as number[],
  })

  const openCreateModal = () => {
    setEditing(null)
    setForm({
      name: '',
      url: '',
      type: 'website',
      category: '',
      description: '',
      is_active: true,
      tag_ids: [],
    })
    setShowModal(true)
  }

  const openEditModal = (source: DataSource) => {
    setEditing(source)
    setForm({
      name: source.name,
      url: source.url,
      type: source.type,
      category: source.category,
      description: source.description,
      is_active: source.is_active,
      tag_ids: source.tags?.map((t) => t.id) || [],
    })
    setShowModal(true)
  }

  const handleSubmit = async (e: Event) => {
    e.preventDefault()
    try {
      if (editing()) {
        await dataSourcesApi.update(editing()!.id, form())
      } else {
        await dataSourcesApi.create(form())
      }
      setShowModal(false)
      refetch()
    } catch (err) {
      alert('操作失败: ' + (err as Error).message)
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('确定要删除这个数据源吗？')) return
    try {
      await dataSourcesApi.delete(id)
      refetch()
    } catch (err) {
      alert('删除失败: ' + (err as Error).message)
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
    <div>
      <div class="flex items-center justify-between mb-6">
        <h1 class="text-2xl font-bold">数据源管理</h1>
        <button class="btn btn-primary" onClick={openCreateModal}>
          新建数据源
        </button>
      </div>

      {/* Search */}
      <div class="card mb-6">
        <input
          type="text"
          class="input"
          placeholder="搜索数据源..."
          value={keyword()}
          onInput={(e) => setKeyword(e.currentTarget.value)}
        />
      </div>

      {/* Table */}
      <div class="card overflow-hidden">
        <table class="table">
          <thead>
            <tr>
              <th>名称</th>
              <th>URL</th>
              <th>类型</th>
              <th>标签</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <Show
              when={!sources.loading}
              fallback={
                <tr>
                  <td colspan="6" class="text-center py-8 text-dark-muted">加载中...</td>
                </tr>
              }
            >
              <For each={sources()?.data}>
                {(source) => (
                  <tr>
                    <td class="font-medium">{source.name}</td>
                    <td class="text-dark-muted truncate max-w-xs">{source.url}</td>
                    <td>{source.type}</td>
                    <td>
                      <div class="flex flex-wrap gap-1">
                        <For each={source.tags}>
                          {(tag) => (
                            <span
                              class="px-2 py-0.5 rounded text-xs"
                              style={{ "background-color": tag.color + '30', color: tag.color }}
                            >
                              {tag.name}
                            </span>
                          )}
                        </For>
                      </div>
                    </td>
                    <td>
                      <span class={source.is_active ? 'text-green-500' : 'text-dark-muted'}>
                        {source.is_active ? '启用' : '禁用'}
                      </span>
                    </td>
                    <td>
                      <button
                        class="text-primary-500 hover:text-primary-400 mr-3"
                        onClick={() => openEditModal(source)}
                      >
                        编辑
                      </button>
                      <button
                        class="text-red-500 hover:text-red-400"
                        onClick={() => handleDelete(source.id)}
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

      {/* Modal */}
      <Show when={showModal()}>
        <div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div class="card w-full max-w-lg max-h-[90vh] overflow-y-auto">
            <h2 class="text-lg font-semibold mb-4">
              {editing() ? '编辑数据源' : '新建数据源'}
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
                    class="input"
                    required
                    value={form().url}
                    onInput={(e) => setForm({ ...form(), url: e.currentTarget.value })}
                  />
                </div>
                <div class="grid grid-cols-2 gap-4">
                  <div>
                    <label class="label">类型</label>
                    <select
                      class="input"
                      value={form().type}
                      onChange={(e) => setForm({ ...form(), type: e.currentTarget.value })}
                    >
                      <option value="website">网站</option>
                      <option value="api">API</option>
                      <option value="github">GitHub</option>
                    </select>
                  </div>
                  <div>
                    <label class="label">分类</label>
                    <input
                      type="text"
                      class="input"
                      value={form().category}
                      onInput={(e) => setForm({ ...form(), category: e.currentTarget.value })}
                    />
                  </div>
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
                <div>
                  <label class="label">标签</label>
                  <div class="flex flex-wrap gap-2 p-3 bg-dark-bg rounded-lg">
                    <For each={allTags()?.data}>
                      {(tag) => (
                        <button
                          type="button"
                          class="px-2 py-1 rounded text-sm transition-colors"
                          classList={{
                            'ring-2 ring-primary-500': form().tag_ids.includes(tag.id),
                          }}
                          style={{ "background-color": tag.color + '30', color: tag.color }}
                          onClick={() => toggleTag(tag.id)}
                        >
                          {tag.name}
                        </button>
                      )}
                    </For>
                  </div>
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
    </div>
  )
}

export default DataSources

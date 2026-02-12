import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { datasetsApi, tagsApi } from '@/api'
import type { DatasetMapping } from '@/types'

const Datasets: Component = () => {
  const [page, setPage] = createSignal(1)
  const [showModal, setShowModal] = createSignal(false)
  const [editing, setEditing] = createSignal<DatasetMapping | null>(null)

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
    try {
      if (editing()) {
        await datasetsApi.update(editing()!.id, form())
      } else {
        await datasetsApi.create(form())
      }
      setShowModal(false)
      refetch()
    } catch (err) {
      alert('操作失败: ' + (err as Error).message)
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('确定要删除这个知识库映射吗？')) return
    try {
      await datasetsApi.delete(id)
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
        <h1 class="text-2xl font-bold">知识库映射</h1>
        <button class="btn btn-primary" onClick={openCreateModal}>
          新建映射
        </button>
      </div>

      {/* Table */}
      <div class="card overflow-hidden">
        <table class="table">
          <thead>
            <tr>
              <th>名称</th>
              <th>显示名称</th>
              <th>Dataset ID</th>
              <th>关联标签</th>
              <th>解析器</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <Show
              when={!datasets.loading}
              fallback={
                <tr>
                  <td colspan="7" class="text-center py-8 text-dark-muted">加载中...</td>
                </tr>
              }
            >
              <For each={datasets()?.data}>
                {(dataset) => (
                  <tr>
                    <td class="font-medium">
                      {dataset.name}
                      {dataset.is_default && (
                        <span class="ml-2 px-2 py-0.5 bg-primary-500/20 text-primary-400 text-xs rounded">
                          默认
                        </span>
                      )}
                    </td>
                    <td>{dataset.display_name || '-'}</td>
                    <td class="text-dark-muted font-mono text-sm truncate max-w-[200px]">
                      {dataset.dataset_id}
                    </td>
                    <td>
                      <div class="flex flex-wrap gap-1">
                        <For each={dataset.tags}>
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
                    <td>{dataset.parser_id}</td>
                    <td>
                      <span class={dataset.is_active ? 'text-green-500' : 'text-dark-muted'}>
                        {dataset.is_active ? '启用' : '禁用'}
                      </span>
                    </td>
                    <td>
                      <button
                        class="text-primary-500 hover:text-primary-400 mr-3"
                        onClick={() => openEditModal(dataset)}
                      >
                        编辑
                      </button>
                      <button
                        class="text-red-500 hover:text-red-400"
                        onClick={() => handleDelete(dataset.id)}
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
              {editing() ? '编辑映射' : '新建映射'}
            </h2>
            <form onSubmit={handleSubmit}>
              <div class="space-y-4">
                <div class="grid grid-cols-2 gap-4">
                  <div>
                    <label class="label">映射名称 (API 使用)</label>
                    <input
                      type="text"
                      class="input"
                      required
                      placeholder="security"
                      value={form().name}
                      onInput={(e) => setForm({ ...form(), name: e.currentTarget.value })}
                    />
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
                  <label class="label">RagFlow Dataset ID</label>
                  <input
                    type="text"
                    class="input font-mono"
                    required
                    placeholder="abc123..."
                    value={form().dataset_id}
                    onInput={(e) => setForm({ ...form(), dataset_id: e.currentTarget.value })}
                  />
                </div>
                <div>
                  <label class="label">解析器</label>
                  <select
                    class="input"
                    value={form().parser_id}
                    onChange={(e) => setForm({ ...form(), parser_id: e.currentTarget.value })}
                  >
                    <option value="naive">Naive</option>
                    <option value="paper">Paper</option>
                    <option value="laws">Laws</option>
                    <option value="qa">QA</option>
                  </select>
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
                  <label class="label">关联标签 (用于自动路由)</label>
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
                <div class="flex items-center gap-6">
                  <div class="flex items-center gap-2">
                    <input
                      type="checkbox"
                      id="is_active"
                      checked={form().is_active}
                      onChange={(e) => setForm({ ...form(), is_active: e.currentTarget.checked })}
                    />
                    <label for="is_active">启用</label>
                  </div>
                  <div class="flex items-center gap-2">
                    <input
                      type="checkbox"
                      id="is_default"
                      checked={form().is_default}
                      onChange={(e) => setForm({ ...form(), is_default: e.currentTarget.checked })}
                    />
                    <label for="is_default">设为默认</label>
                  </div>
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

export default Datasets

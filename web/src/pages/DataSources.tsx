import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { dataSourcesApi, tagsApi } from '@/api'
import { useToast } from '@/components/Toast'
import Modal from '@/components/Modal'
import type { DataSource, Tag } from '@/types'

const DataSources: Component = () => {
  const toast = useToast()
  const [page, setPage] = createSignal(1)
  const [keyword, setKeyword] = createSignal('')
  const [showModal, setShowModal] = createSignal(false)
  const [editing, setEditing] = createSignal<DataSource | null>(null)
  const [submitting, setSubmitting] = createSignal(false)

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
    setSubmitting(true)
    try {
      if (editing()) {
        await dataSourcesApi.update(editing()!.id, form())
        toast.success('数据源更新成功')
      } else {
        await dataSourcesApi.create(form())
        toast.success('数据源创建成功')
      }
      setShowModal(false)
      refetch()
    } catch (err) {
      toast.error('操作失败: ' + (err as Error).message)
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (source: DataSource) => {
    if (!confirm(`确定要删除数据源"${source.name}"吗？`)) return
    try {
      await dataSourcesApi.delete(source.id)
      toast.success('数据源删除成功')
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
          <h1 class="text-2xl font-bold text-white">数据源管理</h1>
          <p class="text-sm text-dark-400 mt-1">管理知识采集的数据来源</p>
        </div>
        <button class="btn btn-primary" onClick={openCreateModal}>
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
          </svg>
          新建数据源
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
              placeholder="搜索数据源名称或 URL..."
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
                <th>名称</th>
                <th>URL</th>
                <th>类型</th>
                <th>标签</th>
                <th>状态</th>
                <th class="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              <Show
                when={!sources.loading}
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
                  when={sources()?.data && sources()!.data.length > 0}
                  fallback={
                    <tr>
                      <td colspan="6">
                        <div class="empty-state">
                          <svg class="empty-state-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4m0 5c0 2.21-3.582 4-8 4s-8-1.79-8-4" />
                          </svg>
                          <p class="empty-state-title">暂无数据源</p>
                          <p class="empty-state-description">点击"新建数据源"添加第一个数据源</p>
                        </div>
                      </td>
                    </tr>
                  }
                >
                  <For each={sources()?.data}>
                    {(source) => (
                      <tr class="group">
                        <td>
                          <div class="flex items-center gap-2">
                            <span class="font-medium text-white">{source.name}</span>
                            <Show when={source.category}>
                              <span class="badge badge-gray">{source.category}</span>
                            </Show>
                          </div>
                        </td>
                        <td>
                          <a
                            href={source.url}
                            target="_blank"
                            rel="noopener noreferrer"
                            class="text-dark-400 hover:text-primary-400 truncate max-w-xs block font-mono text-sm"
                          >
                            {source.url}
                          </a>
                        </td>
                        <td>
                          <span class="badge badge-primary">{source.type}</span>
                        </td>
                        <td>
                          <div class="flex flex-wrap gap-1">
                            <For each={source.tags}>
                              {(tag) => (
                                <span
                                  class="px-2 py-0.5 rounded-md text-xs font-medium"
                                  style={{ "background-color": tag.color + '20', color: tag.color }}
                                >
                                  {tag.name}
                                </span>
                              )}
                            </For>
                            <Show when={!source.tags || source.tags.length === 0}>
                              <span class="text-dark-600">-</span>
                            </Show>
                          </div>
                        </td>
                        <td>
                          <div class="flex items-center gap-2">
                            <span class={`status-dot ${source.is_active ? 'status-dot-success' : 'status-dot-gray'}`} />
                            <span class={source.is_active ? 'text-emerald-400' : 'text-dark-500'}>
                              {source.is_active ? '启用' : '禁用'}
                            </span>
                          </div>
                        </td>
                        <td class="text-right">
                          <div class="flex items-center justify-end gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                            <button
                              class="btn btn-ghost btn-sm"
                              onClick={() => openEditModal(source)}
                            >
                              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                              </svg>
                            </button>
                            <button
                              class="btn btn-ghost btn-sm text-red-400 hover:text-red-300 hover:bg-red-500/10"
                              onClick={() => handleDelete(source)}
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
      <Show when={sources() && sources()!.total > 20}>
        <div class="flex items-center justify-between mt-4">
          <div class="text-sm text-dark-400">
            共 <span class="text-dark-200 font-medium">{sources()!.total}</span> 条记录
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
              {page()} / {Math.ceil((sources()?.total || 0) / 20)}
            </span>
            <button
              class="btn btn-secondary btn-sm"
              disabled={page() * 20 >= (sources()?.total || 0)}
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
        title={editing() ? '编辑数据源' : '新建数据源'}
        size="lg"
        footer={
          <>
            <button type="button" class="btn btn-secondary" onClick={() => setShowModal(false)}>
              取消
            </button>
            <button
              type="submit"
              form="datasource-form"
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
        <form id="datasource-form" onSubmit={handleSubmit} class="space-y-4">
          <div>
            <label class="label">名称 *</label>
            <input
              type="text"
              class="input"
              required
              placeholder="输入数据源名称"
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
              placeholder="https://example.com"
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
                <option value="rss">RSS</option>
              </select>
            </div>
            <div>
              <label class="label">分类</label>
              <input
                type="text"
                class="input"
                placeholder="如：安全、技术..."
                value={form().category}
                onInput={(e) => setForm({ ...form(), category: e.currentTarget.value })}
              />
            </div>
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
            <label class="label">标签</label>
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
          <div class="flex items-center gap-3 pt-2">
            <label class="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                class="sr-only peer"
                checked={form().is_active}
                onChange={(e) => setForm({ ...form(), is_active: e.currentTarget.checked })}
              />
              <div class="w-11 h-6 bg-dark-700 peer-focus:outline-none peer-focus:ring-2 peer-focus:ring-primary-500 rounded-full peer peer-checked:after:translate-x-full rtl:peer-checked:after:-translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:start-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary-600"></div>
              <span class="ms-3 text-sm font-medium text-dark-300">启用数据源</span>
            </label>
          </div>
        </form>
      </Modal>
    </div>
  )
}

export default DataSources

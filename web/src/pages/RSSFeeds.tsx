import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { rssApi, tagsApi } from '@/api'
import type { RSSFeed } from '@/types'

const RSSFeeds: Component = () => {
  const [page, setPage] = createSignal(1)
  const [keyword, setKeyword] = createSignal('')
  const [showModal, setShowModal] = createSignal(false)
  const [editing, setEditing] = createSignal<RSSFeed | null>(null)

  const [feeds, { refetch }] = createResource(
    () => ({ page: page(), keyword: keyword() }),
    ({ page, keyword }) => rssApi.list(page, 20, '', keyword)
  )

  const [allTags] = createResource(() => tagsApi.list(1, 100))

  const [form, setForm] = createSignal({
    name: '',
    url: '',
    category: '',
    description: '',
    is_active: true,
    fetch_interval_minutes: 60,
    tag_ids: [] as number[],
  })

  const openCreateModal = () => {
    setEditing(null)
    setForm({
      name: '',
      url: '',
      category: '',
      description: '',
      is_active: true,
      fetch_interval_minutes: 60,
      tag_ids: [],
    })
    setShowModal(true)
  }

  const openEditModal = (feed: RSSFeed) => {
    setEditing(feed)
    setForm({
      name: feed.name,
      url: feed.url,
      category: feed.category,
      description: feed.description,
      is_active: feed.is_active,
      fetch_interval_minutes: feed.fetch_interval_minutes,
      tag_ids: feed.tags?.map((t) => t.id) || [],
    })
    setShowModal(true)
  }

  const handleSubmit = async (e: Event) => {
    e.preventDefault()
    try {
      if (editing()) {
        await rssApi.update(editing()!.id, form())
      } else {
        await rssApi.create(form())
      }
      setShowModal(false)
      refetch()
    } catch (err) {
      alert('操作失败: ' + (err as Error).message)
    }
  }

  const handleDelete = async (id: number) => {
    if (!confirm('确定要删除这个 RSS 订阅吗？')) return
    try {
      await rssApi.delete(id)
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
        <h1 class="text-2xl font-bold">RSS 订阅管理</h1>
        <button class="btn btn-primary" onClick={openCreateModal}>
          新建订阅
        </button>
      </div>

      {/* Search */}
      <div class="card mb-6">
        <input
          type="text"
          class="input"
          placeholder="搜索订阅..."
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
              <th>分类</th>
              <th>标签</th>
              <th>状态</th>
              <th>最后抓取</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <Show
              when={!feeds.loading}
              fallback={
                <tr>
                  <td colspan="7" class="text-center py-8 text-dark-muted">加载中...</td>
                </tr>
              }
            >
              <For each={feeds()?.data}>
                {(feed) => (
                  <tr>
                    <td class="font-medium">{feed.name}</td>
                    <td class="text-dark-muted truncate max-w-xs">{feed.url}</td>
                    <td>{feed.category || '-'}</td>
                    <td>
                      <div class="flex flex-wrap gap-1">
                        <For each={feed.tags}>
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
                      <span class={feed.is_active ? 'text-green-500' : 'text-dark-muted'}>
                        {feed.is_active ? '启用' : '禁用'}
                      </span>
                    </td>
                    <td class="text-dark-muted">
                      {feed.last_fetched_at
                        ? new Date(feed.last_fetched_at).toLocaleString()
                        : '未抓取'}
                    </td>
                    <td>
                      <button
                        class="text-primary-500 hover:text-primary-400 mr-3"
                        onClick={() => openEditModal(feed)}
                      >
                        编辑
                      </button>
                      <button
                        class="text-red-500 hover:text-red-400"
                        onClick={() => handleDelete(feed.id)}
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
              {editing() ? '编辑订阅' : '新建订阅'}
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
                  <label class="label">RSS URL</label>
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
                    <label class="label">分类</label>
                    <input
                      type="text"
                      class="input"
                      value={form().category}
                      onInput={(e) => setForm({ ...form(), category: e.currentTarget.value })}
                    />
                  </div>
                  <div>
                    <label class="label">抓取间隔 (分钟)</label>
                    <input
                      type="number"
                      class="input"
                      min="5"
                      value={form().fetch_interval_minutes}
                      onInput={(e) => setForm({ ...form(), fetch_interval_minutes: parseInt(e.currentTarget.value) })}
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

export default RSSFeeds

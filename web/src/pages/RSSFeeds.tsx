import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { rssApi, tagsApi } from '@/api'
import { useToast } from '@/components/Toast'
import Modal from '@/components/Modal'
import type { RSSFeed } from '@/types'

const RSSFeeds: Component = () => {
  const toast = useToast()
  const [page, setPage] = createSignal(1)
  const [keyword, setKeyword] = createSignal('')
  const [showModal, setShowModal] = createSignal(false)
  const [editing, setEditing] = createSignal<RSSFeed | null>(null)
  const [submitting, setSubmitting] = createSignal(false)

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
    setSubmitting(true)
    try {
      if (editing()) {
        await rssApi.update(editing()!.id, form())
        toast.success('RSS 订阅更新成功')
      } else {
        await rssApi.create(form())
        toast.success('RSS 订阅创建成功')
      }
      setShowModal(false)
      refetch()
    } catch (err) {
      toast.error('操作失败: ' + (err as Error).message)
    } finally {
      setSubmitting(false)
    }
  }

  const handleDelete = async (feed: RSSFeed) => {
    if (!confirm(`确定要删除 RSS 订阅"${feed.name}"吗？`)) return
    try {
      await rssApi.delete(feed.id)
      toast.success('RSS 订阅删除成功')
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

  const formatLastFetched = (date: string | null) => {
    if (!date) return '从未抓取'
    const d = new Date(date)
    const now = new Date()
    const diff = now.getTime() - d.getTime()
    const minutes = Math.floor(diff / 60000)
    if (minutes < 60) return `${minutes} 分钟前`
    const hours = Math.floor(minutes / 60)
    if (hours < 24) return `${hours} 小时前`
    return d.toLocaleDateString('zh-CN')
  }

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">RSS 订阅管理</h1>
          <p class="text-sm text-dark-400 mt-1">管理自动抓取的 RSS 订阅源</p>
        </div>
        <button class="btn btn-primary" onClick={openCreateModal}>
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
          </svg>
          新建订阅
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
              placeholder="搜索订阅名称..."
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
                <th>分类</th>
                <th>标签</th>
                <th>状态</th>
                <th>最后抓取</th>
                <th class="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              <Show
                when={!feeds.loading}
                fallback={
                  <tr>
                    <td colspan="7" class="text-center py-12">
                      <div class="loading-spinner mx-auto" />
                      <p class="mt-3 text-dark-400">加载中...</p>
                    </td>
                  </tr>
                }
              >
                <Show
                  when={feeds()?.data && feeds()!.data.length > 0}
                  fallback={
                    <tr>
                      <td colspan="7">
                        <div class="empty-state">
                          <svg class="empty-state-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M6 5c7.18 0 13 5.82 13 13M6 11a7 7 0 017 7m-6 0a1 1 0 11-2 0 1 1 0 012 0z" />
                          </svg>
                          <p class="empty-state-title">暂无 RSS 订阅</p>
                          <p class="empty-state-description">点击"新建订阅"添加第一个 RSS 源</p>
                        </div>
                      </td>
                    </tr>
                  }
                >
                  <For each={feeds()?.data}>
                    {(feed) => (
                      <tr class="group">
                        <td>
                          <div class="flex items-center gap-2">
                            <svg class="w-4 h-4 text-orange-400" fill="currentColor" viewBox="0 0 24 24">
                              <path d="M6.18 15.64a2.18 2.18 0 010 4.36 2.18 2.18 0 010-4.36m.82-4.64v3.27c0 .1.04.18.13.24a.34.34 0 00.28.06c1.84-.22 3.4.52 4.7 1.81 1.29 1.29 2.03 2.86 1.81 4.7a.34.34 0 00.06.28c.06.09.14.13.24.13h3.27c.1 0 .19-.04.27-.11a.36.36 0 00.12-.27c-.08-2.63-1.02-4.87-2.81-6.67-1.8-1.79-4.04-2.73-6.67-2.81a.36.36 0 00-.27.12c-.07.08-.11.17-.11.27M7 4v3.27c0 .1.04.18.13.24a.34.34 0 00.28.06c4.41-.53 8.17.86 11.27 4.17 3.1 3.31 4.48 7.07 4.17 11.27a.34.34 0 00.06.28c.06.09.14.13.24.13h3.27c.1 0 .19-.04.27-.11a.36.36 0 00.12-.27 18.52 18.52 0 00-5.71-14.27A18.52 18.52 0 007.25 3.73a.36.36 0 00-.27.12c-.07.08-.11.17-.11.27l.13-.12z" />
                            </svg>
                            <span class="font-medium text-white">{feed.name}</span>
                          </div>
                        </td>
                        <td>
                          <a
                            href={feed.url}
                            target="_blank"
                            rel="noopener noreferrer"
                            class="text-dark-400 hover:text-primary-400 truncate max-w-[200px] block font-mono text-sm"
                          >
                            {feed.url}
                          </a>
                        </td>
                        <td>
                          <Show when={feed.category} fallback={<span class="text-dark-600">-</span>}>
                            <span class="badge badge-gray">{feed.category}</span>
                          </Show>
                        </td>
                        <td>
                          <div class="flex flex-wrap gap-1">
                            <For each={feed.tags}>
                              {(tag) => (
                                <span
                                  class="px-2 py-0.5 rounded-md text-xs font-medium"
                                  style={{ "background-color": tag.color + '20', color: tag.color }}
                                >
                                  {tag.name}
                                </span>
                              )}
                            </For>
                            <Show when={!feed.tags || feed.tags.length === 0}>
                              <span class="text-dark-600">-</span>
                            </Show>
                          </div>
                        </td>
                        <td>
                          <div class="flex items-center gap-2">
                            <span class={`status-dot ${feed.is_active ? 'status-dot-success' : 'status-dot-gray'}`} />
                            <span class={feed.is_active ? 'text-emerald-400' : 'text-dark-500'}>
                              {feed.is_active ? '启用' : '禁用'}
                            </span>
                          </div>
                        </td>
                        <td>
                          <span class="text-dark-400 text-sm">
                            {formatLastFetched(feed.last_fetched_at)}
                          </span>
                        </td>
                        <td class="text-right">
                          <div class="flex items-center justify-end gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                            <button
                              class="btn btn-ghost btn-sm"
                              onClick={() => openEditModal(feed)}
                            >
                              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                              </svg>
                            </button>
                            <button
                              class="btn btn-ghost btn-sm text-red-400 hover:text-red-300 hover:bg-red-500/10"
                              onClick={() => handleDelete(feed)}
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
      <Show when={feeds() && feeds()!.total > 20}>
        <div class="flex items-center justify-between mt-4">
          <div class="text-sm text-dark-400">
            共 <span class="text-dark-200 font-medium">{feeds()!.total}</span> 条记录
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
              {page()} / {Math.ceil((feeds()?.total || 0) / 20)}
            </span>
            <button
              class="btn btn-secondary btn-sm"
              disabled={page() * 20 >= (feeds()?.total || 0)}
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
        title={editing() ? '编辑 RSS 订阅' : '新建 RSS 订阅'}
        size="lg"
        footer={
          <>
            <button type="button" class="btn btn-secondary" onClick={() => setShowModal(false)}>
              取消
            </button>
            <button
              type="submit"
              form="rss-form"
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
        <form id="rss-form" onSubmit={handleSubmit} class="space-y-4">
          <div>
            <label class="label">名称 *</label>
            <input
              type="text"
              class="input"
              required
              placeholder="输入订阅名称"
              value={form().name}
              onInput={(e) => setForm({ ...form(), name: e.currentTarget.value })}
            />
          </div>
          <div>
            <label class="label">RSS URL *</label>
            <input
              type="url"
              class="input font-mono"
              required
              placeholder="https://example.com/feed.xml"
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
                placeholder="如：技术、安全..."
                value={form().category}
                onInput={(e) => setForm({ ...form(), category: e.currentTarget.value })}
              />
            </div>
            <div>
              <label class="label">抓取间隔</label>
              <div class="relative">
                <input
                  type="number"
                  class="input pr-12"
                  min="5"
                  value={form().fetch_interval_minutes}
                  onInput={(e) => setForm({ ...form(), fetch_interval_minutes: parseInt(e.currentTarget.value) || 60 })}
                />
                <span class="absolute right-4 top-1/2 -translate-y-1/2 text-dark-500 text-sm">分钟</span>
              </div>
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
              <span class="ms-3 text-sm font-medium text-dark-300">启用订阅</span>
            </label>
          </div>
        </form>
      </Modal>
    </div>
  )
}

export default RSSFeeds

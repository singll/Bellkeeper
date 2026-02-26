import { Component, createSignal, createEffect, For, Show } from 'solid-js'
import { datasetsApi, ragflowApi, type RagFlowDocument } from '@/api'
import { useToast } from '@/components/Toast'
import Modal from '@/components/Modal'
import type { DatasetMapping } from '@/types'

const Documents: Component = () => {
  const toast = useToast()
  const [datasets, setDatasets] = createSignal<DatasetMapping[]>([])
  const [selectedDataset, setSelectedDataset] = createSignal<string>('')
  const [documents, setDocuments] = createSignal<RagFlowDocument[]>([])
  const [total, setTotal] = createSignal(0)
  const [page, setPage] = createSignal(1)
  const [loading, setLoading] = createSignal(false)
  const [error, setError] = createSignal('')

  const [showUploadModal, setShowUploadModal] = createSignal(false)
  const [uploadForm, setUploadForm] = createSignal({
    content: '',
    filename: '',
    title: '',
    url: '',
    tags: '',
    category: '',
    useRouting: true,
  })
  const [uploading, setUploading] = createSignal(false)
  const [urlCheckResult, setUrlCheckResult] = createSignal<{ checked: boolean; exists: boolean } | null>(null)

  createEffect(async () => {
    try {
      const res = await datasetsApi.list(1, 100)
      setDatasets(res.data || [])
      if (res.data && res.data.length > 0) {
        const defaultDs = res.data.find(d => d.is_default) || res.data[0]
        setSelectedDataset(defaultDs.dataset_id)
      }
    } catch (err) {
      setError('加载知识库列表失败')
    }
  })

  createEffect(async () => {
    const dsId = selectedDataset()
    if (!dsId) return
    await loadDocuments()
  })

  const loadDocuments = async () => {
    const dsId = selectedDataset()
    if (!dsId) return

    setLoading(true)
    setError('')
    try {
      const res = await ragflowApi.listDocuments(dsId, page(), 20)
      if (res.code === 0 && res.data) {
        setDocuments(res.data.docs || [])
        setTotal(res.data.total || 0)
      } else {
        setDocuments([])
        setTotal(0)
      }
    } catch (err) {
      setError('加载文档列表失败')
      setDocuments([])
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = async (docId: string, name: string) => {
    if (!confirm(`确定要删除文档"${name}"吗？`)) return

    try {
      await ragflowApi.deleteDocument(docId, selectedDataset())
      toast.success('文档删除成功')
      await loadDocuments()
    } catch (err) {
      toast.error('删除失败: ' + (err as Error).message)
    }
  }

  const handleCheckUrl = async () => {
    const url = uploadForm().url
    if (!url) return

    try {
      const res = await ragflowApi.checkUrl(url)
      setUrlCheckResult({ checked: true, exists: res.exists })
    } catch (err) {
      setUrlCheckResult(null)
    }
  }

  const handleUpload = async () => {
    const form = uploadForm()
    if (!form.filename) {
      toast.error('请填写文件名')
      return
    }
    if (!form.filename.includes('.')) {
      toast.error('文件名需要包含扩展名，例如 document.md')
      return
    }
    if (!form.content) {
      toast.error('请填写文档内容')
      return
    }
    if (!form.useRouting && !selectedDataset()) {
      toast.error('请先选择一个知识库')
      return
    }

    setUploading(true)
    try {
      const tags = form.tags ? form.tags.split(',').map(t => t.trim()).filter(t => t) : []

      if (form.useRouting) {
        await ragflowApi.uploadWithRouting({
          content: form.content,
          filename: form.filename,
          title: form.title || form.filename,
          url: form.url,
          tags,
          category: form.category,
          auto_create_tags: true,
        })
      } else {
        await ragflowApi.upload({
          content: form.content,
          filename: form.filename,
          title: form.title || form.filename,
          url: form.url,
          dataset_id: selectedDataset(),
        })
      }

      toast.success('文档上传成功')
      setShowUploadModal(false)
      setUploadForm({
        content: '',
        filename: '',
        title: '',
        url: '',
        tags: '',
        category: '',
        useRouting: true,
      })
      setUrlCheckResult(null)
      await loadDocuments()
    } catch (err) {
      toast.error('上传失败: ' + (err as Error).message)
    } finally {
      setUploading(false)
    }
  }

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'done':
        return 'badge-success'
      case 'parsing':
        return 'badge-warning'
      case 'failed':
        return 'badge-danger'
      default:
        return 'badge-gray'
    }
  }

  const getStatusText = (status: string) => {
    switch (status) {
      case 'done':
        return '已完成'
      case 'parsing':
        return '解析中'
      case 'failed':
        return '失败'
      default:
        return status
    }
  }

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">文档管理</h1>
          <p class="text-sm text-dark-400 mt-1">管理 RagFlow 知识库中的文档</p>
        </div>
        <button class="btn btn-primary" onClick={() => setShowUploadModal(true)}>
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
          </svg>
          上传文档
        </button>
      </div>

      {/* Dataset Selector */}
      <div class="card mb-6">
        <div class="flex items-center gap-4">
          <label class="text-sm font-medium text-dark-300">选择知识库</label>
          <select
            class="input max-w-md"
            value={selectedDataset()}
            onChange={(e) => {
              setSelectedDataset(e.currentTarget.value)
              setPage(1)
            }}
          >
            <For each={datasets()}>
              {(ds) => (
                <option value={ds.dataset_id}>
                  {ds.display_name || ds.name} {ds.is_default ? '(默认)' : ''}
                </option>
              )}
            </For>
          </select>
          <button class="btn btn-ghost btn-sm" onClick={loadDocuments}>
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
            刷新
          </button>
        </div>
      </div>

      {/* Error */}
      <Show when={error()}>
        <div class="card mb-6 bg-red-500/10 border-red-500/30">
          <div class="flex items-center gap-2 text-red-400">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            {error()}
          </div>
        </div>
      </Show>

      {/* Table */}
      <div class="card overflow-hidden p-0">
        <div class="overflow-x-auto">
          <table class="table">
            <thead>
              <tr>
                <th>文件名</th>
                <th>状态</th>
                <th>分块数</th>
                <th>创建时间</th>
                <th class="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              <Show
                when={!loading()}
                fallback={
                  <tr>
                    <td colspan="5" class="text-center py-12">
                      <div class="loading-spinner mx-auto" />
                      <p class="mt-3 text-dark-400">加载中...</p>
                    </td>
                  </tr>
                }
              >
                <Show
                  when={documents().length > 0}
                  fallback={
                    <tr>
                      <td colspan="5">
                        <div class="empty-state">
                          <svg class="empty-state-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                          </svg>
                          <p class="empty-state-title">暂无文档</p>
                          <p class="empty-state-description">点击"上传文档"添加第一个文档</p>
                        </div>
                      </td>
                    </tr>
                  }
                >
                  <For each={documents()}>
                    {(doc) => (
                      <tr class="group">
                        <td>
                          <div class="flex items-center gap-2">
                            <svg class="w-5 h-5 text-dark-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                            </svg>
                            <span class="font-medium text-white">{doc.name}</span>
                          </div>
                        </td>
                        <td>
                          <span class={`badge ${getStatusBadge(doc.status)}`}>
                            {getStatusText(doc.status)}
                          </span>
                        </td>
                        <td>
                          <span class="text-dark-400">{doc.chunk_count ?? '-'}</span>
                        </td>
                        <td>
                          <span class="text-dark-400 text-sm">
                            {new Date(doc.created_at).toLocaleDateString('zh-CN')}
                          </span>
                        </td>
                        <td class="text-right">
                          <div class="flex items-center justify-end gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                            <button
                              class="btn btn-ghost btn-sm text-red-400 hover:text-red-300 hover:bg-red-500/10"
                              onClick={() => handleDelete(doc.id, doc.name)}
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
      <Show when={total() > 20}>
        <div class="flex items-center justify-between mt-4">
          <div class="text-sm text-dark-400">
            共 <span class="text-dark-200 font-medium">{total()}</span> 条记录
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
              {page()} / {Math.ceil(total() / 20)}
            </span>
            <button
              class="btn btn-secondary btn-sm"
              disabled={page() >= Math.ceil(total() / 20)}
              onClick={() => setPage((p) => p + 1)}
            >
              下一页
            </button>
          </div>
        </div>
      </Show>

      {/* Upload Modal */}
      <Modal
        open={showUploadModal()}
        onClose={() => setShowUploadModal(false)}
        title="上传文档"
        size="xl"
        footer={
          <>
            <button type="button" class="btn btn-secondary" onClick={() => setShowUploadModal(false)}>
              取消
            </button>
            <button
              type="button"
              class="btn btn-primary"
              disabled={uploading()}
              onClick={handleUpload}
            >
              {uploading() ? (
                <>
                  <div class="loading-spinner" />
                  上传中...
                </>
              ) : '上传'}
            </button>
          </>
        }
      >
        <div class="space-y-4">
          {/* Routing Toggle */}
          <div class="flex items-center gap-3 p-3 bg-dark-800/50 rounded-xl border border-dark-700/50">
            <label class="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                class="sr-only peer"
                checked={uploadForm().useRouting}
                onChange={(e) => setUploadForm(f => ({ ...f, useRouting: e.currentTarget.checked }))}
              />
              <div class="w-11 h-6 bg-dark-700 peer-focus:outline-none peer-focus:ring-2 peer-focus:ring-primary-500 rounded-full peer peer-checked:after:translate-x-full rtl:peer-checked:after:-translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:start-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary-600"></div>
            </label>
            <div>
              <p class="text-sm font-medium text-dark-200">智能路由</p>
              <p class="text-xs text-dark-500">根据标签/分类自动选择知识库</p>
            </div>
          </div>

          <div class="grid grid-cols-2 gap-4">
            <div>
              <label class="label">文件名 *</label>
              <input
                type="text"
                class="input"
                placeholder="document.md"
                value={uploadForm().filename}
                onInput={(e) => setUploadForm(f => ({ ...f, filename: e.currentTarget.value }))}
              />
            </div>
            <div>
              <label class="label">标题</label>
              <input
                type="text"
                class="input"
                placeholder="文档标题"
                value={uploadForm().title}
                onInput={(e) => setUploadForm(f => ({ ...f, title: e.currentTarget.value }))}
              />
            </div>
          </div>

          <div>
            <label class="label">来源 URL</label>
            <div class="flex gap-2">
              <input
                type="url"
                class="input flex-1 font-mono"
                placeholder="https://example.com/article"
                value={uploadForm().url}
                onInput={(e) => {
                  setUploadForm(f => ({ ...f, url: e.currentTarget.value }))
                  setUrlCheckResult(null)
                }}
              />
              <button
                type="button"
                class="btn btn-secondary"
                onClick={handleCheckUrl}
              >
                检查
              </button>
            </div>
            <Show when={urlCheckResult()}>
              <p class={`text-sm mt-1 ${urlCheckResult()!.exists ? 'text-amber-400' : 'text-emerald-400'}`}>
                {urlCheckResult()!.exists ? 'URL 已存在于数据库中' : 'URL 可用'}
              </p>
            </Show>
          </div>

          <Show when={uploadForm().useRouting}>
            <div class="grid grid-cols-2 gap-4">
              <div>
                <label class="label">标签</label>
                <input
                  type="text"
                  class="input"
                  placeholder="security, web, vulnerability"
                  value={uploadForm().tags}
                  onInput={(e) => setUploadForm(f => ({ ...f, tags: e.currentTarget.value }))}
                />
                <p class="text-xs text-dark-500 mt-1">逗号分隔</p>
              </div>
              <div>
                <label class="label">分类</label>
                <input
                  type="text"
                  class="input"
                  placeholder="security"
                  value={uploadForm().category}
                  onInput={(e) => setUploadForm(f => ({ ...f, category: e.currentTarget.value }))}
                />
                <p class="text-xs text-dark-500 mt-1">用于路由匹配</p>
              </div>
            </div>
          </Show>

          <div>
            <label class="label">内容 *</label>
            <textarea
              class="input font-mono text-sm resize-none"
              rows="10"
              placeholder="文档内容..."
              value={uploadForm().content}
              onInput={(e) => setUploadForm(f => ({ ...f, content: e.currentTarget.value }))}
            />
          </div>
        </div>
      </Modal>
    </div>
  )
}

export default Documents

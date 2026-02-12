import { Component, createSignal, createEffect, For, Show } from 'solid-js'
import { datasetsApi, ragflowApi, type RagFlowDocument } from '@/api'
import type { DatasetMapping } from '@/types'

const Documents: Component = () => {
  const [datasets, setDatasets] = createSignal<DatasetMapping[]>([])
  const [selectedDataset, setSelectedDataset] = createSignal<string>('')
  const [documents, setDocuments] = createSignal<RagFlowDocument[]>([])
  const [total, setTotal] = createSignal(0)
  const [page, setPage] = createSignal(1)
  const [loading, setLoading] = createSignal(false)
  const [error, setError] = createSignal('')

  // Upload modal state
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

  // URL check state
  const [urlCheckResult, setUrlCheckResult] = createSignal<{ checked: boolean; exists: boolean } | null>(null)

  // Load datasets on mount
  createEffect(async () => {
    try {
      const res = await datasetsApi.list(1, 100)
      setDatasets(res.data || [])
      if (res.data && res.data.length > 0) {
        const defaultDs = res.data.find(d => d.is_default) || res.data[0]
        setSelectedDataset(defaultDs.dataset_id)
      }
    } catch (err) {
      setError('Failed to load datasets')
    }
  })

  // Load documents when dataset changes
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
      setError('Failed to load documents')
      setDocuments([])
    } finally {
      setLoading(false)
    }
  }

  const handleDelete = async (docId: string) => {
    if (!confirm('Are you sure you want to delete this document?')) return

    try {
      await ragflowApi.deleteDocument(docId, selectedDataset())
      await loadDocuments()
    } catch (err) {
      setError('Failed to delete document')
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
    if (!form.content || !form.filename) {
      alert('Content and filename are required')
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
      alert('Upload failed: ' + (err as Error).message)
    } finally {
      setUploading(false)
    }
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'done':
        return 'text-green-400'
      case 'parsing':
        return 'text-yellow-400'
      case 'failed':
        return 'text-red-400'
      default:
        return 'text-dark-muted'
    }
  }

  return (
    <div>
      <div class="flex justify-between items-center mb-6">
        <h1 class="text-2xl font-bold">RagFlow Documents</h1>
        <button
          class="bg-primary-600 hover:bg-primary-700 text-white px-4 py-2 rounded-lg transition-colors"
          onClick={() => setShowUploadModal(true)}
        >
          + Upload Document
        </button>
      </div>

      {/* Dataset selector */}
      <div class="mb-6">
        <label class="block text-sm text-dark-muted mb-2">Select Dataset</label>
        <select
          class="bg-dark-card border border-dark-border rounded-lg px-4 py-2 w-full max-w-md"
          value={selectedDataset()}
          onChange={(e) => {
            setSelectedDataset(e.currentTarget.value)
            setPage(1)
          }}
        >
          <For each={datasets()}>
            {(ds) => (
              <option value={ds.dataset_id}>
                {ds.display_name || ds.name} {ds.is_default ? '(default)' : ''}
              </option>
            )}
          </For>
        </select>
      </div>

      {/* Error message */}
      <Show when={error()}>
        <div class="bg-red-900/20 border border-red-500 text-red-400 px-4 py-3 rounded-lg mb-6">
          {error()}
        </div>
      </Show>

      {/* Documents table */}
      <div class="bg-dark-card rounded-lg border border-dark-border overflow-hidden">
        <table class="w-full">
          <thead class="bg-dark-bg">
            <tr>
              <th class="text-left px-6 py-3 text-sm font-medium text-dark-muted">Name</th>
              <th class="text-left px-6 py-3 text-sm font-medium text-dark-muted">Status</th>
              <th class="text-left px-6 py-3 text-sm font-medium text-dark-muted">Chunks</th>
              <th class="text-left px-6 py-3 text-sm font-medium text-dark-muted">Created</th>
              <th class="text-right px-6 py-3 text-sm font-medium text-dark-muted">Actions</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-dark-border">
            <Show when={loading()}>
              <tr>
                <td colspan="5" class="px-6 py-8 text-center text-dark-muted">
                  Loading...
                </td>
              </tr>
            </Show>
            <Show when={!loading() && documents().length === 0}>
              <tr>
                <td colspan="5" class="px-6 py-8 text-center text-dark-muted">
                  No documents found
                </td>
              </tr>
            </Show>
            <For each={documents()}>
              {(doc) => (
                <tr class="hover:bg-dark-border/30">
                  <td class="px-6 py-4">
                    <span class="font-medium">{doc.name}</span>
                  </td>
                  <td class="px-6 py-4">
                    <span class={getStatusColor(doc.status)}>{doc.status}</span>
                  </td>
                  <td class="px-6 py-4 text-dark-muted">
                    {doc.chunk_count ?? '-'}
                  </td>
                  <td class="px-6 py-4 text-dark-muted">
                    {new Date(doc.created_at).toLocaleDateString()}
                  </td>
                  <td class="px-6 py-4 text-right">
                    <button
                      class="text-red-400 hover:text-red-300"
                      onClick={() => handleDelete(doc.id)}
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              )}
            </For>
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      <Show when={total() > 20}>
        <div class="flex justify-center gap-2 mt-6">
          <button
            class="px-3 py-1 rounded bg-dark-card border border-dark-border disabled:opacity-50"
            disabled={page() === 1}
            onClick={() => setPage(p => p - 1)}
          >
            Previous
          </button>
          <span class="px-3 py-1 text-dark-muted">
            Page {page()} of {Math.ceil(total() / 20)}
          </span>
          <button
            class="px-3 py-1 rounded bg-dark-card border border-dark-border disabled:opacity-50"
            disabled={page() >= Math.ceil(total() / 20)}
            onClick={() => setPage(p => p + 1)}
          >
            Next
          </button>
        </div>
      </Show>

      {/* Upload Modal */}
      <Show when={showUploadModal()}>
        <div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div class="bg-dark-card rounded-lg border border-dark-border w-full max-w-2xl max-h-[90vh] overflow-y-auto">
            <div class="px-6 py-4 border-b border-dark-border flex justify-between items-center">
              <h2 class="text-lg font-semibold">Upload Document</h2>
              <button
                class="text-dark-muted hover:text-dark-text"
                onClick={() => setShowUploadModal(false)}
              >
                X
              </button>
            </div>
            <div class="px-6 py-4 space-y-4">
              {/* Routing option */}
              <div class="flex items-center gap-2">
                <input
                  type="checkbox"
                  id="useRouting"
                  checked={uploadForm().useRouting}
                  onChange={(e) => setUploadForm(f => ({ ...f, useRouting: e.currentTarget.checked }))}
                />
                <label for="useRouting" class="text-sm">
                  Use intelligent routing (auto-select dataset based on tags/category)
                </label>
              </div>

              {/* Filename */}
              <div>
                <label class="block text-sm text-dark-muted mb-1">Filename *</label>
                <input
                  type="text"
                  class="w-full bg-dark-bg border border-dark-border rounded-lg px-4 py-2"
                  placeholder="document.md"
                  value={uploadForm().filename}
                  onInput={(e) => setUploadForm(f => ({ ...f, filename: e.currentTarget.value }))}
                />
              </div>

              {/* Title */}
              <div>
                <label class="block text-sm text-dark-muted mb-1">Title</label>
                <input
                  type="text"
                  class="w-full bg-dark-bg border border-dark-border rounded-lg px-4 py-2"
                  placeholder="Document title"
                  value={uploadForm().title}
                  onInput={(e) => setUploadForm(f => ({ ...f, title: e.currentTarget.value }))}
                />
              </div>

              {/* URL */}
              <div>
                <label class="block text-sm text-dark-muted mb-1">Source URL</label>
                <div class="flex gap-2">
                  <input
                    type="text"
                    class="flex-1 bg-dark-bg border border-dark-border rounded-lg px-4 py-2"
                    placeholder="https://example.com/article"
                    value={uploadForm().url}
                    onInput={(e) => {
                      setUploadForm(f => ({ ...f, url: e.currentTarget.value }))
                      setUrlCheckResult(null)
                    }}
                  />
                  <button
                    class="px-4 py-2 bg-dark-border hover:bg-dark-muted/30 rounded-lg"
                    onClick={handleCheckUrl}
                  >
                    Check
                  </button>
                </div>
                <Show when={urlCheckResult()}>
                  <p class={urlCheckResult()!.exists ? 'text-yellow-400 text-sm mt-1' : 'text-green-400 text-sm mt-1'}>
                    {urlCheckResult()!.exists ? 'URL already exists in database' : 'URL is new'}
                  </p>
                </Show>
              </div>

              {/* Tags (for routing) */}
              <Show when={uploadForm().useRouting}>
                <div>
                  <label class="block text-sm text-dark-muted mb-1">Tags (comma-separated)</label>
                  <input
                    type="text"
                    class="w-full bg-dark-bg border border-dark-border rounded-lg px-4 py-2"
                    placeholder="security, web, vulnerability"
                    value={uploadForm().tags}
                    onInput={(e) => setUploadForm(f => ({ ...f, tags: e.currentTarget.value }))}
                  />
                </div>

                <div>
                  <label class="block text-sm text-dark-muted mb-1">Category (fallback for routing)</label>
                  <input
                    type="text"
                    class="w-full bg-dark-bg border border-dark-border rounded-lg px-4 py-2"
                    placeholder="security"
                    value={uploadForm().category}
                    onInput={(e) => setUploadForm(f => ({ ...f, category: e.currentTarget.value }))}
                  />
                </div>
              </Show>

              {/* Content */}
              <div>
                <label class="block text-sm text-dark-muted mb-1">Content *</label>
                <textarea
                  class="w-full bg-dark-bg border border-dark-border rounded-lg px-4 py-2 h-48 font-mono text-sm"
                  placeholder="Document content..."
                  value={uploadForm().content}
                  onInput={(e) => setUploadForm(f => ({ ...f, content: e.currentTarget.value }))}
                />
              </div>
            </div>
            <div class="px-6 py-4 border-t border-dark-border flex justify-end gap-3">
              <button
                class="px-4 py-2 border border-dark-border rounded-lg hover:bg-dark-border"
                onClick={() => setShowUploadModal(false)}
              >
                Cancel
              </button>
              <button
                class="px-4 py-2 bg-primary-600 hover:bg-primary-700 text-white rounded-lg disabled:opacity-50"
                disabled={uploading()}
                onClick={handleUpload}
              >
                {uploading() ? 'Uploading...' : 'Upload'}
              </button>
            </div>
          </div>
        </div>
      </Show>
    </div>
  )
}

export default Documents

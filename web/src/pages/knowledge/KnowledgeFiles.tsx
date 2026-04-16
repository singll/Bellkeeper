import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { knowledgeFilesApi } from '@/api/knowledge'
import { useToast } from '@/components/Toast'
import Modal from '@/components/Modal'
import type { TreeNode, KnowledgeFileEntry, FileContent } from '@/types'

const KnowledgeFiles: Component = () => {
  const toast = useToast()

  // State
  const [currentPath, setCurrentPath] = createSignal('')
  const [expandedDirs, setExpandedDirs] = createSignal<Set<string>>(new Set())
  const [selectedFile, setSelectedFile] = createSignal<KnowledgeFileEntry | null>(null)
  const [showPreview, setShowPreview] = createSignal(false)

  // Load directory tree
  const [tree, { refetch: refetchTree }] = createResource(() =>
    knowledgeFilesApi.getTree('')
  )

  // Load files in current directory
  const [files, { refetch: refetchFiles }] = createResource(
    () => currentPath(),
    (path) => knowledgeFilesApi.listFiles(path)
  )

  // Load file content for preview
  const [fileContent] = createResource(
    () => selectedFile(),
    (file) => file ? knowledgeFilesApi.readFile(file.path) : Promise.resolve(null)
  )

  // Stats
  const [stats] = createResource(() => knowledgeFilesApi.getStats())

  // Toggle directory expansion
  const toggleDir = (path: string) => {
    const expanded = new Set(expandedDirs())
    if (expanded.has(path)) {
      expanded.delete(path)
    } else {
      expanded.add(path)
    }
    setExpandedDirs(expanded)
  }

  // Navigate to directory
  const navigateTo = (path: string) => {
    setCurrentPath(path)
  }

  // Get breadcrumb items
  const getBreadcrumbs = () => {
    if (!currentPath()) return [{ name: '/', path: '' }]
    const parts = currentPath().split('/').filter(Boolean)
    const crumbs = [{ name: '/', path: '' }]
    let accPath = ''
    for (const part of parts) {
      accPath += '/' + part
      crumbs.push({ name: part, path: accPath })
    }
    return crumbs
  }

  // Format file size
  const formatSize = (bytes: number) => {
    if (bytes < 1024) return `${bytes} B`
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  }

  // Format date
  const formatDate = (dateStr: string) => {
    return new Date(dateStr).toLocaleString('zh-CN')
  }

  // Open file preview
  const openPreview = (file: KnowledgeFileEntry) => {
    setSelectedFile(file)
    setShowPreview(true)
  }

  // Render tree node recursively
  const renderTreeNode = (node: TreeNode, depth = 0) => {
    const isExpanded = expandedDirs().has(node.path)
    const isActive = currentPath() === node.path

    return (
      <div>
        <div
          class={`tree-node ${isActive ? 'active' : ''}`}
          style={{ "padding-left": `${depth * 16 + 8}px` }}
          onClick={() => {
            if (node.type === 'dir') {
              toggleDir(node.path)
              navigateTo(node.path)
            } else {
              openPreview({
                name: node.name,
                path: node.path,
                size: node.size || 0,
                modified: node.modified || '',
                type: node.name.split('.').pop() || '',
                layer: '',
              })
            }
          }}
        >
          <span class="tree-icon">
            {node.type === 'dir' ? (isExpanded ? '📂' : '📁') : '📄'}
          </span>
          <span class="tree-name">{node.name}</span>
        </div>
        <Show when={node.type === 'dir' && isExpanded && node.children}>
          <For each={node.children}>
            {(child) => renderTreeNode(child, depth + 1)}
          </For>
        </Show>
      </div>
    )
  }

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">知识库文件</h1>
          <p class="text-sm text-dark-400 mt-1">浏览和管理知识库文件</p>
        </div>
        <Show when={stats()?.data}>
          <div class="flex gap-4 text-sm">
            <div class="stat-badge">
              <span class="stat-value">{stats()?.data?.total_files}</span>
              <span class="stat-label">文件</span>
            </div>
            <div class="stat-badge">
              <span class="stat-value">{formatSize(stats()?.data?.total_size || 0)}</span>
              <span class="stat-label">总大小</span>
            </div>
          </div>
        </Show>
      </div>

      <div class="flex gap-6" style={{ "min-height": "600px" }}>
        {/* Left: Directory Tree */}
        <div class="w-64 flex-shrink-0">
          <div class="card p-4">
            <h3 class="text-sm font-medium text-dark-400 mb-3">目录树</h3>
            <div class="tree-view">
              <Show when={tree()?.data} fallback={
                <div class="text-dark-500 text-sm">加载中...</div>
              }>
                {renderTreeNode(tree()!.data)}
              </Show>
            </div>
          </div>
        </div>

        {/* Right: File List */}
        <div class="flex-1">
          <div class="card overflow-hidden">
            {/* Breadcrumb */}
            <div class="flex items-center gap-2 px-4 py-3 border-b border-dark-700 text-sm">
              <For each={getBreadcrumbs()}>
                {(crumb, i) => (
                  <>
                    <Show when={i() > 0}>
                      <span class="text-dark-500">/</span>
                    </Show>
                    <button
                      class={`${crumb.path === currentPath() ? 'text-primary-400' : 'text-dark-400 hover:text-white'} transition-colors`}
                      onClick={() => navigateTo(crumb.path)}
                    >
                      {crumb.name}
                    </button>
                  </>
                )}
              </For>
            </div>

            {/* File Table */}
            <div class="overflow-x-auto">
              <table class="table">
                <thead>
                  <tr>
                    <th>名称</th>
                    <th>大小</th>
                    <th>类型</th>
                    <th>修改时间</th>
                  </tr>
                </thead>
                <tbody>
                  <Show
                    when={!files.loading && files()?.data}
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
                      when={(files()?.data?.length ?? 0) > 0}
                      fallback={
                        <tr>
                          <td colspan="4">
                            <div class="empty-state">
                              <p class="empty-state-title">此目录为空</p>
                              <p class="empty-state-description">选择一个子目录查看文件</p>
                            </div>
                          </td>
                        </tr>
                      }
                    >
                      <For each={files()?.data ?? []}>
                        {(file) => (
                          <tr
                            class="cursor-pointer hover:bg-dark-700/50"
                            onClick={() => openPreview(file)}
                          >
                            <td>
                              <div class="flex items-center gap-2">
                                <span>📄</span>
                                <span class="font-medium text-white">{file.name}</span>
                              </div>
                            </td>
                            <td class="text-dark-400">{formatSize(file.size)}</td>
                            <td>
                              <span class="badge badge-gray">{file.type}</span>
                            </td>
                            <td class="text-dark-400 text-sm">
                              {formatDate(file.modified)}
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
        </div>
      </div>

      {/* File Preview Modal */}
      <Modal
        open={showPreview()}
        onClose={() => setShowPreview(false)}
        title={selectedFile()?.name || '文件预览'}
        size="xl"
      >
        <Show when={fileContent()?.data}>
          <div class="max-h-[60vh] overflow-auto">
            <pre class="text-sm text-dark-300 whitespace-pre-wrap font-mono bg-dark-800 p-4 rounded">
              {fileContent()?.data?.content}
            </pre>
          </div>
          <div class="mt-4 text-sm text-dark-400 flex gap-4">
            <span>大小: {formatSize(fileContent()?.data?.size || 0)}</span>
            <span>路径: {fileContent()?.data?.path}</span>
          </div>
        </Show>
        <Show when={fileContent.loading}>
          <div class="flex justify-center py-8">
            <div class="loading-spinner" />
          </div>
        </Show>
      </Modal>
    </div>
  )
}

export default KnowledgeFiles

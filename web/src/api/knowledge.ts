import type {
  TreeNode,
  KnowledgeFileEntry,
  FileContent,
  FilesStats,
  KnowledgeSearchResult,
  KnowledgeSearchHit,
  KnowledgeAskRequest,
  KnowledgeAskResponse,
} from '@/types'

const API_BASE = '/api'

async function request<T>(
  endpoint: string,
  options: RequestInit = {}
): Promise<T> {
  const response = await fetch(`${API_BASE}${endpoint}`, {
    headers: {
      'Content-Type': 'application/json',
      ...options.headers,
    },
    ...options,
  })

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: 'Unknown error' }))
    throw new Error(error.error || `HTTP ${response.status}`)
  }

  return response.json()
}

// Knowledge Files API
export const knowledgeFilesApi = {
  // Get directory tree
  getTree: (path = '') =>
    request<{ data: TreeNode }>(`/knowledge/files/tree?path=${encodeURIComponent(path)}`),

  // List files in directory
  listFiles: (path = '', layer?: string) => {
    const params = new URLSearchParams({ path })
    if (layer) params.set('layer', layer)
    return request<{ data: KnowledgeFileEntry[] }>(`/knowledge/files/list?${params}`)
  },

  // Read file content
  readFile: (path: string) =>
    request<{ data: FileContent }>(`/knowledge/files/read?path=${encodeURIComponent(path)}`),

  // Get file statistics
  getStats: () => request<{ data: FilesStats }>('/knowledge/files/stats'),

  // Search files by name
  searchFiles: (q: string) =>
    request<{ data: KnowledgeFileEntry[] }>(`/knowledge/files/search?q=${encodeURIComponent(q)}`),
}

// Knowledge Search API (Meilisearch-based)
export const knowledgeSearchApi = {
  search: (params: {
    query: string
    layers?: string[]
    categories?: string[]
    tags?: string[]
    limit?: number
  }) => {
    const { query, layers, categories, tags, limit = 20 } = params
    const body = JSON.stringify({ query, layers, categories, tags, limit })
    return request<{ data: KnowledgeSearchResult }>('/files/search', {
      method: 'POST',
      body,
    })
  },
}

// Knowledge Ask API (RAG-based)
export const knowledgeAskApi = {
  ask: (params: KnowledgeAskRequest) =>
    request<{ data: KnowledgeAskResponse }>('/files/ask', {
      method: 'POST',
      body: JSON.stringify(params),
    }),
}

// Knowledge Index API
export const knowledgeIndexApi = {
  getStats: () => request<{ data: { indexed_count: number; is_indexing: boolean; last_indexed_at?: string } }>('/files/stats'),
  rebuild: () =>
    request<{ message: string }>('/files/rebuild', { method: 'POST' }),
  health: () => request<{ status: string; meilisearch: string }>('/files/health'),
}

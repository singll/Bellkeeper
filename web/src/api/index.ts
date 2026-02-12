import type {
  Tag,
  DataSource,
  RSSFeed,
  WebhookConfig,
  WebhookHistory,
  DatasetMapping,
  Setting,
  PaginatedResponse,
  HealthStatus,
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

// Tags API
export const tagsApi = {
  list: (page = 1, perPage = 20, keyword = '') =>
    request<PaginatedResponse<Tag>>(
      `/tags?page=${page}&per_page=${perPage}&keyword=${encodeURIComponent(keyword)}`
    ),

  get: (id: number) =>
    request<{ data: Tag }>(`/tags/${id}`),

  create: (data: Partial<Tag>) =>
    request<{ data: Tag }>('/tags', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  update: (id: number, data: Partial<Tag>) =>
    request<{ data: Tag }>(`/tags/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  delete: (id: number) =>
    request<{ message: string }>(`/tags/${id}`, { method: 'DELETE' }),
}

// Data Sources API
export const dataSourcesApi = {
  list: (page = 1, perPage = 20, category = '', keyword = '') =>
    request<PaginatedResponse<DataSource>>(
      `/datasources?page=${page}&per_page=${perPage}&category=${encodeURIComponent(category)}&keyword=${encodeURIComponent(keyword)}`
    ),

  get: (id: number) =>
    request<{ data: DataSource }>(`/datasources/${id}`),

  create: (data: Partial<DataSource> & { tag_ids?: number[] }) =>
    request<{ data: DataSource }>('/datasources', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  update: (id: number, data: Partial<DataSource> & { tag_ids?: number[] }) =>
    request<{ data: DataSource }>(`/datasources/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  delete: (id: number) =>
    request<{ message: string }>(`/datasources/${id}`, { method: 'DELETE' }),
}

// RSS Feeds API
export const rssApi = {
  list: (page = 1, perPage = 20, category = '', keyword = '') =>
    request<PaginatedResponse<RSSFeed>>(
      `/rss?page=${page}&per_page=${perPage}&category=${encodeURIComponent(category)}&keyword=${encodeURIComponent(keyword)}`
    ),

  get: (id: number) =>
    request<{ data: RSSFeed }>(`/rss/${id}`),

  create: (data: Partial<RSSFeed> & { tag_ids?: number[] }) =>
    request<{ data: RSSFeed }>('/rss', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  update: (id: number, data: Partial<RSSFeed> & { tag_ids?: number[] }) =>
    request<{ data: RSSFeed }>(`/rss/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  delete: (id: number) =>
    request<{ message: string }>(`/rss/${id}`, { method: 'DELETE' }),
}

// Webhooks API
export const webhooksApi = {
  list: (page = 1, perPage = 20) =>
    request<PaginatedResponse<WebhookConfig>>(
      `/webhooks?page=${page}&per_page=${perPage}`
    ),

  get: (id: number) =>
    request<{ data: WebhookConfig }>(`/webhooks/${id}`),

  create: (data: Partial<WebhookConfig>) =>
    request<{ data: WebhookConfig }>('/webhooks', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  update: (id: number, data: Partial<WebhookConfig>) =>
    request<{ data: WebhookConfig }>(`/webhooks/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  delete: (id: number) =>
    request<{ message: string }>(`/webhooks/${id}`, { method: 'DELETE' }),

  trigger: (id: number, payload?: Record<string, unknown>) =>
    request<{ data: WebhookHistory }>(`/webhooks/${id}/trigger`, {
      method: 'POST',
      body: JSON.stringify(payload || {}),
    }),

  history: (id: number, limit = 20) =>
    request<{ data: WebhookHistory[] }>(`/webhooks/${id}/history?limit=${limit}`),
}

// Datasets API
export const datasetsApi = {
  list: (page = 1, perPage = 20) =>
    request<PaginatedResponse<DatasetMapping>>(
      `/datasets?page=${page}&per_page=${perPage}`
    ),

  get: (id: number) =>
    request<{ data: DatasetMapping }>(`/datasets/${id}`),

  create: (data: Partial<DatasetMapping> & { tag_ids?: number[] }) =>
    request<{ data: DatasetMapping }>('/datasets', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  update: (id: number, data: Partial<DatasetMapping> & { tag_ids?: number[] }) =>
    request<{ data: DatasetMapping }>(`/datasets/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  delete: (id: number) =>
    request<{ message: string }>(`/datasets/${id}`, { method: 'DELETE' }),
}

// Settings API
export const settingsApi = {
  list: (category = '') =>
    request<{ data: Setting[] }>(`/settings?category=${encodeURIComponent(category)}`),

  get: (key: string) =>
    request<{ data: Setting }>(`/settings/${encodeURIComponent(key)}`),

  update: (key: string, data: { value: string; value_type?: string; category?: string; description?: string; is_secret?: boolean }) =>
    request<{ message: string }>(`/settings/${encodeURIComponent(key)}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
}

// Health API
export const healthApi = {
  check: () => request<HealthStatus>('/health'),
  detailed: () => request<HealthStatus>('/health/detailed'),
}

// Workflows API
export const workflowsApi = {
  status: () => request<{ data: { name: string; active: boolean }[] }>('/workflows/status'),
  trigger: (name: string, payload?: Record<string, unknown>) =>
    request<{ data: unknown }>(`/workflows/trigger/${encodeURIComponent(name)}`, {
      method: 'POST',
      body: JSON.stringify(payload || {}),
    }),
}

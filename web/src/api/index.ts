import type {
  Tag,
  RSSFeed,
  DatasetMapping,
  Setting,
  PaginatedResponse,
  HealthStatus,
  Workflow,
  WorkflowExecution,
  LLMChannelStatus,
  LLMGroupStatus,
  LLMChannelConfig,
  LLMModelGroupConfig,
  LLMProxyLog,
  ActivityLogsPage,
  ModuleStat,
  MatrixRoom,
  MatrixChannel,
  MatrixCommand,
  MatrixNotification,
  MatrixEvent,
  MatrixCommandLog,
  MatrixStats,
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

// System API
export const systemApi = {
  restart: () =>
    request<{ message: string }>('/system/restart', { method: 'POST' }),
}

// Workflows API
export const workflowsApi = {
  list: () => request<{ data: Workflow[] }>('/workflows/status'),

  get: (id: string) => request<{ data: Workflow }>(`/workflows/${encodeURIComponent(id)}`),

  activate: (id: string) =>
    request<{ message: string }>(`/workflows/${encodeURIComponent(id)}/activate`, {
      method: 'POST',
    }),

  deactivate: (id: string) =>
    request<{ message: string }>(`/workflows/${encodeURIComponent(id)}/deactivate`, {
      method: 'POST',
    }),

  executions: (workflowId?: string, limit = 20) => {
    const params = new URLSearchParams({ limit: String(limit) })
    if (workflowId) params.set('workflow_id', workflowId)
    return request<{ data: WorkflowExecution[] }>(`/workflows/executions?${params}`)
  },

  trigger: (name: string, payload?: Record<string, unknown>) =>
    request<{ data: unknown }>(`/workflows/trigger/${encodeURIComponent(name)}`, {
      method: 'POST',
      body: JSON.stringify(payload || {}),
    }),
}

// LLM Proxy API
export const llmProxyApi = {
  channelsStatus: () =>
    request<{ data: LLMChannelStatus[] }>('/llm/channels/status'),

  groupsStatus: () =>
    request<{ data: LLMGroupStatus[] }>('/llm/groups/status'),

  resetChannelCircuit: (name: string) =>
    request<{ message: string }>(`/llm/channels/${encodeURIComponent(name)}/reset`, {
      method: 'POST',
    }),

  clearGroupSticky: (name: string) =>
    request<{ data: { cleared: number } }>(`/llm/groups/${encodeURIComponent(name)}/sticky`, {
      method: 'DELETE',
    }),

  // Config CRUD
  listChannels: () =>
    request<{ data: LLMChannelConfig[] }>('/llm/config/channels'),

  createChannel: (data: Partial<LLMChannelConfig>) =>
    request<{ data: LLMChannelConfig }>('/llm/config/channels', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  updateChannel: (id: number, data: Partial<LLMChannelConfig>) =>
    request<{ data: LLMChannelConfig }>(`/llm/config/channels/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteChannel: (id: number) =>
    request<{ message: string }>(`/llm/config/channels/${id}`, { method: 'DELETE' }),

  listGroups: () =>
    request<{ data: LLMModelGroupConfig[] }>('/llm/config/groups'),

  createGroup: (data: Partial<LLMModelGroupConfig>) =>
    request<{ data: LLMModelGroupConfig }>('/llm/config/groups', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  updateGroup: (id: number, data: Partial<LLMModelGroupConfig>) =>
    request<{ data: LLMModelGroupConfig }>(`/llm/config/groups/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteGroup: (id: number) =>
    request<{ message: string }>(`/llm/config/groups/${id}`, { method: 'DELETE' }),

  reload: () =>
    request<{ message: string }>('/llm/reload', { method: 'POST' }),

  logs: (channel?: string, limit?: number) => {
    const p = new URLSearchParams()
    if (channel) p.set('channel', channel)
    if (limit) p.set('limit', String(limit))
    return request<{ data: LLMProxyLog[] }>(`/llm/logs?${p}`)
  },

  // Token CRUD
  listTokens: () => request<{ data: LLMToken[] }>('/llm/tokens'),
  createToken: (data: Partial<LLMToken>) =>
    request<{ token: LLMToken; key: string }>('/llm/tokens', { method: 'POST', body: JSON.stringify(data) }),
  updateToken: (id: number, data: Partial<LLMToken>) =>
    request<{ data: LLMToken }>(`/llm/tokens/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deleteToken: (id: number) =>
    request<{ message: string }>(`/llm/tokens/${id}`, { method: 'DELETE' }),
  regenerateTokenKey: (id: number) =>
    request<{ key: string }>(`/llm/tokens/${id}/regenerate`, { method: 'POST' }),
  getTokenUsage: (id: number, days?: number) =>
    request<{ data: LLMTokenUsageDaily[] }>(`/llm/tokens/${id}/usage?days=${days || 7}`),

  // Pricing CRUD
  listPricing: () => request<{ data: LLMModelPricing[] }>('/llm/pricing'),
  createPricing: (data: Partial<LLMModelPricing>) =>
    request<{ data: LLMModelPricing }>('/llm/pricing', { method: 'POST', body: JSON.stringify(data) }),
  updatePricing: (id: number, data: Partial<LLMModelPricing>) =>
    request<{ data: LLMModelPricing }>(`/llm/pricing/${id}`, { method: 'PUT', body: JSON.stringify(data) }),
  deletePricing: (id: number) =>
    request<{ message: string }>(`/llm/pricing/${id}`, { method: 'DELETE' }),
  testPricingCalc: (data: { channel_name: string; model: string; prompt_tokens: number; completion_tokens: number; cached_tokens: number }) =>
    request<{ cost_cents: number; cost_usd: string }>('/llm/pricing/test-calc', { method: 'POST', body: JSON.stringify(data) }),
}

// Activity Logs API
export const logsApi = {
  list: (params: { module?: string; status?: string; ref_id?: string; since?: string; page?: number; limit?: number } = {}) => {
    const p = new URLSearchParams()
    if (params.module) p.set('module', params.module)
    if (params.status) p.set('status', params.status)
    if (params.ref_id) p.set('ref_id', params.ref_id)
    if (params.since) p.set('since', params.since)
    if (params.page) p.set('page', String(params.page))
    if (params.limit) p.set('limit', String(params.limit))
    return request<{ data: ActivityLogsPage }>(`/logs?${p}`)
  },

  modules: () =>
    request<{ data: string[] }>('/logs/modules'),

  stats: (since?: string) => {
    const p = since ? `?since=${encodeURIComponent(since)}` : ''
    return request<{ data: ModuleStat[] }>(`/logs/stats${p}`)
  },
}

// Matrix Platform API
export const matrixApi = {
  // Stats - returns stats directly, not wrapped in { data: ... }
  getStats: () =>
    request<MatrixStats>('/matrix/admin/stats'),

  // Rooms
  listRooms: (params?: { page?: number; page_size?: number }) => {
    const p = new URLSearchParams()
    if (params?.page) p.set('page', String(params.page))
    if (params?.page_size) p.set('page_size', String(params.page_size))
    return request<{ data: PaginatedResponse<MatrixRoom> }>(`/matrix/admin/rooms?${p}`)
  },

  createRoom: (data: Partial<MatrixRoom>) =>
    request<{ data: MatrixRoom }>('/matrix/admin/rooms', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  updateRoom: (id: number, data: Partial<MatrixRoom>) =>
    request<{ data: MatrixRoom }>(`/matrix/admin/rooms/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteRoom: (id: number) =>
    request<{ message: string }>(`/matrix/admin/rooms/${id}`, { method: 'DELETE' }),

  // Channels
  listChannels: () =>
    request<{ data: MatrixChannel[] }>('/matrix/admin/channels'),

  createChannel: (data: Partial<MatrixChannel>) =>
    request<{ data: MatrixChannel }>('/matrix/admin/channels', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  updateChannel: (id: number, data: Partial<MatrixChannel>) =>
    request<{ data: MatrixChannel }>(`/matrix/admin/channels/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteChannel: (id: number) =>
    request<{ message: string }>(`/matrix/admin/channels/${id}`, { method: 'DELETE' }),

  // Commands
  listCommands: () =>
    request<{ data: MatrixCommand[] }>('/matrix/admin/commands'),

  createCommand: (data: Partial<MatrixCommand>) =>
    request<{ data: MatrixCommand }>('/matrix/admin/commands', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  updateCommand: (id: number, data: Partial<MatrixCommand>) =>
    request<{ data: MatrixCommand }>(`/matrix/admin/commands/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteCommand: (id: number) =>
    request<{ message: string }>(`/matrix/admin/commands/${id}`, { method: 'DELETE' }),

  testCommand: (id: number, data: { room_id: string; user_id: string; args: string }) =>
    request<{ message: string; response?: string; duration_ms?: number }>(
      `/matrix/admin/commands/${id}/test`,
      { method: 'POST', body: JSON.stringify(data) }
    ),

  // Notifications
  listNotifications: (params?: { page?: number; page_size?: number; channel?: string; status?: string }) => {
    const p = new URLSearchParams()
    if (params?.page) p.set('page', String(params.page))
    if (params?.page_size) p.set('page_size', String(params.page_size))
    if (params?.channel) p.set('channel', params.channel)
    if (params?.status) p.set('status', params.status)
    return request<{ data: PaginatedResponse<MatrixNotification> }>(`/matrix/admin/notifications?${p}`)
  },

  getNotification: (id: number) =>
    request<{ data: MatrixNotification }>(`/matrix/admin/notifications/${id}`),

  retryNotification: (id: number) =>
    request<{ message: string }>(`/matrix/admin/notifications/${id}/retry`, { method: 'POST' }),

  // Events
  listEvents: (params?: { page?: number; page_size?: number; event_type?: string; room_id?: string }) => {
    const p = new URLSearchParams()
    if (params?.page) p.set('page', String(params.page))
    if (params?.page_size) p.set('page_size', String(params.page_size))
    if (params?.event_type) p.set('event_type', params.event_type)
    if (params?.room_id) p.set('room_id', params.room_id)
    return request<{ data: PaginatedResponse<MatrixEvent> }>(`/matrix/admin/events?${p}`)
  },

  getEvent: (id: number) =>
    request<{ data: MatrixEvent }>(`/matrix/admin/events/${id}`),

  // Command Logs
  listCommandLogs: (params?: { page?: number; per_page?: number; command?: string; status?: string }) => {
    const p = new URLSearchParams()
    if (params?.page) p.set('page', String(params.page))
    if (params?.per_page) p.set('per_page', String(params.per_page))
    if (params?.command) p.set('command', params.command)
    if (params?.status) p.set('status', params.status)
    return request<PaginatedResponse<MatrixCommandLog>>(`/matrix/admin/command-logs?${p}`)
  },
}

// Search API
export interface SearchResultTag {
  id: number
  name: string
  color?: string
  description?: string
}

export interface SearchResultDocument {
  id: string
  article_title: string
  article_url?: string
}

export interface SearchResultRSSFeed {
  id: number
  name: string
  description?: string
}

export interface SearchResult {
  tags: SearchResultTag[]
  documents: SearchResultDocument[]
  rss_feeds: SearchResultRSSFeed[]
  total_count: number
}

export const search = async (
  query: string,
  scope: 'all' | 'tags' | 'documents' | 'rss' = 'all',
  limit = 10
): Promise<SearchResult> => {
  const response = await request<{ success: boolean; data: SearchResult }>(
    `/search?q=${encodeURIComponent(query)}&scope=${scope}&limit=${limit}`
  )
  return response.data
}

// LogCenter API
export const logCenterApi = {
  listEntries: (params: { module?: string; level?: string; status?: string; keyword?: string; source_id?: number; page?: number; limit?: number } = {}) => {
    const p = new URLSearchParams()
    if (params.module) p.set('module', params.module)
    if (params.level) p.set('level', params.level)
    if (params.status) p.set('status', params.status)
    if (params.keyword) p.set('keyword', params.keyword)
    if (params.source_id) p.set('source_id', String(params.source_id))
    if (params.page) p.set('page', String(params.page))
    if (params.limit) p.set('limit', String(params.limit))
    return request<{ data: { items: import('@/types').LogEntry[]; total: number; page: number; limit: number } }>(`/logs/entries?${p}`)
  },

  getEntry: (id: number) =>
    request<{ data: import('@/types').LogEntry }>(`/logs/entries/${id}`),

  // Sources
  listSources: () =>
    request<{ data: import('@/types').LogSource[] }>('/logs/sources'),

  registerSource: (data: { name: string; source_type: string; description?: string }) =>
    request<{ data: import('@/types').LogSource & { api_key?: string } }>('/logs/sources', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  updateSource: (id: number, data: Partial<{ name: string; source_type: string; description: string; is_active: boolean }>) =>
    request<{ data: import('@/types').LogSource }>(`/logs/sources/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteSource: (id: number) =>
    request<{ message: string }>(`/logs/sources/${id}`, { method: 'DELETE' }),

  // Dashboard
  getDashboard: (period = '24h') =>
    request<{ data: import('@/types').DashboardData }>(`/logs/dashboard/${period}`),

  // Alerts
  listAlerts: () =>
    request<{ data: import('@/types').LogAlertRule[] }>('/logs/alerts'),

  createAlert: (data: { name: string; condition: Record<string, unknown>; notify_channel: string }) =>
    request<{ data: import('@/types').LogAlertRule }>('/logs/alerts', {
      method: 'POST',
      body: JSON.stringify(data),
    }),

  updateAlert: (id: number, data: Partial<{ name: string; condition: Record<string, unknown>; notify_channel: string; is_active: boolean }>) =>
    request<{ data: import('@/types').LogAlertRule }>(`/logs/alerts/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),

  deleteAlert: (id: number) =>
    request<{ message: string }>(`/logs/alerts/${id}`, { method: 'DELETE' }),
}

// API Types

export interface Tag {
  id: number
  name: string
  description: string
  color: string
  created_at: string
  updated_at: string
}

export interface DataSource {
  id: number
  name: string
  url: string
  type: string
  category: string
  description: string
  is_active: boolean
  tags: Tag[]
  created_at: string
  updated_at: string
}

export interface RSSFeed {
  id: number
  name: string
  url: string
  category: string
  description: string
  is_active: boolean
  last_fetched_at: string | null
  fetch_interval_minutes: number
  tags: Tag[]
  created_at: string
  updated_at: string
}

export interface WebhookConfig {
  id: number
  name: string
  url: string
  method: string
  content_type: string
  headers: Record<string, string>
  body_template: string
  timeout_seconds: number
  description: string
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface WebhookHistory {
  id: number
  webhook_id: number
  request_url: string
  request_method: string
  request_body: string
  status: string
  response_code: number
  response_body: string
  duration_ms: number
  error_message: string
  created_at: string
}

export interface DatasetMapping {
  id: number
  name: string
  display_name: string
  dataset_id: string
  description: string
  is_default: boolean
  is_active: boolean
  parser_id: string
  tags: Tag[]
  created_at: string
  updated_at: string
}

export interface Setting {
  id: number
  key: string
  value: string
  value_type: string
  category: string
  description: string
  is_secret: boolean
}

export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  per_page: number
}

export interface HealthStatus {
  status: string
  version?: string
  services?: Record<string, ServiceStatus>
  metrics?: Record<string, unknown>
}

export interface ServiceStatus {
  status: string
  latency_ms?: number
  error?: string
}

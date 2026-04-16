// API Types

export interface Tag {
  id: number
  name: string
  description: string
  color: string
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

export interface Workflow {
  id: string
  name: string
  active: boolean
  created_at?: string
  updated_at?: string
  tags?: WorkflowTag[]
  meta?: Record<string, unknown>
}

export interface WorkflowTag {
  id: string
  name: string
}

export interface WorkflowExecution {
  id: string
  workflow_id: string
  finished: boolean
  status: string
  started_at: string
  stopped_at?: string
}

// LLM Proxy Types

export interface LLMChannelHealth {
  state: 'closed' | 'open' | 'half_open'
  consecutive_fails: number
  last_error_type: string
  recent_success_rate: number
  last_success_at?: string
  last_error_at?: string
  circuit_open_until?: string
}

export interface LLMChannelStatus {
  name: string
  base_url: string
  models: string[]
  priority: number
  rpm_limit: number
  rpd_limit: number
  is_free: boolean
  available_tokens: number
  max_tokens: number
  daily_used: number
  daily_limit: number
  refill_rate_per_s: string
  health: LLMChannelHealth
}

export interface LLMGroupMemberStatus {
  channel: string
  model: string
  weight: number
  available: boolean
  health: LLMChannelHealth
}

export interface LLMGroupStatus {
  name: string
  description: string
  strategy: string
  sticky_ttl_seconds: number
  sticky_bindings: number
  members: LLMGroupMemberStatus[]
}

// LLM Proxy Config Types (DB-backed)

export interface LLMChannelConfig {
  id: number
  name: string
  base_url: string
  api_key_env: string
  rpm: number
  rpd: number
  priority: number
  is_free: boolean
  is_enabled: boolean
  models: string // JSON array string
  created_at: string
  updated_at: string
}

export interface LLMModelGroupConfig {
  id: number
  name: string
  description: string
  strategy: string
  sticky_ttl_seconds: number
  members: LLMModelGroupMemberConfig[]
  created_at: string
  updated_at: string
}

export interface LLMModelGroupMemberConfig {
  id?: number
  group_id?: number
  channel_name: string
  model: string
  weight: number
}

// Activity Log Types

export interface ActivityLog {
  id: number
  module: string
  action: string
  status: string
  summary: string
  detail?: string
  ref_id?: string
  duration_ms?: number
  created_at: string
}

export interface ActivityLogsPage {
  items: ActivityLog[]
  total: number
  page: number
  limit: number
}

export interface ModuleStat {
  module: string
  count: number
}

// Parse Task Types

export interface ParseDocState {
  dataset_id: string
  document_id: string
  current_status: string
  stage: string
  submitted_at?: string
  last_progress_at?: string
  last_state_change_at?: string
  recovery_attempts: number
  last_error?: string
}

export interface ParseTask {
  id: string
  status: string
  total: number
  completed: number
  failed: number
  pending: number
  batch_size: number
  running_count: number
  recovering_count: number
  succeeded_count: number
  final_failed_count: number
  current_dataset_id?: string
  current_batch_index?: number
  current_stage?: string
  result_status?: string
  failed_docs?: { dataset_id: string; document_id: string; error: string; retries: number }[]
  suspected_stuck_docs?: string[]
  doc_states?: ParseDocState[]
  started_at: string
  last_progress_at?: string
  completed_at?: string
  log?: string[]
}

// Matrix Platform Types

export interface MatrixRoom {
  room_id: string
  room_name: string
  room_type: string
  is_active: boolean
}

export interface MatrixChannel {
  name: string
  room_id: string
  is_active: boolean
  priority: number
  config?: Record<string, unknown>
}

export interface MatrixCommand {
  name: string
  handler_type: string
  permission_level: string
  room_scope: string
  is_active: boolean
  description?: string
  usage_example?: string
}

export interface MatrixNotification {
  id: number
  channel_name: string
  status: string
  retry_count: number
  message_content?: string
  error_message?: string
  created_at: string
  sent_at?: string
}

export interface MatrixEvent {
  event_id: string
  room_id: string
  sender: string
  type: string
  status: string
  created_at: string
}

export interface MatrixCommandLog {
  id: number
  event_id: string
  room_id: string
  sender: string
  command_name: string
  command_args?: string
  handler_type?: string
  execution_status: string
  execution_time_ms?: number
  error_message?: string
  response_event_id?: string
  created_at: string
  completed_at?: string
}

export interface MatrixStats {
  rooms: number
  channels: number
  commands: number
  events_24h: number
  notifications_24h: number
  active_rooms: number
}

// Knowledge Files Types

export interface TreeNode {
  name: string
  path: string
  type: 'dir' | 'file'
  children?: TreeNode[]
  size?: number
  modified?: string
}

export interface KnowledgeFileEntry {
  name: string
  path: string
  size: number
  modified: string
  type: string
  layer: string
}

export interface FileContent {
  path: string
  name: string
  content: string
  size: number
  modified: string
}

export interface FilesStats {
  total_files: number
  total_dirs: number
  total_size: number
  by_layer: Record<string, number>
  by_type: Record<string, number>
}

// Knowledge Search Types

export interface KnowledgeSearchHit {
  file_path: string
  title: string
  heading: string
  content: string
  layer: string
  category: string
  tags: string[]
  source_url?: string
  source_domain?: string
  highlights: string[]
}

export interface KnowledgeSearchResult {
  files: KnowledgeSearchHit[]
  total: number
  query_ms: number
}

// Knowledge Ask Types

export interface KnowledgeAskRequest {
  question: string
  layers?: string[]
  top_k?: number
}

export interface KnowledgeAskReference {
  title: string
  file_path: string
  source_url?: string
  snippet: string
}

export interface KnowledgeAskResponse {
  answer: string
  references: KnowledgeAskReference[]
  search_ms: number
  llm_ms?: number
}

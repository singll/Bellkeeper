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
  provider_type: string  // "openai" | "anthropic"
  rpm: number
  rpd: number
  priority: number
  is_free: boolean
  is_enabled: boolean
  models: string // JSON array string
  // Tier 4 capability tags / balance config (optional; backend defaults to empty string)
  balance_provider_type?: string
  balance_config_json?: string
  model_rpm_overrides?: string // JSON: {"model": rpm}
  task_types?: string // JSON array string, e.g. ["coding","analysis"]; empty = eligible for all task types
  tier?: string // "free" | "standard" | "premium"; empty = derived from is_free
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

// LLM Proxy Log Types

export interface LLMProxyLog {
  id: number
  channel_name: string
  model: string
  request_path: string
  status_code: number
  is_rate_limit: boolean
  retry_count: number
  duration_ms: number
  prompt_tokens: number
  comp_tokens: number
  cached_tokens: number
  cost_cents: number
  error_message: string
  caller_id: string
  created_at: string
}

export interface LLMToken {
  id: number
  name: string
  key_prefix: string
  caller_id: string
  allowed_models: string
  allowed_groups: string
  quota_requests_daily: number
  quota_tokens_daily: number
  quota_cost_monthly_cents: number
  expires_at: string | null
  enabled: boolean
  last_used_at: string | null
  created_at: string
  updated_at: string
}

export interface LLMTokenUsageDaily {
  id: number
  token_id: number
  date: string
  requests: number
  prompt_tokens: number
  completion_tokens: number
  cached_tokens: number
  cost_cents: number
  error_count: number
}

export interface LLMModelPricing {
  id: number
  channel_name: string
  model: string
  input_price_per_1m_cents: number
  output_price_per_1m_cents: number
  cached_input_price_per_1m_cents: number
  currency: string
  effective_from: string
  notes: string
  created_at: string
  updated_at: string
}

// LLM Proxy — Alert events (Tier 5 aggregator)

export interface LLMAlertEvent {
  id: number
  alert_type: string   // circuit_open | quota_threshold | balance_zero | session_expired | ...
  severity: string     // info | warning | error | critical
  channel_id: number
  channel_name: string
  message: string
  dedup_key: string
  flushed_at: string | null  // null until the alert has been delivered to a notifier
  created_at: string
}

// LLM Proxy — Balances (Tier 6). GET /llm/balances returns a map keyed by channel name.

export interface LLMBalanceInfo {
  provider_type: string
  channel_name: string
  balance: number
  currency: string
  total_granted: number
  total_used: number
  expires_at: string | null
  fetched_at: string
  error?: string
}

export interface LLMChannelBalanceSnapshot {
  id: number
  channel_id: number
  channel_name: string
  balance_usd: number
  currency: string
  total_granted: number
  total_used: number
  balance_raw: string  // full balance.Info JSON
  latency_ms: number
  fetched_at: string
  created_at: string
}

// LLM Proxy — Channel credentials (Tier 6, masked view; secret never leaves the server)

export interface LLMChannelCredentialView {
  id: number
  channel_id: number
  provider_type: string
  status: string  // active | error | expired
  error_message?: string
  last_refreshed_at: string | null
  created_at: string
  updated_at: string
  credential_preview: string  // masked, e.g. "abcd...wxyz"
}

// LLM Proxy — Conversation sticky bindings (Tier 2)

export interface LLMConversationBinding {
  id: number
  conversation_id: string
  channel_id: number
  channel_name: string
  model: string
  task_type: string
  first_seen_at: string
  last_seen_at: string
  expires_at: string
  request_count: number
  total_tokens: number
  total_cost_cents: number
}

// LLM Proxy — Adaptive rate-limit learning state (Tier 3)

export interface LLMModelRateLimit {
  id: number
  channel_id: number
  model: string
  configured_rpm: number
  configured_rpd: number
  learned_rpm_safe: number
  learned_rpd_safe: number
  learned_concurrent_max: number
  reset_pattern: string  // sliding_60s | fixed_minute | sliding_5h | sliding_7d | daily_utc8 | daily_utc
  confidence_score: number
  last_429_at: string | null
  last_429_observed_rpm: number
  last_adjust_at: string | null
  locked: boolean
  adjustment_log: string | null
  created_at: string
  updated_at: string
}

// LLM Proxy — Usage aggregation by model (Tier 1, group_by=model)

export interface LLMUsageByModel {
  model: string
  requests: number
  prompt_tokens: number
  completion_tokens: number
  cached_tokens: number
  cost_cents: number
  cost_micro_cents: number
  error_count: number
}

// LLM Proxy — Coding routing strategy (Tier 4)

export type LLMCodingStrategy = 'free_first' | 'quality_first' | 'complexity_aware'

// LLM Proxy Fetch Response Types (for raw fetch in LLMProxy.tsx)

export interface LLMProxyFetchResponse<T> {
  data: T[]
}

export interface LLMProxyChannelFetchResponse {
  data: LLMChannelStatus[]
}

export interface LLMProxyGroupFetchResponse {
  data: LLMGroupStatus[]
}

export interface LLMProxyChannelConfigFetchResponse {
  data: LLMChannelConfig[]
}

export interface LLMProxyGroupConfigFetchResponse {
  data: LLMModelGroupConfig[]
}

// LogCenter Types

export interface LogEntry {
  id: number
  source_id: number
  source?: LogSource
  module: string
  action: string
  level: string
  status: string
  summary: string
  detail?: Record<string, unknown>
  ref_id?: string
  duration_ms?: number
  trace_id?: string
  created_at: string
}

export interface LogSource {
  id: number
  name: string
  source_type: string
  description: string
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface LogAlertRule {
  id: number
  name: string
  condition: { module: string; level: string; threshold: number; window_minutes: number }
  notify_channel: string
  is_active: boolean
  created_at: string
}

export interface DashboardData {
  by_level: { level: string; count: number }[]
  by_module: { module: string; count: number }[]
  by_source: { source_id: number; source_name: string; count: number }[]
  by_hour: { hour: string; level: string; count: number }[]
  top_errors: { module: string; count: number }[]
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

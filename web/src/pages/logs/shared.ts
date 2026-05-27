import type { LogEntry, LogSource, LogAlertRule } from '@/types'

export const formatDateTime = (value?: string) => {
  if (!value) return '--'
  return new Date(value).toLocaleString('zh-CN')
}

export const formatDuration = (ms?: number) => {
  if (!ms) return '--'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

export const levelColor = (level: string) => {
  switch (level) {
    case 'error': return 'text-red-400 bg-red-400/10 border-red-400/30'
    case 'warn': return 'text-amber-400 bg-amber-400/10 border-amber-400/30'
    case 'info': return 'text-sky-400 bg-sky-400/10 border-sky-400/30'
    case 'debug': return 'text-dark-400 bg-dark-400/10 border-dark-400/30'
    default: return 'text-dark-300 bg-dark-300/10 border-dark-300/30'
  }
}

export const levelLabel = (level: string) => {
  switch (level) {
    case 'error': return '错误'
    case 'warn': return '警告'
    case 'info': return '信息'
    case 'debug': return '调试'
    default: return level
  }
}

export const statusColor = (status: string) => {
  switch (status) {
    case 'success': return 'text-emerald-400 bg-emerald-400/10 border-emerald-400/30'
    case 'failed': return 'text-red-400 bg-red-400/10 border-red-400/30'
    case 'skipped': return 'text-dark-400 bg-dark-400/10 border-dark-400/30'
    default: return 'text-dark-300 bg-dark-300/10 border-dark-300/30'
  }
}

export const statusLabel = (status: string) => {
  switch (status) {
    case 'success': return '成功'
    case 'failed': return '失败'
    case 'skipped': return '跳过'
    default: return status
  }
}

export const moduleLabel = (module: string) => {
  const map: Record<string, string> = {
    classify: '文章分类',
    llm_proxy: 'LLM Proxy',
    rss_fetch: 'RSS采集',
    file_ingest: '文件入库',
    crawler: '爬虫',
    matrix_notify: 'Matrix通知',
    'bellkeeper-core': '核心系统',
  }
  return map[module] || module
}

export const sourceTypeLabel = (t: string) => {
  const map: Record<string, string> = { internal: '内部', n8n: 'n8n', external: '外部' }
  return map[t] || t
}

export type { LogEntry, LogSource, LogAlertRule }

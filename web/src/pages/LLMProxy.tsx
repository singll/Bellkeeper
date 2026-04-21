import { A } from '@solidjs/router'
import { Component, createSignal, Show, Switch, Match, onMount } from 'solid-js'
import type { LLMChannelStatus, LLMGroupStatus, LLMChannelConfig, LLMModelGroupConfig } from '@/types'
import { llmProxyApi } from '@/api'
import LLMProxyOverview from './llm-proxy/LLMProxyOverview'
import LLMProxyChannels from './llm-proxy/LLMProxyChannels'
import LLMProxyGroups from './llm-proxy/LLMProxyGroups'
import LLMProxyConfig from './llm-proxy/LLMProxyConfig'

type TabKey = 'overview' | 'channels' | 'groups' | 'config'

const LLMProxy: Component = () => {
  const [activeTab, setActiveTab] = createSignal<TabKey>('overview')
  const [refreshing, setRefreshing] = createSignal(false)

  // Fetch data directly using fetch
  const fetchChannels = async (): Promise<{ data: LLMChannelStatus[] }> => {
    const res = await fetch('/api/llm/channels/status')
    if (!res.ok) throw new Error('Failed to fetch channels')
    return res.json()
  }

  const fetchGroups = async (): Promise<{ data: LLMGroupStatus[] }> => {
    const res = await fetch('/api/llm/groups/status')
    if (!res.ok) throw new Error('Failed to fetch groups')
    return res.json()
  }

  const fetchChannelConfigs = async (): Promise<{ data: LLMChannelConfig[] }> => {
    const res = await fetch('/api/llm/config/channels')
    if (!res.ok) throw new Error('Failed to fetch channel configs')
    return res.json()
  }

  const fetchGroupConfigs = async (): Promise<{ data: LLMModelGroupConfig[] }> => {
    const res = await fetch('/api/llm/config/groups')
    if (!res.ok) throw new Error('Failed to fetch group configs')
    return res.json()
  }

  // State
  const [channelsData, setChannelsData] = createSignal<{ data: LLMChannelStatus[] } | null>(null)
  const [groupsData, setGroupsData] = createSignal<{ data: LLMGroupStatus[] } | null>(null)
  const [channelConfigsData, setChannelConfigsData] = createSignal<{ data: LLMChannelConfig[] } | null>(null)
  const [groupConfigsData, setGroupConfigsData] = createSignal<{ data: LLMModelGroupConfig[] } | null>(null)
  const [loading, setLoading] = createSignal(true)
  const [error, setError] = createSignal<Error | null>(null)

  // Fetch all data
  const loadAll = async () => {
    setLoading(true)
    setError(null)
    try {
      const [ch, gr, cc, gc] = await Promise.all([
        fetchChannels(),
        fetchGroups(),
        fetchChannelConfigs(),
        fetchGroupConfigs(),
      ])
      setChannelsData(ch)
      setGroupsData(gr)
      setChannelConfigsData(cc)
      setGroupConfigsData(gc)
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)))
    } finally {
      setLoading(false)
    }
  }

  // Initial load
  onMount(() => {
    loadAll()
  })

  const handleRefreshAll = async () => {
    setRefreshing(true)
    try {
      await loadAll()
    } finally {
      setRefreshing(false)
    }
  }

  const handleRefreshChannels = async () => {
    const data = await fetchChannels()
    setChannelsData(data)
  }

  const handleRefreshGroups = async () => {
    const data = await fetchGroups()
    setGroupsData(data)
  }

  const handleRefreshConfigs = async () => {
    const [cc, gc] = await Promise.all([fetchChannelConfigs(), fetchGroupConfigs()])
    setChannelConfigsData(cc)
    setGroupConfigsData(gc)
  }

  const channels = () => channelsData()?.data || []
  const groups = () => groupsData()?.data || []
  const channelConfigs = () => channelConfigsData()?.data || undefined
  const groupConfigs = () => groupConfigsData()?.data || undefined

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col xl:flex-row xl:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">LLM Proxy</h1>
          <p class="text-sm text-dark-400 mt-1">查看渠道健康、模型组路由与熔断/粘性运行态</p>
        </div>
        <div class="flex items-center gap-3">
          <A href="/settings" class="btn btn-secondary">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
            </svg>
            查看设置
          </A>
          <button class="btn btn-primary" onClick={handleRefreshAll} disabled={refreshing()}>
            <svg class={`w-4 h-4 ${refreshing() ? 'animate-spin' : ''}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
            {refreshing() ? '刷新中...' : '刷新状态'}
          </button>
        </div>
      </div>

      {/* Usage Guide */}
      <div class="card mb-6 bg-primary-500/10 border-primary-500/30">
        <div class="flex items-start gap-3">
          <svg class="w-5 h-5 text-primary-400 mt-0.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <div class="text-sm">
            <p class="text-primary-300 font-medium">使用指引</p>
            <ul class="text-dark-400 mt-1 space-y-0.5">
              <li>本页展示 LLM 渠道运行状态和模型组路由信息。</li>
              <li>在「配置」标签页中可添加/编辑/删除渠道和模型组，保存后自动生效。</li>
              <li>渠道 API Key 通过环境变量名引用（如 LLM_SILICONFLOW_API_KEY），需先在 .env 中配置。</li>
            </ul>
          </div>
        </div>
      </div>

      {/* Tab Navigation */}
      <div class="tabs mb-6 w-fit">
        <button class={`tab ${activeTab() === 'overview' ? 'tab-active' : ''}`} onClick={() => setActiveTab('overview')}>
          总览
        </button>
        <button class={`tab ${activeTab() === 'channels' ? 'tab-active' : ''}`} onClick={() => setActiveTab('channels')}>
          渠道
        </button>
        <button class={`tab ${activeTab() === 'groups' ? 'tab-active' : ''}`} onClick={() => setActiveTab('groups')}>
          模型组
        </button>
        <button class={`tab ${activeTab() === 'config' ? 'tab-active' : ''}`} onClick={() => setActiveTab('config')}>
          配置
        </button>
      </div>

      {/* Loading State */}
      <Show when={!loading()}>
        <Show
          when={!error()}
          fallback={
            <div class="card py-12">
              <div class="empty-state py-4">
                <svg class="empty-state-icon w-12 h-12 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                <p class="empty-state-title">LLM Proxy 数据加载失败</p>
                <p class="empty-state-description">{error()?.message || '请检查后端服务状态'}</p>
                <button class="btn btn-secondary btn-sm mt-3" onClick={handleRefreshAll}>重试</button>
              </div>
            </div>
          }
        >
          {/* Tab Content */}
          <Switch>
            <Match when={activeTab() === 'overview'}>
              <LLMProxyOverview
                channels={channels()}
                groups={groups()}
                onRefresh={handleRefreshAll}
                refreshing={refreshing()}
              />
            </Match>
            <Match when={activeTab() === 'channels'}>
              <LLMProxyChannels
                channels={channels()}
                onRefreshChannels={handleRefreshChannels}
              />
            </Match>
            <Match when={activeTab() === 'groups'}>
              <LLMProxyGroups
                groups={groups()}
                onRefreshGroups={handleRefreshGroups}
              />
            </Match>
            <Match when={activeTab() === 'config'}>
              <LLMProxyConfig
                channelConfigs={channelConfigs()}
                groupConfigs={groupConfigs()}
                onRefreshConfigs={handleRefreshConfigs}
                onRefreshAll={handleRefreshAll}
              />
            </Match>
          </Switch>
        </Show>
      </Show>

      {/* Loading Indicator */}
      <Show when={loading()}>
        <div class="card py-12">
          <div class="flex items-center justify-center">
            <div class="loading-spinner" />
            <span class="ml-3 text-dark-400">加载 LLM Proxy 状态中...</span>
          </div>
        </div>
      </Show>
    </div>
  )
}

export default LLMProxy

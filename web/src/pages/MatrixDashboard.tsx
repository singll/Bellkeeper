import { Component, createResource, For, Show } from 'solid-js'
import { matrixApi } from '@/api'
import type { MatrixStats, MatrixEvent } from '@/types'
import { formatDateShort } from '@/utils/format'

const MatrixDashboard: Component = () => {
  const [stats] = createResource(() => matrixApi.getStats())
  const [events] = createResource(() => matrixApi.listEvents({ page: 1, page_size: 10 }))

  const statCards = () => [
    { label: '房间总数', value: stats()?.data.rooms ?? 0, icon: 'home' },
    { label: '活跃频道', value: stats()?.data.channels ?? 0, icon: 'broadcast' },
    { label: '注册命令', value: stats()?.data.commands ?? 0, icon: 'command' },
    { label: '24h 通知', value: stats()?.data.notifications_24h ?? 0, icon: 'bell' },
  ]

  const quickStats = () => [
    { label: '24h 事件数', value: stats()?.data.events_24h ?? 0 },
    { label: '活跃房间', value: stats()?.data.active_rooms ?? 0 },
  ]

  const iconSvg = (type: string) => {
    switch (type) {
      case 'home':
        return (
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6" />
          </svg>
        )
      case 'broadcast':
        return (
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8.111 16.404a5.5 5.5 0 017.778 0M12 20h.01m-7.08-7.071c3.904-3.905 10.236-3.905 14.141 0M1.394 9.393c5.857-5.857 15.355-5.857 21.213 0" />
          </svg>
        )
      case 'command':
        return (
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
          </svg>
        )
      case 'bell':
        return (
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
          </svg>
        )
      default:
        return null
    }
  }

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
        <div>
          <h1 class="text-2xl font-bold text-white">Matrix 平台总览</h1>
          <p class="text-sm text-dark-400 mt-1">Matrix 平台运行状态与统计</p>
        </div>
      </div>

      {/* Stats Cards */}
      <div class="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
        <Show
          when={!stats.loading}
          fallback={
            <For each={statCards()}>
              {() => (
                <div class="card p-6">
                  <div class="loading-skeleton h-4 w-20 mb-2" />
                  <div class="loading-skeleton h-8 w-12 mt-3" />
                </div>
              )}
            </For>
          }
        >
          <For each={statCards()}>
            {(stat) => (
              <div class="card p-6">
                <div class="flex items-center justify-between mb-2">
                  <span class="text-sm text-dark-400">{stat.label}</span>
                  <span class="text-dark-500">{iconSvg(stat.icon)}</span>
                </div>
                <div class="text-3xl font-bold text-white mt-2">{stat.value}</div>
              </div>
            )}
          </For>
        </Show>
      </div>

      {/* Recent Events */}
      <div class="card overflow-hidden p-0 mb-6">
        <div class="px-6 py-4 border-b border-dark-700">
          <h2 class="text-lg font-semibold text-white">最近事件</h2>
        </div>
        <div class="divide-y divide-dark-700">
          <Show
            when={!events.loading && (events()?.data?.length ?? 0) > 0}
            fallback={
              <div class="px-6 py-8 text-center">
                <p class="text-dark-500">暂无事件记录</p>
              </div>
            }
          >
            <For each={events()?.data ?? []}>
              {(event) => (
                <div class="px-6 py-3 flex items-center justify-between hover:bg-dark-800/50 transition-colors">
                  <div class="flex items-center gap-4">
                    <span class="text-sm text-dark-400">{formatDateShort(event.created_at)}</span>
                    <span class={`badge ${event.event_type === 'command' ? 'badge-blue' : 'badge-green'}`}>
                      {event.event_type === 'command' ? '命令' : event.event_type}
                    </span>
                    <span class="text-sm text-dark-300">{event.sender}</span>
                  </div>
                  <span class="text-xs text-dark-500 font-mono truncate max-w-[120px]">{event.room_id}</span>
                </div>
              )}
            </For>
          </Show>
        </div>
      </div>

      {/* Quick Stats */}
      <div class="grid grid-cols-2 gap-4">
        <Show
          when={!stats.loading}
          fallback={
            <For each={quickStats()}>
              {() => (
                <div class="card p-6">
                  <div class="loading-skeleton h-4 w-20 mb-2" />
                  <div class="loading-skeleton h-6 w-10 mt-3" />
                </div>
              )}
            </For>
          }
        >
          <For each={quickStats()}>
            {(stat) => (
              <div class="card p-6">
                <div class="text-sm text-dark-400 mb-1">{stat.label}</div>
                <div class="text-2xl font-bold text-white">{stat.value}</div>
              </div>
            )}
          </For>
        </Show>
      </div>
    </div>
  )
}

export default MatrixDashboard

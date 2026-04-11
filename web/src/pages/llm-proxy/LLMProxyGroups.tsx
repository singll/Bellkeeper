import { Component, For, Show, createSignal } from 'solid-js'
import type { LLMGroupStatus } from '@/types'
import { useToast } from '@/components/Toast'
import { llmProxyApi } from '@/api'
import {
  formatPercent,
  getCircuitBadge,
  getCircuitLabel,
  getCircuitDot,
  getSuccessRateColor,
} from './shared'

interface LLMProxyGroupsProps {
  groups: LLMGroupStatus[]
  onRefreshGroups: () => Promise<void>
}

const LLMProxyGroups: Component<LLMProxyGroupsProps> = (props) => {
  const toast = useToast()
  const [busyGroup, setBusyGroup] = createSignal<string | null>(null)

  const handleClearSticky = async (name: string) => {
    setBusyGroup(name)
    try {
      const result = await llmProxyApi.clearGroupSticky(name)
      toast.success(`已清理 ${result.data.cleared} 条粘性绑定`)
      await props.onRefreshGroups()
    } catch (err) {
      toast.error('清理失败: ' + (err as Error).message)
    } finally {
      setBusyGroup(null)
    }
  }

  return (
    <div class="space-y-4">
      <Show
        when={props.groups.length > 0}
        fallback={
          <div class="card empty-state">
            <p class="empty-state-title">暂无模型组</p>
            <p class="empty-state-description">当前配置尚未定义任何虚拟模型组。</p>
          </div>
        }
      >
        <For each={props.groups}>
          {(group: LLMGroupStatus) => (
            <div class="card card-hover">
              <div class="flex flex-col xl:flex-row xl:items-start justify-between gap-4 mb-4">
                <div>
                  <div class="flex items-center gap-2 flex-wrap mb-2">
                    <h2 class="text-lg font-semibold text-white">{group.name}</h2>
                    <span class="badge badge-primary">{group.strategy}</span>
                    <span class="badge badge-gray">Sticky TTL {group.sticky_ttl_seconds}s</span>
                    <span class="badge badge-gray">绑定 {group.sticky_bindings}</span>
                  </div>
                  <p class="text-sm text-dark-400">{group.description || '未填写描述'}</p>
                </div>
                <button
                  class="btn btn-secondary btn-sm"
                  disabled={busyGroup() === group.name}
                  onClick={() => handleClearSticky(group.name)}
                >
                  {busyGroup() === group.name ? '清理中...' : '清理粘性绑定'}
                </button>
              </div>

              <div class="overflow-x-auto rounded-xl border border-dark-600/50">
                <table class="table">
                  <thead>
                    <tr>
                      <th>渠道</th>
                      <th>模型</th>
                      <th>权重</th>
                      <th>可用</th>
                      <th>状态</th>
                      <th>成功率</th>
                    </tr>
                  </thead>
                  <tbody>
                    <For each={group.members}>
                      {(member) => (
                        <tr>
                          <td>
                            <div class="flex items-center gap-2">
                              <span class={`status-dot ${member.available ? getCircuitDot(member.health.state) : 'status-dot-gray'}`} />
                              <span>{member.channel}</span>
                            </div>
                          </td>
                          <td class="font-mono text-xs text-dark-300">{member.model}</td>
                          <td>{member.weight}</td>
                          <td>
                            <span class={`badge ${member.available ? 'badge-success' : 'badge-danger'}`}>
                              {member.available ? '可用' : '不可用'}
                            </span>
                          </td>
                          <td>
                            <span class={`badge ${getCircuitBadge(member.health.state)}`}>
                              {getCircuitLabel(member.health.state)}
                            </span>
                          </td>
                          <td>
                            <span class={getSuccessRateColor(member.health.recent_success_rate)}>
                              {formatPercent(member.health.recent_success_rate)}
                            </span>
                          </td>
                        </tr>
                      )}
                    </For>
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </For>
      </Show>
    </div>
  )
}

export default LLMProxyGroups

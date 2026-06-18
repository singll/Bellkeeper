import { Component, createResource, createSignal, For, Show } from 'solid-js'
import { pkbSteerApi } from '@/api/pkb'
import { useToast } from '@/components/Toast'
import type { PKBProposal, PKBDomain } from '@/types'

// 提议动作对应的徽章配色
const actionBadge = (action: string): string => {
  switch (action) {
    case 'delete':
      return 'badge-danger'
    case 'merge':
      return 'badge-warning'
    case 'restructure':
      return 'badge-primary'
    default: // add / none
      return 'badge-gray'
  }
}

// 知识骨架 · 调方向（Phase I 窄掌舵面）：批准/驳回 LLM 骨架提议 + 设领域大方向(scope)。
// 浏览与文件编辑仍归 Obsidian；骨架由 digest 机器独占维护，此处只调方向（ADR-0004 W1）。
const KnowledgeSkeleton: Component = () => {
  const toast = useToast()

  const [proposals, { refetch: refetchProposals }] = createResource(async () => {
    try {
      const res = await pkbSteerApi.listProposals()
      return res.data ?? []
    } catch (err) {
      toast.error('加载待批提议失败: ' + (err as Error).message)
      return []
    }
  })

  const [domains, { refetch: refetchDomains }] = createResource(async () => {
    try {
      const res = await pkbSteerApi.listDomains()
      return res.data ?? []
    } catch (err) {
      toast.error('加载领域失败: ' + (err as Error).message)
      return []
    }
  })

  // 当前操作中的提议 id 或领域 name（禁用其按钮，防重复提交）
  const [busy, setBusy] = createSignal<string | null>(null)

  const approve = async (p: PKBProposal) => {
    if (!confirm(`批准提议 ${p.id}？将快照旧骨架并替换「${p.domain}」知识树（可回滚）。`)) return
    setBusy(p.id)
    try {
      const res = await pkbSteerApi.approveProposal(p.id)
      toast.success(res.data.message)
      refetchProposals()
    } catch (err) {
      toast.error('批准失败: ' + (err as Error).message)
    } finally {
      setBusy(null)
    }
  }

  const reject = async (p: PKBProposal) => {
    if (!confirm(`驳回提议 ${p.id}？提议文件将被删除，骨架不动。`)) return
    setBusy(p.id)
    try {
      const res = await pkbSteerApi.rejectProposal(p.id)
      toast.success(res.data.message)
      refetchProposals()
    } catch (err) {
      toast.error('驳回失败: ' + (err as Error).message)
    } finally {
      setBusy(null)
    }
  }

  // scope 编辑：本地草稿 name -> 编辑中的值；未编辑时回落到服务端值
  const [drafts, setDrafts] = createSignal<Record<string, string>>({})
  const draftFor = (d: PKBDomain): string => {
    const m = drafts()
    return d.name in m ? m[d.name] : d.scope
  }
  const setDraft = (name: string, v: string) => setDrafts({ ...drafts(), [name]: v })

  const saveScope = async (d: PKBDomain) => {
    const next = draftFor(d).trim()
    if (!next) {
      toast.warning('大方向不能为空')
      return
    }
    if (next === d.scope) {
      toast.info('未改动')
      return
    }
    setBusy(d.name)
    try {
      const res = await pkbSteerApi.setScope(d.name, next)
      toast.success(res.data.message)
      const m = { ...drafts() }
      delete m[d.name]
      setDrafts(m)
      refetchDomains()
    } catch (err) {
      toast.error('保存失败: ' + (err as Error).message)
    } finally {
      setBusy(null)
    }
  }

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="mb-6">
        <h1 class="text-2xl font-bold text-white">知识骨架 · 调方向</h1>
        <p class="text-sm text-dark-400 mt-1">
          批准/驳回 LLM 骨架提议、设领域大方向。浏览与文件编辑仍在
          Obsidian；骨架由机器独占维护，此处只调方向。
        </p>
      </div>

      {/* 待批提议 */}
      <section class="mb-8">
        <h2 class="text-lg font-semibold text-white mb-3">待批骨架提议</h2>
        <Show
          when={(proposals()?.length ?? 0) > 0}
          fallback={
            <div class="card p-8 text-center">
              <p class="text-dark-400">{proposals.loading ? '加载中…' : '无待批提议'}</p>
            </div>
          }
        >
          <div class="space-y-4">
            <For each={proposals()}>
              {(p) => (
                <div class="card p-4">
                  <div class="flex items-start justify-between gap-4">
                    <div class="flex-1 min-w-0">
                      <div class="flex items-center gap-2 mb-1 flex-wrap">
                        <span class={`badge ${actionBadge(p.action)}`}>{p.action}</span>
                        <span class="text-white font-medium">{p.domain}</span>
                        <span class="badge badge-gray">影响半径 {p.impact_radius}</span>
                        <span class="text-xs text-dark-500 font-mono">{p.id}</span>
                      </div>
                      <p class="text-sm text-dark-300 whitespace-pre-wrap">{p.summary}</p>
                      <Show when={p.proposed_tree}>
                        <details class="mt-2">
                          <summary class="text-sm text-primary-400 cursor-pointer">
                            查看提议的知识树
                          </summary>
                          <pre class="mt-2 p-3 bg-dark-800 rounded text-xs text-dark-200 overflow-x-auto whitespace-pre-wrap">
                            {p.proposed_tree}
                          </pre>
                        </details>
                      </Show>
                    </div>
                    <div class="flex flex-col gap-2 shrink-0">
                      <button
                        class="btn btn-primary"
                        disabled={busy() === p.id}
                        onClick={() => approve(p)}
                      >
                        批准
                      </button>
                      <button
                        class="btn btn-danger"
                        disabled={busy() === p.id}
                        onClick={() => reject(p)}
                      >
                        驳回
                      </button>
                    </div>
                  </div>
                </div>
              )}
            </For>
          </div>
        </Show>
      </section>

      {/* 领域大方向 */}
      <section>
        <h2 class="text-lg font-semibold text-white mb-3">领域大方向（scope）</h2>
        <p class="text-sm text-dark-400 mb-3">
          改一句话大方向 → 保存写回 domains.yaml；下次 pkb-curate skeleton
          运行即生效。资讯流/兜底领域不生成骨架，不可设。
        </p>
        <div class="space-y-4">
          <For each={domains()}>
            {(d) => (
              <div class="card p-4">
                <div class="flex items-center gap-2 mb-2 flex-wrap">
                  <span class="text-white font-medium">{d.display}</span>
                  <span class="text-xs text-dark-500 font-mono">{d.name}</span>
                  <span class="text-xs text-dark-500 font-mono">{d.vault_subpath}</span>
                  <Show when={d.feed}>
                    <span class="badge badge-warning">资讯流</span>
                  </Show>
                  <Show when={d.is_default}>
                    <span class="badge badge-gray">兜底</span>
                  </Show>
                </div>
                <Show
                  when={d.can_set_scope}
                  fallback={
                    <p class="text-sm text-dark-500 italic">
                      资讯流/兜底领域不生成骨架，不设 scope。
                    </p>
                  }
                >
                  <textarea
                    class="input w-full"
                    rows={2}
                    placeholder="一句话大方向，决定骨架生成的目标结构…"
                    value={draftFor(d)}
                    onInput={(e) => setDraft(d.name, e.currentTarget.value)}
                  />
                  <div class="flex justify-end mt-2">
                    <button
                      class="btn btn-primary"
                      disabled={busy() === d.name || draftFor(d).trim() === d.scope}
                      onClick={() => saveScope(d)}
                    >
                      保存
                    </button>
                  </div>
                </Show>
              </div>
            )}
          </For>
        </div>
      </section>
    </div>
  )
}

export default KnowledgeSkeleton

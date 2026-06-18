import { Component, createResource, createSignal, For, Show } from 'solid-js'
import { pkbSteerApi } from '@/api/pkb'
import { useToast } from '@/components/Toast'
import type { PKBProposal, PKBDomain, PKBDomainStat } from '@/types'

// 提议动作对应的徽章配色
const actionBadge = (action: string): string => {
  switch (action) {
    case 'delete':
      return 'badge-danger'
    case 'merge':
      return 'badge-warning'
    case 'restructure':
      return 'badge-primary'
    default:
      return 'badge-gray'
  }
}

const fmtDigest = (s: string): string => {
  if (!s) return '—'
  const d = new Date(s)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleDateString()
}

// 知识骨架 · 调方向（窄掌舵面）：管理知识领域(增删改名)、设大方向、批准 LLM 骨架提议、看状态。
// 浏览与文件编辑仍归 Obsidian；骨架由 digest 机器独占维护（ADR-0004 W1），此处只调方向。
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

  const [stats, { refetch: refetchStats }] = createResource(async () => {
    try {
      const res = await pkbSteerApi.listStats()
      return res.data ?? []
    } catch {
      return [] as PKBDomainStat[]
    }
  })

  const statOf = (name: string): PKBDomainStat | undefined =>
    (stats() ?? []).find((s) => s.name === name)

  const refetchDomainData = () => {
    refetchDomains()
    refetchStats()
  }

  const [busy, setBusy] = createSignal<string | null>(null)

  // ---- 提议审批 ----
  const approve = async (p: PKBProposal) => {
    if (!confirm(`批准提议 ${p.id}？将快照旧骨架并替换「${p.domain}」知识树（可回滚）。`)) return
    setBusy(p.id)
    try {
      const res = await pkbSteerApi.approveProposal(p.id)
      toast.success(res.data.message)
      refetchProposals()
      refetchStats()
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

  // ---- 新建领域 ----
  const [newDisplay, setNewDisplay] = createSignal('')
  const [newScope, setNewScope] = createSignal('')
  const [creating, setCreating] = createSignal(false)
  const createDomain = async () => {
    const display = newDisplay().trim()
    const scope = newScope().trim()
    if (!display || !scope) {
      toast.warning('请填写领域名与大方向')
      return
    }
    setCreating(true)
    try {
      const res = await pkbSteerApi.createDomain(display, scope)
      toast.success(res.data.message)
      setNewDisplay('')
      setNewScope('')
      refetchDomainData()
    } catch (err) {
      toast.error('创建失败: ' + (err as Error).message)
    } finally {
      setCreating(false)
    }
  }

  // ---- scope 编辑 ----
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

  // ---- 改名（仅 display）----
  const [editingName, setEditingName] = createSignal<string | null>(null)
  const [nameDraft, setNameDraft] = createSignal('')
  const startRename = (d: PKBDomain) => {
    setEditingName(d.name)
    setNameDraft(d.display)
  }
  const saveDisplay = async (d: PKBDomain) => {
    const next = nameDraft().trim()
    if (!next || next === d.display) {
      setEditingName(null)
      return
    }
    setBusy(d.name)
    try {
      const res = await pkbSteerApi.setDisplay(d.name, next)
      toast.success(res.data.message)
      setEditingName(null)
      refetchDomainData()
    } catch (err) {
      toast.error('改名失败: ' + (err as Error).message)
    } finally {
      setBusy(null)
    }
  }

  // ---- 删除 ----
  const removeDomain = async (d: PKBDomain) => {
    if (!confirm(`删除领域「${d.display}」？仅删配置，vault 文件保留（可在 Obsidian 浏览）。`)) return
    setBusy(d.name)
    try {
      const res = await pkbSteerApi.deleteDomain(d.name)
      toast.success(res.data.message)
      refetchDomainData()
    } catch (err) {
      toast.error('删除失败: ' + (err as Error).message)
    } finally {
      setBusy(null)
    }
  }

  // ---- 生成骨架（后台异步）----
  const genSkeleton = async (d: PKBDomain) => {
    setBusy(d.name)
    try {
      const res = await pkbSteerApi.generateSkeleton(d.name)
      toast.success(res.data.message)
      setTimeout(() => refetchStats(), 5000)
    } catch (err) {
      toast.error('触发失败: ' + (err as Error).message)
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
          管理知识领域、设大方向、批准 LLM 骨架提议。浏览与文件编辑仍在 Obsidian；骨架由机器独占维护，此处只调方向。
        </p>
      </div>

      {/* 新建领域 */}
      <section class="card p-4 mb-6">
        <h2 class="text-base font-semibold text-white mb-3">新建知识领域</h2>
        <div class="flex flex-col gap-2 sm:flex-row">
          <input
            class="input sm:w-48"
            placeholder="领域名（如：区块链）"
            value={newDisplay()}
            onInput={(e) => setNewDisplay(e.currentTarget.value)}
          />
          <input
            class="input flex-1"
            placeholder="一句话大方向（scope），决定骨架目标结构…"
            value={newScope()}
            onInput={(e) => setNewScope(e.currentTarget.value)}
          />
          <button class="btn btn-primary" disabled={creating()} onClick={createDomain}>
            {creating() ? '创建中…' : '新建'}
          </button>
        </div>
        <p class="text-xs text-dark-500 mt-2">
          创建后不自动生成骨架——在下方点「生成骨架」播种，或等自动维护。
        </p>
      </section>

      {/* 待批提议 */}
      <section class="mb-8">
        <h2 class="text-lg font-semibold text-white mb-3">待批骨架提议</h2>
        <Show
          when={(proposals()?.length ?? 0) > 0}
          fallback={
            <div class="card p-6 text-center">
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
                          <summary class="text-sm text-primary-400 cursor-pointer">查看提议的知识树</summary>
                          <pre class="mt-2 p-3 bg-dark-800 rounded text-xs text-dark-200 overflow-x-auto whitespace-pre-wrap">
                            {p.proposed_tree}
                          </pre>
                        </details>
                      </Show>
                    </div>
                    <div class="flex flex-col gap-2 shrink-0">
                      <button class="btn btn-primary" disabled={busy() === p.id} onClick={() => approve(p)}>
                        批准
                      </button>
                      <button class="btn btn-danger" disabled={busy() === p.id} onClick={() => reject(p)}>
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

      {/* 知识领域（管理 + 状态） */}
      <section>
        <h2 class="text-lg font-semibold text-white mb-3">知识领域</h2>
        <div class="space-y-4">
          <For each={domains()}>
            {(d) => (
              <div class="card p-4">
                {/* 标题行 */}
                <div class="flex items-center gap-2 mb-2 flex-wrap">
                  <Show
                    when={editingName() === d.name}
                    fallback={<span class="text-white font-medium">{d.display}</span>}
                  >
                    <input
                      class="input w-40 py-1"
                      value={nameDraft()}
                      onInput={(e) => setNameDraft(e.currentTarget.value)}
                    />
                    <button class="btn btn-sm btn-primary" onClick={() => saveDisplay(d)}>
                      保存
                    </button>
                    <button class="btn btn-sm btn-ghost" onClick={() => setEditingName(null)}>
                      取消
                    </button>
                  </Show>
                  <span class="text-xs text-dark-500 font-mono">{d.name}</span>
                  <Show when={d.feed}>
                    <span class="badge badge-warning">资讯流</span>
                  </Show>
                  <Show when={d.is_default}>
                    <span class="badge badge-gray">兜底</span>
                  </Show>
                  <Show when={d.can_set_scope && statOf(d.name) && !statOf(d.name)?.has_skeleton}>
                    <span class="badge badge-danger">无骨架</span>
                  </Show>
                </div>

                {/* 状态行 */}
                <Show when={statOf(d.name)}>
                  {(s) => (
                    <div class="flex flex-wrap gap-x-4 gap-y-1 text-xs text-dark-400 mb-3">
                      <span>卡片 <span class="text-dark-200">{s().cards}</span></span>
                      <span>今日 <span class="text-green-400">+{s().cards_today}</span></span>
                      <span>7天 <span class="text-dark-200">+{s().cards_week}</span></span>
                      <Show when={d.can_set_scope}>
                        <span>缺口 <span class="text-amber-400">{s().skeleton_gaps}</span></span>
                        <span>已挂 <span class="text-dark-200">{s().skeleton_filled}</span></span>
                        <span>待归位 <span class="text-amber-400">{s().waitlist}</span></span>
                        <span>低置信 <span class="text-dark-200">{s().low_confidence}</span></span>
                      </Show>
                      <span>digest <span class="text-dark-200">{fmtDigest(s().last_digest_at)}</span></span>
                    </div>
                  )}
                </Show>

                {/* scope + 操作 */}
                <Show
                  when={d.can_set_scope}
                  fallback={<p class="text-sm text-dark-500 italic">资讯流/兜底领域不生成骨架，不设 scope。</p>}
                >
                  <textarea
                    class="input w-full"
                    rows={2}
                    placeholder="一句话大方向…"
                    value={draftFor(d)}
                    onInput={(e) => setDraft(d.name, e.currentTarget.value)}
                  />
                  <div class="flex flex-wrap justify-end gap-2 mt-2">
                    <button class="btn btn-sm btn-ghost" disabled={busy() === d.name} onClick={() => startRename(d)}>
                      改名
                    </button>
                    <button class="btn btn-sm btn-secondary" disabled={busy() === d.name} onClick={() => genSkeleton(d)}>
                      生成骨架
                    </button>
                    <button class="btn btn-sm btn-danger" disabled={busy() === d.name} onClick={() => removeDomain(d)}>
                      删除
                    </button>
                    <button
                      class="btn btn-sm btn-primary"
                      disabled={busy() === d.name || draftFor(d).trim() === d.scope}
                      onClick={() => saveScope(d)}
                    >
                      保存大方向
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

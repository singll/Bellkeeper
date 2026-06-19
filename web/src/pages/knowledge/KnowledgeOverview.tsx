import { Component, createResource, createSignal, For, Show, onMount } from 'solid-js'
import { A } from '@solidjs/router'
import { pkbSteerApi, pkbFeedApi } from '@/api/pkb'
import { knowledgeFilesApi } from '@/api/knowledge'
import { useToast } from '@/components/Toast'
import type { PKBDomainStat, PKBFeedDay, PKBFeedArchiveContent } from '@/types'

// 知识库总览（ADR-0006 只读观测）：把散在 vault 里、Obsidian 看不出的全局统计/状态/资讯时间线
// 聚合到一处。全为只读聚合；唯一例外=资讯库每日存档允许 Web 只读渲染（后端已清洗 HTML）。
const KnowledgeOverview: Component = () => {
  const toast = useToast()

  const [stats] = createResource(async () => {
    try {
      const res = await pkbSteerApi.listStats()
      return res.data ?? []
    } catch {
      return [] as PKBDomainStat[]
    }
  })

  const [proposals] = createResource(async () => {
    try {
      const res = await pkbSteerApi.listProposals()
      return res.data ?? []
    } catch {
      return []
    }
  })

  const [fileStats] = createResource(async () => {
    try {
      const res = await knowledgeFilesApi.getStats()
      return res.data
    } catch {
      return undefined
    }
  })

  // ---- KPI 聚合 ----
  const sum = (pick: (s: PKBDomainStat) => number) => (stats() ?? []).reduce((acc, s) => acc + pick(s), 0)
  const knowledgeDomains = () => (stats() ?? []).filter((s) => !s.feed && !s.is_default)

  // ---- 资讯时间线（手动累积 + 往前翻）----
  const [days, setDays] = createSignal<PKBFeedDay[]>([])
  const [timelineLoading, setTimelineLoading] = createSignal(false)
  const [timelineDone, setTimelineDone] = createSignal(false)
  const loadTimeline = async (before?: string) => {
    setTimelineLoading(true)
    try {
      const res = await pkbFeedApi.timeline(14, before)
      const got = res.data ?? []
      setDays([...days(), ...got])
      if (got.length === 0) setTimelineDone(true)
    } catch (err) {
      toast.error('加载资讯时间线失败: ' + (err as Error).message)
    } finally {
      setTimelineLoading(false)
    }
  }
  onMount(() => loadTimeline())
  const loadEarlier = () => {
    const list = days()
    const oldest = list.length > 0 ? list[list.length - 1].date : undefined
    loadTimeline(oldest)
  }

  // ---- 单篇资讯只读渲染 ----
  const [archive, setArchive] = createSignal<PKBFeedArchiveContent | null>(null)
  const [archiveLoading, setArchiveLoading] = createSignal(false)
  const openArchive = async (date: string, domain: string) => {
    setArchiveLoading(true)
    try {
      const res = await pkbFeedApi.archive(date, domain)
      setArchive(res.data)
    } catch (err) {
      toast.error('加载资讯存档失败: ' + (err as Error).message)
    } finally {
      setArchiveLoading(false)
    }
  }

  // ---- 需要关注 ----
  const attentionDomains = () =>
    (stats() ?? []).filter((s) => s.waitlist_high || s.low_confidence_high)

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="mb-6">
        <h1 class="text-2xl font-bold text-white">知识库 · 总览</h1>
        <p class="text-sm text-dark-400 mt-1">
          全局统计、各领域状态、资讯时间线与需要关注项。只读观测——浏览与编辑仍在 Obsidian，调方向去知识骨架页。
        </p>
      </div>

      {/* 1. KPI */}
      <section class="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-7 gap-3 mb-8">
        <For
          each={[
            { label: '卡片总数', value: sum((s) => s.cards), cls: 'text-blue-400' },
            { label: '今日新增', value: sum((s) => s.cards_today), cls: 'text-emerald-400' },
            { label: '领域数', value: knowledgeDomains().length, cls: 'text-white' },
            { label: '缺口', value: sum((s) => s.skeleton_gaps), cls: 'text-amber-400' },
            { label: '待归位', value: sum((s) => s.waitlist), cls: 'text-amber-400' },
            { label: '低置信', value: sum((s) => s.low_confidence), cls: 'text-orange-400' },
            { label: '待批提议', value: proposals()?.length ?? 0, cls: 'text-violet-400' },
          ]}
        >
          {(kpi) => (
            <div class="card p-4">
              <div class="text-xs text-dark-400 mb-1">{kpi.label}</div>
              <div class={`text-2xl font-bold ${kpi.cls}`}>{kpi.value}</div>
            </div>
          )}
        </For>
      </section>

      {/* 2. 各领域一览 */}
      <section class="mb-8">
        <h2 class="text-lg font-semibold text-white mb-3">各领域一览</h2>
        <Show
          when={knowledgeDomains().length > 0}
          fallback={<div class="card p-6 text-center text-dark-400">{stats.loading ? '加载中…' : '暂无知识领域'}</div>}
        >
          <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
            <For each={knowledgeDomains()}>
              {(d) => (
                <A href="/knowledge/skeleton" class="card card-hover p-4 block">
                  <div class="flex items-center justify-between mb-2">
                    <span class="text-white font-medium truncate">{d.display}</span>
                    <Show when={!d.has_skeleton}>
                      <span class="badge badge-gray">无骨架</span>
                    </Show>
                  </div>
                  <div class="flex items-baseline gap-3 text-sm text-dark-300">
                    <span>卡片 <span class="font-bold text-white">{d.cards}</span></span>
                    <Show when={d.cards_today > 0}>
                      <span class="text-emerald-400">今日 +{d.cards_today}</span>
                    </Show>
                  </div>
                  <div class="flex items-baseline gap-3 text-xs text-dark-400 mt-1">
                    <span>缺口 {d.skeleton_gaps}</span>
                    <span class={d.waitlist_high ? 'text-amber-400' : ''}>待归位 {d.waitlist}</span>
                    <span class={d.low_confidence_high ? 'text-orange-400' : ''}>低置信 {d.low_confidence}</span>
                  </div>
                </A>
              )}
            </For>
          </div>
        </Show>
      </section>

      {/* 3. 资讯时间线 */}
      <section class="mb-8">
        <h2 class="text-lg font-semibold text-white mb-3">资讯时间线</h2>
        <Show
          when={days().length > 0}
          fallback={<div class="card p-6 text-center text-dark-400">{timelineLoading() ? '加载中…' : '暂无资讯存档'}</div>}
        >
          <div class="space-y-3">
            <For each={days()}>
              {(day) => (
                <div class="card p-4">
                  <div class="text-sm font-medium text-dark-200 mb-2">{day.date}</div>
                  <div class="flex flex-wrap gap-2">
                    <For each={day.archives}>
                      {(a) => (
                        <button
                          type="button"
                          class="badge badge-primary cursor-pointer hover:opacity-80"
                          onClick={() => openArchive(day.date, a.domain)}
                        >
                          {a.domain}
                        </button>
                      )}
                    </For>
                  </div>
                </div>
              )}
            </For>
          </div>
          <div class="text-center mt-3">
            <Show when={!timelineDone()} fallback={<span class="text-xs text-dark-500">已到最早</span>}>
              <button type="button" class="btn btn-secondary" disabled={timelineLoading()} onClick={loadEarlier}>
                {timelineLoading() ? '加载中…' : '加载更早'}
              </button>
            </Show>
          </div>
        </Show>
      </section>

      {/* 4. 需要关注 */}
      <section class="mb-8">
        <h2 class="text-lg font-semibold text-white mb-3">需要关注</h2>
        <Show
          when={(proposals()?.length ?? 0) > 0 || attentionDomains().length > 0}
          fallback={<div class="card p-6 text-center text-dark-400">暂无需要关注项</div>}
        >
          <div class="space-y-3">
            <Show when={(proposals()?.length ?? 0) > 0}>
              <A href="/knowledge/skeleton" class="card card-hover p-4 flex items-center justify-between">
                <span class="text-white">待批骨架提议</span>
                <span class="badge badge-violet">{proposals()?.length} 项待处理 →</span>
              </A>
            </Show>
            <For each={attentionDomains()}>
              {(d) => (
                <A href="/knowledge/skeleton" class="card card-hover p-4 flex items-center justify-between">
                  <span class="text-white">{d.display}</span>
                  <span class="flex gap-2">
                    <Show when={d.waitlist_high}>
                      <span class="badge badge-warning">待归位 {d.waitlist}</span>
                    </Show>
                    <Show when={d.low_confidence_high}>
                      <span class="badge badge-warning">低置信 {d.low_confidence}</span>
                    </Show>
                  </span>
                </A>
              )}
            </For>
          </div>
        </Show>
      </section>

      {/* 5. 采集动态 */}
      <section class="mb-8">
        <h2 class="text-lg font-semibold text-white mb-3">采集动态</h2>
        <div class="card p-4">
          <div class="grid grid-cols-2 sm:grid-cols-4 gap-4">
            <For each={['raw', 'archive', 'vault']}>
              {(layer) => (
                <div>
                  <div class="text-xs text-dark-400 mb-1">{layer}</div>
                  <div class="text-2xl font-bold text-white">{fileStats()?.by_layer?.[layer] ?? '--'}</div>
                </div>
              )}
            </For>
            <div>
              <div class="text-xs text-dark-400 mb-1">今日入库</div>
              <div class={`text-2xl font-bold ${sum((s) => s.cards_today) > 0 ? 'text-emerald-400' : 'text-dark-200'}`}>
                +{sum((s) => s.cards_today)}
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* 资讯存档只读渲染面板（ADR-0006 唯一例外，HTML 已服务端清洗） */}
      <Show when={archive() || archiveLoading()}>
        <div
          class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
          onClick={() => setArchive(null)}
        >
          <div
            class="card max-w-3xl w-full max-h-[85vh] overflow-y-auto p-6"
            onClick={(e) => e.stopPropagation()}
          >
            <div class="flex items-start justify-between mb-4">
              <h3 class="text-lg font-semibold text-white">{archive()?.title ?? '加载中…'}</h3>
              <button type="button" class="text-dark-400 hover:text-white" onClick={() => setArchive(null)}>
                ✕
              </button>
            </div>
            <Show when={archive()} fallback={<div class="text-dark-400">加载中…</div>}>
              {/* 后端已 goldmark 渲染 + bluemonday 清洗；前端只读显示 */}
              <div class="prose prose-invert max-w-none text-dark-200" innerHTML={archive()!.html} />
            </Show>
          </div>
        </div>
      </Show>
    </div>
  )
}

export default KnowledgeOverview

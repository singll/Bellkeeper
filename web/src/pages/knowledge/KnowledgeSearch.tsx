import { Component, createSignal, createResource, For, Show } from 'solid-js'
import { knowledgeSearchApi, knowledgeFilesApi } from '@/api/knowledge'
import { useToast } from '@/components/Toast'
import type { KnowledgeSearchHit } from '@/types'

const KnowledgeSearch: Component = () => {
  const toast = useToast()

  // State
  const [query, setQuery] = createSignal('')
  const [layers, setLayers] = createSignal<string[]>([])
  const [selectedLayer, setSelectedLayer] = createSignal('')
  const [searching, setSearching] = createSignal(false)

  // Search results
  const [searchResults] = createResource(
    () => ({ query: query(), layer: selectedLayer() }),
    async ({ query, layer }) => {
      if (!query || query.length < 2) return null
      setSearching(true)
      try {
        const results = await knowledgeSearchApi.search({
          query,
          layers: layer ? [layer] : undefined,
          limit: 50,
        })
        return results.data
      } catch (err) {
        toast.error('搜索失败: ' + (err as Error).message)
        return null
      } finally {
        setSearching(false)
      }
    }
  )

  // Stats for layer filter
  const [stats] = createResource(() => knowledgeFilesApi.getStats())

  // Layer options
  const layerOptions = () => {
    const statsData = stats()?.data
    if (!statsData?.by_layer) return []
    return Object.entries(statsData.by_layer).map(([layer, count]) => ({
      layer,
      count,
    }))
  }

  // Handle search
  const handleSearch = (e: Event) => {
    e.preventDefault()
    if (query().length < 2) {
      toast.warn('请输入至少 2 个字符')
      return
    }
    // SolidJS resource will auto-refetch
  }

  // Highlight text with search matches
  const highlightText = (text: string, query: string) => {
    if (!query) return text
    const regex = new RegExp(`(${query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi')
    return text.replace(regex, '<mark class="bg-yellow-500/30 text-yellow-300">$1</mark>')
  }

  return (
    <div class="animate-fade-in">
      {/* Header */}
      <div class="mb-6">
        <h1 class="text-2xl font-bold text-white">知识搜索</h1>
        <p class="text-sm text-dark-400 mt-1">在知识库中搜索文档和内容</p>
      </div>

      {/* Search Form */}
      <div class="card p-6 mb-6">
        <form onSubmit={handleSearch} class="flex flex-col gap-4">
          <div class="flex gap-4">
            <div class="flex-1">
              <input
                type="text"
                class="input w-full"
                placeholder="输入关键词搜索..."
                value={query()}
                onInput={(e) => setQuery(e.currentTarget.value)}
              />
            </div>
            <select
              class="input w-40"
              value={selectedLayer()}
              onChange={(e) => setSelectedLayer(e.currentTarget.value)}
            >
              <option value="">全部层级</option>
              <For each={layerOptions()}>
                {(opt) => (
                  <option value={opt.layer}>
                    {opt.layer} ({opt.count})
                  </option>
                )}
              </For>
            </select>
            <button type="submit" class="btn btn-primary" disabled={searching()}>
              {searching() ? (
                <>
                  <div class="loading-spinner" />
                  搜索中...
                </>
              ) : (
                <>
                  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                  </svg>
                  搜索
                </>
              )}
            </button>
          </div>
        </form>
      </div>

      {/* Results */}
      <Show when={searchResults()}>
        <div class="mb-4 text-sm text-dark-400">
          找到 {searchResults()?.total || 0} 个结果 ({searchResults()?.query_ms || 0}ms)
        </div>
      </Show>

      <div class="space-y-4">
        <Show
          when={searchResults()?.files?.length}
          fallback={
            <Show when={query().length >= 2 && !searching()}>
              <div class="card p-8 text-center">
                <p class="text-dark-400">未找到相关结果</p>
              </div>
            </Show>
          }
        >
          <For each={searchResults()?.files ?? []}>
            {(hit) => (
              <div class="card p-4 hover:bg-dark-700/30 transition-colors">
                <div class="flex items-start justify-between gap-4">
                  <div class="flex-1">
                    {/* Title */}
                    <h3 class="text-lg font-medium text-white mb-1">
                      <span innerHTML={highlightText(hit.heading || hit.title, query())} />
                    </h3>

                    {/* Meta */}
                    <div class="flex items-center gap-3 text-sm text-dark-400 mb-2">
                      <span class="badge badge-gray">{hit.layer}</span>
                      <Show when={hit.category}>
                        <span>{hit.category}</span>
                      </Show>
                      <Show when={hit.source_domain}>
                        <a
                          href={`https://${hit.source_domain}`}
                          target="_blank"
                          rel="noopener"
                          class="text-primary-400 hover:underline"
                        >
                          {hit.source_domain}
                        </a>
                      </Show>
                    </div>

                    {/* Content snippet */}
                    <p
                      class="text-sm text-dark-300 line-clamp-3"
                      innerHTML={highlightText(hit.highlights?.[0] || hit.content.substring(0, 300), query())}
                    />

                    {/* Tags */}
                    <Show when={hit.tags?.length}>
                      <div class="flex gap-2 mt-2">
                        <For each={hit.tags}>
                          {(tag) => <span class="badge badge-primary text-xs">{tag}</span>}
                        </For>
                      </div>
                    </Show>
                  </div>

                  {/* File path */}
                  <div class="text-xs text-dark-500 font-mono whitespace-nowrap">
                    {hit.file_path}
                  </div>
                </div>
              </div>
            )}
          </For>
        </Show>
      </div>
    </div>
  )
}

export default KnowledgeSearch

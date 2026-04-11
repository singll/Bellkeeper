import { Component, createSignal, Show, For, createEffect, onCleanup } from 'solid-js'
import { useNavigate } from '@solidjs/router'
import { search } from '../api'

interface SearchResult {
  tags: any[]
  documents: any[]
  rss_feeds: any[]
}

interface GlobalSearchProps {
  open: boolean
  onClose: () => void
}

const GlobalSearch: Component<GlobalSearchProps> = (props) => {
  const navigate = useNavigate()
  const [query, setQuery] = createSignal('')
  const [results, setResults] = createSignal<SearchResult | null>(null)
  const [loading, setLoading] = createSignal(false)
  const [activeTab, setActiveTab] = createSignal<'all' | 'tags' | 'documents' | 'rss'>('all')
  let inputRef: HTMLInputElement | undefined
  let debounceTimer: number | undefined

  // Focus input when modal opens
  createEffect(() => {
    if (props.open && inputRef) {
      inputRef.focus()
    }
  })

  // Debounced search
  createEffect(() => {
    const q = query()
    if (debounceTimer) clearTimeout(debounceTimer)

    if (q.length < 2) {
      setResults(null)
      return
    }

    debounceTimer = window.setTimeout(async () => {
      setLoading(true)
      try {
        const data = await search(q, 'all', 5)
        setResults(data)
      } catch (err) {
        console.error('Search error:', err)
      } finally {
        setLoading(false)
      }
    }, 300)
  })

  onCleanup(() => {
    if (debounceTimer) clearTimeout(debounceTimer)
  })

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Escape') {
      props.onClose()
    }
  }

  const handleNavigate = (path: string) => {
    navigate(path)
    props.onClose()
    setQuery('')
    setResults(null)
  }

  const getFilteredResults = () => {
    const r = results()
    if (!r) return null

    switch (activeTab()) {
      case 'tags':
        return { ...r, documents: [], rss_feeds: [] }
      case 'documents':
        return { ...r, tags: [], rss_feeds: [] }
      case 'rss':
        return { ...r, tags: [], documents: [] }
      default:
        return r
    }
  }

  const totalCount = () => {
    const r = results()
    if (!r) return 0
    return r.tags.length + r.documents.length + r.rss_feeds.length
  }

  return (
    <Show when={props.open}>
      <div
        class="fixed inset-0 bg-dark-900/80 backdrop-blur-sm z-50 flex items-start justify-center pt-20 px-4"
        onClick={() => props.onClose()}
        onKeyDown={handleKeyDown}
      >
        <div
          class="w-full max-w-2xl bg-dark-800 rounded-xl shadow-2xl border border-dark-600 overflow-hidden"
          onClick={(e) => e.stopPropagation()}
        >
          {/* Search input */}
          <div class="p-4 border-b border-dark-600">
            <div class="flex items-center gap-3 px-4 py-3 bg-dark-700/50 rounded-xl">
              <svg class="w-5 h-5 text-dark-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
              </svg>
              <input
                ref={inputRef}
                type="text"
                value={query()}
                onInput={(e) => setQuery(e.currentTarget.value)}
                placeholder="Search documents, tags, RSS feeds..."
                class="flex-1 bg-transparent border-none outline-none text-dark-100 placeholder-dark-400 text-lg"
              />
              <Show when={loading()}>
                <div class="w-5 h-5 border-2 border-primary-500 border-t-transparent rounded-full animate-spin" />
              </Show>
              <kbd class="px-2 py-1 text-xs bg-dark-600 rounded text-dark-300">ESC</kbd>
            </div>
          </div>

          {/* Results */}
          <Show when={results() && totalCount() > 0}>
            <div class="p-2 border-b border-dark-600">
              <div class="flex gap-2">
                <For each={[
                  { id: 'all', label: 'All' },
                  { id: 'tags', label: 'Tags' },
                  { id: 'documents', label: 'Documents' },
                  { id: 'rss', label: 'RSS' },
                ] as const}>
                  {(tab) => (
                    <button
                      class={`px-3 py-1.5 text-sm rounded-lg transition-colors ${
                        activeTab() === tab.id
                          ? 'bg-primary-500/20 text-primary-300'
                          : 'text-dark-400 hover:bg-dark-700 hover:text-dark-200'
                      }`}
                      onClick={() => setActiveTab(tab.id)}
                    >
                      {tab.label}
                    </button>
                  )}
                </For>
              </div>
            </div>

            <div class="max-h-96 overflow-y-auto p-2">
              <Show when={getFilteredResults()?.tags.length}>
                <div class="mb-4">
                  <div class="px-3 py-2 text-xs font-medium text-dark-500 uppercase tracking-wider">
                    Tags
                  </div>
                  <For each={getFilteredResults()?.tags}>
                    {(tag) => (
                      <button
                        class="w-full flex items-center gap-3 px-3 py-2 rounded-lg text-left hover:bg-dark-700/50 transition-colors"
                        onClick={() => handleNavigate(`/tags?highlight=${tag.id}`)}
                      >
                        <div
                          class="w-3 h-3 rounded-full"
                          style={{ "background-color": tag.color || '#409EFF' }}
                        />
                        <div class="flex-1 min-w-0">
                          <div class="text-sm font-medium text-dark-200 truncate">{tag.name}</div>
                          <div class="text-xs text-dark-500 truncate">{tag.description}</div>
                        </div>
                      </button>
                    )}
                  </For>
                </div>
              </Show>

              <Show when={getFilteredResults()?.documents.length}>
                <div class="mb-4">
                  <div class="px-3 py-2 text-xs font-medium text-dark-500 uppercase tracking-wider">
                    Documents
                  </div>
                  <For each={getFilteredResults()?.documents}>
                    {(doc) => (
                      <button
                        class="w-full flex items-center gap-3 px-3 py-2 rounded-lg text-left hover:bg-dark-700/50 transition-colors"
                        onClick={() => handleNavigate(`/documents?highlight=${doc.id}`)}
                      >
                        <svg class="w-4 h-4 text-dark-400 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                        </svg>
                        <div class="flex-1 min-w-0">
                          <div class="text-sm font-medium text-dark-200 truncate">{doc.article_title}</div>
                          <div class="text-xs text-dark-500 truncate">{doc.article_url}</div>
                        </div>
                      </button>
                    )}
                  </For>
                </div>
              </Show>

              <Show when={getFilteredResults()?.rss_feeds.length}>
                <div>
                  <div class="px-3 py-2 text-xs font-medium text-dark-500 uppercase tracking-wider">
                    RSS Feeds
                  </div>
                  <For each={getFilteredResults()?.rss_feeds}>
                    {(feed) => (
                      <button
                        class="w-full flex items-center gap-3 px-3 py-2 rounded-lg text-left hover:bg-dark-700/50 transition-colors"
                        onClick={() => handleNavigate('/rss')}
                      >
                        <svg class="w-4 h-4 text-dark-400 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M6 5c7.18 0 13 5.82 13 13M6 11a7 7 0 017 7m-6 0a1 1 0 11-2 0 1 1 0 012 0z" />
                        </svg>
                        <div class="flex-1 min-w-0">
                          <div class="text-sm font-medium text-dark-200 truncate">{feed.name}</div>
                          <div class="text-xs text-dark-500 truncate">{feed.description}</div>
                        </div>
                      </button>
                    )}
                  </For>
                </div>
              </Show>
            </div>
          </Show>

          {/* Empty state */}
          <Show when={query().length >= 2 && !loading() && totalCount() === 0}>
            <div class="p-8 text-center text-dark-400">
              <svg class="w-12 h-12 mx-auto mb-3 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.172 16.172a4 4 0 015.656 0M9 10h.01M15 10h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
              <p class="text-sm">No results found for "{query()}"</p>
            </div>
          </Show>

          {/* Hint */}
          <Show when={query().length < 2}>
            <div class="p-8 text-center text-dark-400">
              <svg class="w-12 h-12 mx-auto mb-3 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
              </svg>
              <p class="text-sm">Type to search across documents, tags, and RSS feeds</p>
              <p class="text-xs text-dark-500 mt-2">Press <kbd class="px-1 py-0.5 bg-dark-700 rounded">Tab</kbd> to switch tabs</p>
            </div>
          </Show>
        </div>
      </div>
    </Show>
  )
}

export default GlobalSearch

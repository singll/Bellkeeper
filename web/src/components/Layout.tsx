import { Component, JSX, createSignal, createEffect, Show, For } from 'solid-js'
import { A, useLocation, RouteSectionProps } from '@solidjs/router'
import GlobalSearch from './GlobalSearch'

interface NavItem {
  path: string
  label: string
  icon: JSX.Element
  badge?: string
}

interface NavGroup {
  id: string
  label: string
  icon: JSX.Element
  items: NavItem[]
  defaultExpanded?: boolean
}

// Icon components
const DashboardIcon = () => (
  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 5a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM14 5a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1V5zM4 15a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1H5a1 1 0 01-1-1v-4zM14 15a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1v-4z" />
  </svg>
)

const TagIcon = () => (
  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z" />
  </svg>
)

const RSSIcon = () => (
  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M6 5c7.18 0 13 5.82 13 13M6 11a7 7 0 017 7m-6 0a1 1 0 11-2 0 1 1 0 012 0z" />
  </svg>
)

const DatasetIcon = () => (
  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
  </svg>
)

const DocumentIcon = () => (
  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
  </svg>
)

const WorkflowIcon = () => (
  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
  </svg>
)

const LLMProxyIcon = () => (
  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M13 10V3L4 14h7v7l9-11h-7z" />
  </svg>
)

const MatrixIcon = () => (
  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
  </svg>
)

const LogsIcon = () => (
  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-3 7h3m-3 4h3m-6-4h.01M9 16h.01" />
  </svg>
)

const SettingsIcon = () => (
  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
  </svg>
)

const ChevronIcon = () => (
  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7" />
  </svg>
)

const SearchIcon = () => (
  <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
  </svg>
)

// Navigation groups
const navGroups: NavGroup[] = [
  {
    id: 'dashboard',
    label: '仪表盘',
    icon: <DashboardIcon />,
    defaultExpanded: true,
    items: [
      {
        path: '/',
        label: '仪表盘',
        icon: <DashboardIcon />,
      },
    ],
  },
  {
    id: 'knowledge',
    label: '知识管理',
    icon: <DocumentIcon />,
    defaultExpanded: true,
    items: [
      { path: '/documents', label: 'Documents', icon: <DocumentIcon /> },
      { path: '/datasets', label: 'Datasets', icon: <DatasetIcon /> },
      { path: '/tags', label: 'Tags', icon: <TagIcon /> },
      { path: '/rss', label: 'RSS Feeds', icon: <RSSIcon /> },
    ],
  },
  {
    id: 'ai',
    label: 'AI 服务',
    icon: <LLMProxyIcon />,
    defaultExpanded: true,
    items: [
      { path: '/llm-proxy', label: 'LLM Proxy', icon: <LLMProxyIcon /> },
      { path: '/workflows', label: 'Workflows', icon: <WorkflowIcon /> },
    ],
  },
  {
    id: 'matrix',
    label: 'Matrix',
    icon: <MatrixIcon />,
    defaultExpanded: true,
    items: [
      { path: '/matrix', label: '总览', icon: <MatrixIcon /> },
      { path: '/matrix/rooms', label: '房间管理', icon: <></> },
      { path: '/matrix/channels', label: '频道管理', icon: <></> },
      { path: '/matrix/commands', label: '命令管理', icon: <></> },
      { path: '/matrix/notifications', label: '通知管理', icon: <></> },
      { path: '/matrix/events', label: '事件日志', icon: <></> },
      { path: '/matrix/command-logs', label: '命令日志', icon: <></> },
    ],
  },
  {
    id: 'system',
    label: '系统',
    icon: <SettingsIcon />,
    defaultExpanded: true,
    items: [
      { path: '/logs', label: 'Logs', icon: <LogsIcon /> },
      { path: '/settings', label: 'Settings', icon: <SettingsIcon /> },
    ],
  },
]

// Storage key for collapse state
const STORAGE_KEY = 'nav_collapse_state'

const Layout: Component<RouteSectionProps> = (props) => {
  const location = useLocation()
  const [sidebarCollapsed, setSidebarCollapsed] = createSignal(false)
  const [mobileMenuOpen, setMobileMenuOpen] = createSignal(false)
  const [expandedGroups, setExpandedGroups] = createSignal<Record<string, boolean>>({})
  const [searchOpen, setSearchOpen] = createSignal(false)

  // Load saved collapse state
  createEffect(() => {
    try {
      const saved = localStorage.getItem(STORAGE_KEY)
      if (saved) {
        setExpandedGroups(JSON.parse(saved))
      } else {
        // Initialize with defaults
        const defaults: Record<string, boolean> = {}
        navGroups.forEach(g => { defaults[g.id] = g.defaultExpanded ?? true })
        setExpandedGroups(defaults)
      }
    } catch {
      // Ignore errors
    }
  })

  // Save collapse state
  const saveCollapseState = (state: Record<string, boolean>) => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(state))
    } catch {
      // Ignore errors
    }
  }

  const toggleGroup = (groupId: string) => {
    setExpandedGroups(prev => {
      const newState = { ...prev, [groupId]: !prev[groupId] }
      saveCollapseState(newState)
      return newState
    })
  }

  const isActive = (path: string) => {
    if (path === '/') return location.pathname === '/'
    return location.pathname.startsWith(path)
  }

  const hasActiveItem = (items: NavItem[]) => {
    return items.some(item => isActive(item.path))
  }

  // Handle Ctrl+K / Cmd+K
  const handleKeyDown = (e: KeyboardEvent) => {
    if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
      e.preventDefault()
      setSearchOpen(true)
    }
    if (e.key === 'Escape') {
      setSearchOpen(false)
    }
  }

  createEffect(() => {
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  })

  return (
    <div class="min-h-screen flex" onKeyDown={handleKeyDown}>
      {/* Mobile Menu Button */}
      <button
        class="lg:hidden fixed top-4 left-4 z-50 btn btn-icon btn-secondary"
        onClick={() => setMobileMenuOpen(!mobileMenuOpen())}
      >
        <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
        </svg>
      </button>

      {/* Mobile Overlay */}
      <Show when={mobileMenuOpen()}>
        <div
          class="lg:hidden fixed inset-0 bg-dark-900/80 backdrop-blur-sm z-40"
          onClick={() => setMobileMenuOpen(false)}
        />
      </Show>

      {/* Sidebar */}
      <aside
        class={`fixed lg:sticky top-0 left-0 h-screen z-40 flex flex-col
                bg-dark-800/90 backdrop-blur-xl border-r border-dark-600/50
                transition-all duration-300 ease-out
                ${sidebarCollapsed() ? 'w-20' : 'w-64'}
                ${mobileMenuOpen() ? 'translate-x-0' : '-translate-x-full lg:translate-x-0'}`}
      >
        {/* Logo */}
        <div class="h-16 flex items-center justify-between px-4 border-b border-dark-600/50">
          <Show when={!sidebarCollapsed()}>
            <div class="flex items-center gap-2">
              <div class="w-8 h-8 rounded-lg bg-gradient-to-br from-primary-500 to-accent-500 flex items-center justify-center">
                <svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253" />
                </svg>
              </div>
              <span class="text-lg font-bold text-gradient">Bellkeeper</span>
            </div>
          </Show>
          <div class="flex items-center gap-1">
            {/* Search button */}
            <button
              class="btn btn-icon btn-ghost text-dark-400 hover:text-dark-200"
              onClick={() => setSearchOpen(true)}
              title="Search (Ctrl+K)"
            >
              <SearchIcon />
            </button>
            <button
              class="hidden lg:flex btn btn-icon btn-ghost text-dark-400"
              onClick={() => setSidebarCollapsed(!sidebarCollapsed())}
            >
              <svg
                class={`w-5 h-5 transition-transform ${sidebarCollapsed() ? 'rotate-180' : ''}`}
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
              >
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 19l-7-7 7-7m8 14l-7-7 7-7" />
              </svg>
            </button>
          </div>
        </div>

        {/* Navigation */}
        <nav class="flex-1 px-3 py-4 space-y-1 overflow-y-auto scrollbar-hide">
          <Show when={!sidebarCollapsed()} fallback={<CollapsedNav />}>
            <For each={navGroups}>
              {(group) => (
                <div class="mb-2">
                  {/* Group header */}
                  <button
                    class={`w-full flex items-center justify-between px-3 py-2 rounded-lg transition-colors
                            ${hasActiveItem(group.items)
                              ? 'bg-primary-500/10 text-primary-300'
                              : 'text-dark-400 hover:bg-dark-700/50 hover:text-dark-200'
                            }`}
                    onClick={() => toggleGroup(group.id)}
                  >
                    <div class="flex items-center gap-2">
                      <span class={hasActiveItem(group.items) ? 'text-primary-400' : ''}>
                        {group.icon}
                      </span>
                      <span class="text-sm font-medium">{group.label}</span>
                    </div>
                    <span
                      class={`transition-transform ${expandedGroups()[group.id] ? 'rotate-90' : ''}`}
                    >
                      <ChevronIcon />
                    </span>
                  </button>

                  {/* Group items */}
                  <Show when={expandedGroups()[group.id]}>
                    <div class="mt-1 ml-2 pl-2 border-l border-dark-600/30 space-y-0.5">
                      <For each={group.items}>
                        {(item) => (
                          <A
                            href={item.path}
                            class={`flex items-center gap-3 px-3 py-2 rounded-lg transition-all duration-200
                                    ${isActive(item.path)
                                      ? 'bg-primary-500/20 text-primary-300 shadow-sm'
                                      : 'text-dark-400 hover:bg-dark-700/50 hover:text-dark-200'
                                    }`}
                            onClick={() => setMobileMenuOpen(false)}
                          >
                            <span class={`flex-shrink-0 ${isActive(item.path) ? 'text-primary-400' : ''}`}>
                              {item.icon}
                            </span>
                            <span class="text-sm font-medium">{item.label}</span>
                          </A>
                        )}
                      </For>
                    </div>
                  </Show>
                </div>
              )}
            </For>
          </Show>
        </nav>

        {/* Footer */}
        <div class="px-4 py-3 border-t border-dark-600/50">
          <Show
            when={!sidebarCollapsed()}
            fallback={
              <div class="flex justify-center">
                <div class="status-dot status-dot-success" />
              </div>
            }
          >
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2">
                <div class="status-dot status-dot-success" />
                <span class="text-xs text-dark-500">系统正常</span>
              </div>
              <span class="text-xs text-dark-600">v1.0.0</span>
            </div>
          </Show>
        </div>
      </aside>

      {/* Search Modal */}
      <GlobalSearch open={searchOpen()} onClose={() => setSearchOpen(false)} />

      {/* Main content */}
      <main class="flex-1 min-h-screen lg:min-w-0">
        <div class="p-4 lg:p-6 pt-16 lg:pt-6">
          {props.children}
        </div>
      </main>
    </div>
  )
}

// Collapsed navigation (just icons)
const CollapsedNav: Component = () => {
  const location = useLocation()

  const isActive = (path: string) => {
    if (path === '/') return location.pathname === '/'
    return location.pathname.startsWith(path)
  }

  return (
    <div class="space-y-1">
      <For each={navGroups}>
        {(group) => (
          <A
            href={group.items[0].path}
            class={`flex justify-center p-3 rounded-xl transition-all
                    ${isActive(group.items[0].path)
                      ? 'bg-primary-500/20 text-primary-300'
                      : 'text-dark-400 hover:bg-dark-700/50 hover:text-dark-200'
                    }`}
            title={group.label}
          >
            <span class={isActive(group.items[0].path) ? 'text-primary-400' : ''}>
              {group.icon}
            </span>
          </A>
        )}
      </For>
    </div>
  )
}

export default Layout

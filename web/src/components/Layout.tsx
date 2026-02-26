import { Component, JSX, createSignal, Show } from 'solid-js'
import { A, useLocation, RouteSectionProps } from '@solidjs/router'

interface NavItem {
  path: string
  label: string
  icon: JSX.Element
  badge?: string
}

const navItems: NavItem[] = [
  {
    path: '/',
    label: '仪表盘',
    icon: (
      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 5a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM14 5a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1V5zM4 15a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1H5a1 1 0 01-1-1v-4zM14 15a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1v-4z" />
      </svg>
    ),
  },
  {
    path: '/tags',
    label: '标签管理',
    icon: (
      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M7 7h.01M7 3h5c.512 0 1.024.195 1.414.586l7 7a2 2 0 010 2.828l-7 7a2 2 0 01-2.828 0l-7-7A1.994 1.994 0 013 12V7a4 4 0 014-4z" />
      </svg>
    ),
  },
  {
    path: '/datasources',
    label: '数据源',
    icon: (
      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4" />
      </svg>
    ),
  },
  {
    path: '/rss',
    label: 'RSS 订阅',
    icon: (
      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M6 5c7.18 0 13 5.82 13 13M6 11a7 7 0 017 7m-6 0a1 1 0 11-2 0 1 1 0 012 0z" />
      </svg>
    ),
  },
  {
    path: '/datasets',
    label: '知识库映射',
    icon: (
      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
      </svg>
    ),
  },
  {
    path: '/documents',
    label: 'RagFlow 文档',
    icon: (
      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
      </svg>
    ),
  },
  {
    path: '/webhooks',
    label: 'Webhook',
    icon: (
      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
      </svg>
    ),
  },
  {
    path: '/workflows',
    label: 'n8n 工作流',
    icon: (
      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
      </svg>
    ),
  },
  {
    path: '/settings',
    label: '设置',
    icon: (
      <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
      </svg>
    ),
  },
]

const Layout: Component<RouteSectionProps> = (props) => {
  const location = useLocation()
  const [sidebarCollapsed, setSidebarCollapsed] = createSignal(false)
  const [mobileMenuOpen, setMobileMenuOpen] = createSignal(false)

  const isActive = (path: string) => {
    if (path === '/') {
      return location.pathname === '/'
    }
    return location.pathname.startsWith(path)
  }

  return (
    <div class="min-h-screen flex">
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

        {/* Navigation */}
        <nav class="flex-1 px-3 py-4 space-y-1 overflow-y-auto scrollbar-hide">
          {navItems.map((item) => (
            <A
              href={item.path}
              class={`flex items-center gap-3 px-3 py-2.5 rounded-xl transition-all duration-200 group
                      ${sidebarCollapsed() ? 'justify-center' : ''}
                      ${isActive(item.path)
                        ? 'bg-primary-500/20 text-primary-300 shadow-sm'
                        : 'text-dark-400 hover:bg-dark-700/50 hover:text-dark-200'
                      }`}
              onClick={() => setMobileMenuOpen(false)}
            >
              <span class={`flex-shrink-0 ${isActive(item.path) ? 'text-primary-400' : ''}`}>
                {item.icon}
              </span>
              <Show when={!sidebarCollapsed()}>
                <span class="flex-1 text-sm font-medium">{item.label}</span>
                <Show when={item.badge}>
                  <span class="badge badge-primary">{item.badge}</span>
                </Show>
              </Show>
            </A>
          ))}
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

      {/* Main content */}
      <main class="flex-1 min-h-screen lg:min-w-0">
        <div class="p-4 lg:p-6 pt-16 lg:pt-6">
          {props.children}
        </div>
      </main>
    </div>
  )
}

export default Layout

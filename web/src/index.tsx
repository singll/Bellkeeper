/* @refresh reload */
import { render } from 'solid-js/web'
import { ErrorBoundary } from 'solid-js'
import { Router, Route } from '@solidjs/router'
import { ToastProvider } from './components/Toast'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import Tags from './pages/Tags'
import RSSFeeds from './pages/RSSFeeds'
import Workflows from './pages/Workflows'
import Settings from './pages/Settings'
import CrawlQueue from './pages/CrawlQueue'
import MatrixDashboard from './pages/MatrixDashboard'
import MatrixConsole from './pages/MatrixConsole'
// Knowledge pages
import { KnowledgeFiles, KnowledgeSearch, KnowledgeAsk, KnowledgeSkeleton, KnowledgeOverview } from './pages/knowledge'
// LLM pages (split from old LLMProxy)
import { LLMOverview, LLMChannels, LLMGroupsAndRouting, LLMUsageAndBilling, LLMLogsAndAlerts } from './pages/llm'
// Log pages (split from old LogCenter)
import { LogBrowser, LogDashboard, LogSources, LogAlerts } from './pages/logs'
import ErrorFallback from './components/ErrorFallback'
import './index.css'

const root = document.getElementById('root')

// Redirect old URL
import { useNavigate } from '@solidjs/router'
const LLMProxyRedirect = () => { const nav = useNavigate(); nav('/llm', { replace: true }); return null }

render(() => (
  <ErrorBoundary fallback={(err) => <ErrorFallback error={err} />}>
    <ToastProvider>
      <Router root={Layout}>
        <Route path="/" component={Dashboard} />
        {/* Knowledge (core system) */}
        <Route path="/knowledge/overview" component={KnowledgeOverview} />
        <Route path="/knowledge/search" component={KnowledgeSearch} />
        <Route path="/knowledge/ask" component={KnowledgeAsk} />
        <Route path="/knowledge/skeleton" component={KnowledgeSkeleton} />
        <Route path="/knowledge/files" component={KnowledgeFiles} />
        <Route path="/rss" component={RSSFeeds} />
        <Route path="/crawl-queue" component={CrawlQueue} />
        <Route path="/tags" component={Tags} />
        {/* LLM (core system, split from old /llm-proxy) */}
        <Route path="/llm" component={LLMOverview} />
        <Route path="/llm/channels" component={LLMChannels} />
        <Route path="/llm/groups-routing" component={LLMGroupsAndRouting} />
        <Route path="/llm/usage-billing" component={LLMUsageAndBilling} />
        <Route path="/llm/logs-alerts" component={LLMLogsAndAlerts} />
        <Route path="/llm-proxy" component={LLMProxyRedirect} />
        {/* Logs (core system, split from old /logs tab) */}
        <Route path="/logs" component={LogBrowser} />
        <Route path="/logs/dashboard" component={LogDashboard} />
        <Route path="/logs/sources" component={LogSources} />
        <Route path="/logs/alerts" component={LogAlerts} />
        {/* Matrix (1.0: 7→2 页重构，§2.3.3 T9；配置归全局 Settings) */}
        <Route path="/matrix" component={MatrixDashboard} />
        <Route path="/matrix/console" component={MatrixConsole} />
        {/* More */}
        <Route path="/workflows" component={Workflows} />
        <Route path="/settings" component={Settings} />
      </Router>
    </ToastProvider>
  </ErrorBoundary>
), root!)

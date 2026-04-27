/* @refresh reload */
import { render } from 'solid-js/web'
import { ErrorBoundary } from 'solid-js'
import { Router, Route } from '@solidjs/router'
import { ToastProvider } from './components/Toast'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import Tags from './pages/Tags'
import RSSFeeds from './pages/RSSFeeds'
import Datasets from './pages/Datasets'
import Documents from './pages/Documents'
import Workflows from './pages/Workflows'
import Settings from './pages/Settings'
import MatrixDashboard from './pages/MatrixDashboard'
import MatrixRooms from './pages/MatrixRooms'
import MatrixChannels from './pages/MatrixChannels'
import MatrixCommands from './pages/MatrixCommands'
import MatrixNotifications from './pages/MatrixNotifications'
import MatrixEvents from './pages/MatrixEvents'
import MatrixCommandLogs from './pages/MatrixCommandLogs'
// Knowledge pages
import { KnowledgeFiles, KnowledgeSearch, KnowledgeAsk } from './pages/knowledge'
// LLM pages (split from old LLMProxy)
import { LLMOverview, LLMChannels, LLMGroups, LLMConfig, LLMLogs } from './pages/llm'
// Log pages (split from old LogCenter)
import { LogBrowser, LogDashboard, LogSources, LogAlerts, LogParseTasks } from './pages/logs'
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
        <Route path="/knowledge/files" component={KnowledgeFiles} />
        <Route path="/knowledge/search" component={KnowledgeSearch} />
        <Route path="/knowledge/ask" component={KnowledgeAsk} />
        <Route path="/rss" component={RSSFeeds} />
        <Route path="/tags" component={Tags} />
        <Route path="/datasets" component={Datasets} />
        {/* LLM (core system, split from old /llm-proxy) */}
        <Route path="/llm" component={LLMOverview} />
        <Route path="/llm/channels" component={LLMChannels} />
        <Route path="/llm/groups" component={LLMGroups} />
        <Route path="/llm/config" component={LLMConfig} />
        <Route path="/llm/logs" component={LLMLogs} />
        <Route path="/llm-proxy" component={LLMProxyRedirect} />
        {/* Logs (core system, split from old /logs tab) */}
        <Route path="/logs" component={LogBrowser} />
        <Route path="/logs/dashboard" component={LogDashboard} />
        <Route path="/logs/sources" component={LogSources} />
        <Route path="/logs/alerts" component={LogAlerts} />
        <Route path="/logs/parse-tasks" component={LogParseTasks} />
        {/* Matrix (core system) */}
        <Route path="/matrix" component={MatrixDashboard} />
        <Route path="/matrix/rooms" component={MatrixRooms} />
        <Route path="/matrix/channels" component={MatrixChannels} />
        <Route path="/matrix/commands" component={MatrixCommands} />
        <Route path="/matrix/notifications" component={MatrixNotifications} />
        <Route path="/matrix/events" component={MatrixEvents} />
        <Route path="/matrix/command-logs" component={MatrixCommandLogs} />
        {/* More */}
        <Route path="/workflows" component={Workflows} />
        <Route path="/documents" component={Documents} />
        <Route path="/settings" component={Settings} />
      </Router>
    </ToastProvider>
  </ErrorBoundary>
), root!)

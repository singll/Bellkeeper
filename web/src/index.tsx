/* @refresh reload */
import { render } from 'solid-js/web'
import { Router, Route } from '@solidjs/router'
import { ToastProvider } from './components/Toast'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import Tags from './pages/Tags'
import RSSFeeds from './pages/RSSFeeds'
import Datasets from './pages/Datasets'
import Documents from './pages/Documents'
import Workflows from './pages/Workflows'
import LLMProxy from './pages/LLMProxy'
import Logs from './pages/Logs'
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
import './index.css'

const root = document.getElementById('root')

render(() => (
  <ToastProvider>
    <Router root={Layout}>
      <Route path="/" component={Dashboard} />
      {/* Knowledge (new file-based knowledge management) */}
      <Route path="/knowledge/files" component={KnowledgeFiles} />
      <Route path="/knowledge/search" component={KnowledgeSearch} />
      <Route path="/knowledge/ask" component={KnowledgeAsk} />
      {/* Knowledge supporting features */}
      <Route path="/rss" component={RSSFeeds} />
      <Route path="/tags" component={Tags} />
      {/* Archive (RAGFlow documents) */}
      <Route path="/documents" component={Documents} />
      {/* Datasets */}
      <Route path="/datasets" component={Datasets} />
      {/* AI Services */}
      <Route path="/workflows" component={Workflows} />
      <Route path="/llm-proxy" component={LLMProxy} />
      {/* System */}
      <Route path="/logs" component={Logs} />
      <Route path="/settings" component={Settings} />
      {/* Matrix */}
      <Route path="/matrix" component={MatrixDashboard} />
      <Route path="/matrix/rooms" component={MatrixRooms} />
      <Route path="/matrix/channels" component={MatrixChannels} />
      <Route path="/matrix/commands" component={MatrixCommands} />
      <Route path="/matrix/notifications" component={MatrixNotifications} />
      <Route path="/matrix/events" component={MatrixEvents} />
      <Route path="/matrix/command-logs" component={MatrixCommandLogs} />
    </Router>
  </ToastProvider>
), root!)


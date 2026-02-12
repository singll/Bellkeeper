/* @refresh reload */
import { render } from 'solid-js/web'
import { Router, Route } from '@solidjs/router'
import { ToastProvider } from './components/Toast'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import Tags from './pages/Tags'
import DataSources from './pages/DataSources'
import RSSFeeds from './pages/RSSFeeds'
import Datasets from './pages/Datasets'
import Documents from './pages/Documents'
import Webhooks from './pages/Webhooks'
import Workflows from './pages/Workflows'
import Settings from './pages/Settings'
import './index.css'

const root = document.getElementById('root')

render(() => (
  <ToastProvider>
    <Router root={Layout}>
      <Route path="/" component={Dashboard} />
      <Route path="/tags" component={Tags} />
      <Route path="/datasources" component={DataSources} />
      <Route path="/rss" component={RSSFeeds} />
      <Route path="/datasets" component={Datasets} />
      <Route path="/documents" component={Documents} />
      <Route path="/webhooks" component={Webhooks} />
      <Route path="/workflows" component={Workflows} />
      <Route path="/settings" component={Settings} />
    </Router>
  </ToastProvider>
), root!)

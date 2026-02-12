/* @refresh reload */
import { render } from 'solid-js/web'
import { Router, Route } from '@solidjs/router'
import Layout from './components/Layout'
import Dashboard from './pages/Dashboard'
import Tags from './pages/Tags'
import DataSources from './pages/DataSources'
import RSSFeeds from './pages/RSSFeeds'
import Datasets from './pages/Datasets'
import Documents from './pages/Documents'
import Webhooks from './pages/Webhooks'
import Settings from './pages/Settings'
import './index.css'

const root = document.getElementById('root')

render(() => (
  <Router root={Layout}>
    <Route path="/" component={Dashboard} />
    <Route path="/tags" component={Tags} />
    <Route path="/datasources" component={DataSources} />
    <Route path="/rss" component={RSSFeeds} />
    <Route path="/datasets" component={Datasets} />
    <Route path="/documents" component={Documents} />
    <Route path="/webhooks" component={Webhooks} />
    <Route path="/settings" component={Settings} />
  </Router>
), root!)

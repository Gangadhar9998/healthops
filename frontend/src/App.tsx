import { lazy, Suspense } from 'react'
import { Routes, Route } from 'react-router-dom'
import { Layout } from '@/components/Layout'
import { LoadingState } from '@/components/LoadingState'

const Dashboard = lazy(() => import('@/pages/Dashboard'))
const Checks = lazy(() => import('@/pages/Checks'))
const CheckDetail = lazy(() => import('@/pages/CheckDetail'))
const Incidents = lazy(() => import('@/pages/Incidents'))
const IncidentDetail = lazy(() => import('@/pages/IncidentDetail'))
const MySQL = lazy(() => import('@/pages/MySQL'))
const MySQLConnections = lazy(() => import('@/pages/mysql/MySQLConnections'))
const MySQLQueries = lazy(() => import('@/pages/mysql/MySQLQueries'))
const MySQLThreads = lazy(() => import('@/pages/mysql/MySQLThreads'))
const MySQLServer = lazy(() => import('@/pages/mysql/MySQLServer'))
const Analytics = lazy(() => import('@/pages/Analytics'))
const AIAnalysis = lazy(() => import('@/pages/AIAnalysis'))
const Settings = lazy(() => import('@/pages/Settings'))
const NotFound = lazy(() => import('@/pages/NotFound'))

export default function App() {
  return (
    <Suspense fallback={<LoadingState message="Loading…" />}>
      <Routes>
        <Route element={<Layout />}>
          <Route index element={<Dashboard />} />
          <Route path="checks" element={<Checks />} />
          <Route path="checks/:id" element={<CheckDetail />} />
          <Route path="incidents" element={<Incidents />} />
          <Route path="incidents/:id" element={<IncidentDetail />} />
          <Route path="mysql" element={<MySQL />} />
          <Route path="mysql/connections" element={<MySQLConnections />} />
          <Route path="mysql/queries" element={<MySQLQueries />} />
          <Route path="mysql/threads" element={<MySQLThreads />} />
          <Route path="mysql/server" element={<MySQLServer />} />
          <Route path="analytics" element={<Analytics />} />
          <Route path="ai" element={<AIAnalysis />} />
          <Route path="settings" element={<Settings />} />
          <Route path="*" element={<NotFound />} />
        </Route>
      </Routes>
    </Suspense>
  )
}

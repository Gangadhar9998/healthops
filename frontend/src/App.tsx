import { lazy, Suspense } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import { Layout } from '@/components/Layout'
import { LoadingState } from '@/components/LoadingState'
import { useAuth } from '@/hooks/useAuth'

const Login = lazy(() => import('@/pages/Login'))
const Dashboard = lazy(() => import('@/pages/Dashboard'))
const Servers = lazy(() => import('@/pages/Servers'))
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
const UserManagement = lazy(() => import('@/pages/UserManagement'))
const NotificationChannels = lazy(() => import('@/pages/NotificationChannels'))
const NotFound = lazy(() => import('@/pages/NotFound'))

export default function App() {
  const { isAuthenticated, isLoading } = useAuth()

  if (isLoading) {
    return <LoadingState message="Loading…" />
  }

  return (
    <Suspense fallback={<LoadingState message="Loading…" />}>
      <Routes>
        {!isAuthenticated ? (
          <>
            <Route path="/login" element={<Login />} />
            <Route path="*" element={<Navigate to="/login" replace />} />
          </>
        ) : (
          <Route element={<Layout />}>
            <Route index element={<Dashboard />} />
            <Route path="servers" element={<Servers />} />
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
            <Route path="users" element={<UserManagement />} />
            <Route path="notifications" element={<NotificationChannels />} />
            <Route path="login" element={<Navigate to="/" replace />} />
            <Route path="*" element={<NotFound />} />
          </Route>
        )}
      </Routes>
    </Suspense>
  )
}

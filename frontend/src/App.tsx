import { Button, Result } from 'antd'
import { lazy } from 'react'
import { BrowserRouter, Route, Routes, useNavigate } from 'react-router'
import { AppLayout } from './components/layout/AppLayout'
import { useAlertSocket } from './hooks/useAlertSocket'

// Each page is its own chunk, loaded on first visit (smaller initial download).
const DashboardPage = lazy(() => import('./pages/DashboardPage').then((m) => ({ default: m.DashboardPage })))
const GeofencesPage = lazy(() => import('./pages/GeofencesPage').then((m) => ({ default: m.GeofencesPage })))
const VehiclesPage = lazy(() => import('./pages/VehiclesPage').then((m) => ({ default: m.VehiclesPage })))
const TrackingPage = lazy(() => import('./pages/TrackingPage').then((m) => ({ default: m.TrackingPage })))
const AlertRulesPage = lazy(() => import('./pages/AlertRulesPage').then((m) => ({ default: m.AlertRulesPage })))
const LiveAlertsPage = lazy(() => import('./pages/LiveAlertsPage').then((m) => ({ default: m.LiveAlertsPage })))
const ViolationHistoryPage = lazy(() => import('./pages/ViolationHistoryPage').then((m) => ({ default: m.ViolationHistoryPage })))

function NotFound() {
  const navigate = useNavigate()
  return <Result status="404" title="Page not found" extra={<Button onClick={() => navigate('/')}>Back to dashboard</Button>} />
}

function Shell() {
  useAlertSocket() // one WebSocket for the whole app, alive on every page
  return (
    <Routes>
      <Route element={<AppLayout />}>
        <Route index element={<DashboardPage />} />
        <Route path="geofences" element={<GeofencesPage />} />
        <Route path="vehicles" element={<VehiclesPage />} />
        <Route path="tracking" element={<TrackingPage />} />
        <Route path="alert-rules" element={<AlertRulesPage />} />
        <Route path="alerts" element={<LiveAlertsPage />} />
        <Route path="history" element={<ViolationHistoryPage />} />
        <Route path="*" element={<NotFound />} />
      </Route>
    </Routes>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <Shell />
    </BrowserRouter>
  )
}

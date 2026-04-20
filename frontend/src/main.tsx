import { QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { createBrowserRouter, RouterProvider } from 'react-router'

import { ErrorBoundary } from './components/common/ErrorBoundary'
import { OfflineBanner } from './components/common/OfflineBanner'
import { Toaster } from './components/ui/sonner'
import RootLayout from './layouts/RootLayout'
import { queryClient } from './lib/queryClient'
import AIChatPage from './pages/AIChatPage'
import AppListPage from './pages/AppListPage'
import AppViewPage from './pages/AppViewPage'
import EntryPage from './pages/EntryPage'
import GlobalCalendarPage from './pages/GlobalCalendarPage'
import GlobalDashboardPage from './pages/GlobalDashboardPage'
import LoginPage from './pages/LoginPage'
import MyTasksPage from './pages/MyTasksPage'
import NotFoundPage from './pages/NotFoundPage'
import OrgChartPage from './pages/OrgChartPage'
import ProfilePage from './pages/ProfilePage'
import SettingsPage from './pages/SettingsPage'
import UsersPage from './pages/UsersPage'
import './index.css'

const EB = ErrorBoundary

const router = createBrowserRouter([
  {
    path: '/login',
    element: <LoginPage />,
  },
  {
    element: <RootLayout />,
    children: [
      { index: true, element: <EB><AppListPage /></EB> },
      { path: 'apps', element: <EB><AppListPage /></EB> },
      { path: 'apps/:appId', element: <EB><AppViewPage /></EB> },
      { path: 'apps/:appId/entries/new', element: <EB><EntryPage /></EB> },
      { path: 'apps/:appId/entries/:entryId', element: <EB><EntryPage /></EB> },
      { path: 'my-tasks', element: <EB><MyTasksPage /></EB> },
      { path: 'dashboard', element: <EB><GlobalDashboardPage /></EB> },
      { path: 'calendar', element: <EB><GlobalCalendarPage /></EB> },
      { path: 'settings', element: <EB><SettingsPage /></EB> },
      { path: 'admin/users', element: <EB><UsersPage /></EB> },
      { path: 'admin/org', element: <EB><OrgChartPage /></EB> },
      { path: 'ai', element: <EB><AIChatPage /></EB> },
      { path: 'profile', element: <EB><ProfilePage /></EB> },
      { path: '*', element: <NotFoundPage /> },
    ],
  },
])

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <ErrorBoundary>
        <OfflineBanner />
        <RouterProvider router={router} />
      </ErrorBoundary>
      <Toaster richColors closeButton position="top-right" />
    </QueryClientProvider>
  </StrictMode>,
)

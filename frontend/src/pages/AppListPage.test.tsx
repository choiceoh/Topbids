import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import AppListPage from './AppListPage'
import { renderWithProviders, jsonResponse } from '@/test/helpers'
import type { Collection } from '@/lib/types'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

vi.mock('@/components/works/AppCard', () => ({
  default: ({ collection }: { collection: Collection }) => (
    <div data-testid="app-card">{collection.label}</div>
  ),
}))

// Mock RoleGate to always render children for testing.
vi.mock('@/components/common/RoleGate', () => ({
  default: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

const mockFetch = vi.fn()

beforeEach(() => {
  vi.stubGlobal('fetch', mockFetch)
})

afterEach(() => {
  vi.restoreAllMocks()
})

const collections: Collection[] = [
  {
    id: 'c1', slug: 'permits', label: '인허가', description: '인허가 관리',
    is_system: false, process_enabled: false, sort_order: 0, access_config: {},
    created_at: '2024-01-01', updated_at: '2024-01-01',
  },
  {
    id: 'c2', slug: 'tasks', label: '앱 관리', description: '앱을 관리합니다',
    is_system: false, process_enabled: false, sort_order: 1, access_config: {},
    created_at: '2024-01-02', updated_at: '2024-01-02',
  },
]

function setupDefaultMocks() {
  mockFetch.mockImplementation((url: string) => {
    if (url.includes('/schema/collections/counts')) {
      return jsonResponse({ data: { permits: 10, tasks: 25 } })
    }
    if (url.includes('/schema/collections')) {
      return jsonResponse({ data: collections })
    }
    return jsonResponse({ data: null })
  })
}

function renderPage() {
  return renderWithProviders(<AppListPage />, {
    route: '/apps',
    path: '/apps',
  })
}

describe('AppListPage', () => {
  it('renders page header and collection cards', async () => {
    setupDefaultMocks()
    renderPage()

    await waitFor(() => {
      expect(screen.getByText('인허가')).toBeInTheDocument()
      expect(screen.getByText('앱 관리')).toBeInTheDocument()
    })
  })

  it('shows empty state when no collections', async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes('/schema/collections/counts')) {
        return jsonResponse({ data: {} })
      }
      if (url.includes('/schema/collections')) {
        return jsonResponse({ data: [] })
      }
      return jsonResponse({ data: null })
    })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('앱이 아직 없습니다')).toBeInTheDocument()
    })
  })

  it('filters collections by search term', async () => {
    setupDefaultMocks()
    const user = userEvent.setup()
    renderPage()

    await waitFor(() => {
      expect(screen.getByText('인허가')).toBeInTheDocument()
    })

    const searchInput = screen.getByPlaceholderText('앱 검색…')
    await user.type(searchInput, '인허가')

    expect(screen.getByText('인허가')).toBeInTheDocument()
    expect(screen.queryByText('앱 관리')).not.toBeInTheDocument()
  })

  it('shows no results message when search has no matches', async () => {
    setupDefaultMocks()
    const user = userEvent.setup()
    renderPage()

    await waitFor(() => {
      expect(screen.getByText('인허가')).toBeInTheDocument()
    })

    const searchInput = screen.getByPlaceholderText('앱 검색…')
    await user.type(searchInput, '존재하지않는앱')

    await waitFor(() => {
      expect(screen.getByText(/에 해당하는 앱이 없습니다/)).toBeInTheDocument()
    })
  })

  it('shows error state when fetch fails', async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url.includes('/schema/collections') && !url.includes('counts')) {
        return jsonResponse({ code: 'INTERNAL', message: 'Server error' }, 500)
      }
      return jsonResponse({ data: null })
    })

    renderPage()

    await waitFor(() => {
      expect(screen.getByText('불러오지 못했습니다')).toBeInTheDocument()
    })
  })

})

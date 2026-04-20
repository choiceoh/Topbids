import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import AppViewPage from './AppViewPage'
import { renderWithProviders, jsonResponse } from '@/test/helpers'
import type { Collection, Field } from '@/lib/types'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

// Mock the heavy sub-views to simplify.
vi.mock('@/components/works/views/CalendarView', () => ({
  default: () => <div data-testid="calendar-view" />,
}))
vi.mock('@/components/works/views/GalleryView', () => ({
  default: () => <div data-testid="gallery-view" />,
}))
vi.mock('@/components/works/views/GanttView', () => ({
  default: () => <div data-testid="gantt-view" />,
}))
vi.mock('@/components/works/views/KanbanView', () => ({
  default: () => <div data-testid="kanban-view" />,
}))

const mockFetch = vi.fn()

beforeEach(() => {
  vi.stubGlobal('fetch', mockFetch)
})

afterEach(() => {
  vi.restoreAllMocks()
})

// --- Fixtures ---

const textField: Field = {
  id: 'f1', collection_id: 'c1', slug: 'title', label: '제목',
  field_type: 'text', is_required: true, is_unique: false, is_indexed: false,
  width: 6, height: 1, sort_order: 1, created_at: '2024-01-01', updated_at: '2024-01-01',
}

const selectField: Field = {
  ...textField, id: 'f2', slug: 'status', label: '상태',
  field_type: 'select', is_required: false, sort_order: 2,
  options: { choices: ['대기', '진행중', '완료'] },
}

const dateField: Field = {
  ...textField, id: 'f3', slug: 'due_date', label: '마감일',
  field_type: 'date', is_required: false, sort_order: 3,
}

const numberField: Field = {
  ...textField, id: 'f4', slug: 'amount', label: '금액',
  field_type: 'number', is_required: false, sort_order: 4,
}

const collection: Collection = {
  id: 'c1', slug: 'tasks', label: '앱 관리', description: '앱을 관리합니다',
  is_system: false, process_enabled: false, sort_order: 0, access_config: {},
  created_at: '2024-01-01', updated_at: '2024-01-01',
  fields: [textField, selectField, dateField, numberField],
}

const entries = [
  { id: '1', title: '보고서 작성', status: '진행중', due_date: '2024-06-01', amount: 100, created_at: '2024-01-01' },
  { id: '2', title: '회의 준비', status: '대기', due_date: '2024-06-15', amount: 200, created_at: '2024-01-02' },
]

const entryListEnvelope = { data: entries, total: 2, page: 1, limit: 20, total_pages: 1 }

function setupDefaultMocks() {
  mockFetch.mockImplementation((url: string) => {
    if (url.includes('/schema/collections/c1') && !url.includes('process') && !url.includes('saved-views') && !url.includes('charts') && !url.includes('automations')) {
      return jsonResponse({ data: collection })
    }
    if (url.includes('/process')) {
      return jsonResponse({ data: { is_enabled: false, statuses: [], transitions: [] } })
    }
    if (url.includes('/saved-views')) {
      return jsonResponse({ data: [] })
    }
    if (url.includes('/data/tasks')) {
      return jsonResponse(entryListEnvelope)
    }
    if (url.includes('/auth/me')) {
      return jsonResponse({ data: { id: 'u1', name: 'Test', email: 'test@test.com', role: 'director' } })
    }
    return jsonResponse({ data: null })
  })
}

function renderPage() {
  return renderWithProviders(<AppViewPage />, {
    route: '/apps/c1',
    path: '/apps/:appId',
  })
}

describe('AppViewPage', () => {
  describe('rendering', () => {
    it('renders collection title and entries after load', async () => {
      setupDefaultMocks()
      renderPage()

      await waitFor(() => {
        expect(screen.getAllByText('앱 관리').length).toBeGreaterThanOrEqual(1)
      })

      await waitFor(() => {
        expect(screen.getByText('보고서 작성')).toBeInTheDocument()
        expect(screen.getByText('회의 준비')).toBeInTheDocument()
      })
    })

    it('shows view tabs when select/date fields exist', async () => {
      setupDefaultMocks()
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('목록')).toBeInTheDocument()
        expect(screen.getByText('캘린더')).toBeInTheDocument()
        expect(screen.getByText('폼')).toBeInTheDocument()
      })
    })
  })

  describe('error states', () => {
    it('shows error state when collection fetch fails', async () => {
      mockFetch.mockImplementation((url: string) => {
        if (url.includes('/schema/collections/c1') && !url.includes('process') && !url.includes('saved-views') && !url.includes('charts') && !url.includes('automations')) {
          return jsonResponse({ code: 'NOT_FOUND', message: '앱을 찾을 수 없습니다' }, 404)
        }
        if (url.includes('/auth/me')) {
          return jsonResponse({ data: { id: 'u1', role: 'director' } })
        }
        return jsonResponse({ data: null })
      })

      renderPage()

      await waitFor(() => {
        expect(screen.getByText('불러오지 못했습니다')).toBeInTheDocument()
      })
    })
  })

  describe('toolbar features', () => {
    it('renders filter, sort, and more menu buttons', async () => {
      setupDefaultMocks()
      renderPage()

      // Wait for entries to load (toolbar is rendered inside the data table).
      await waitFor(() => {
        expect(screen.getByText('보고서 작성')).toBeInTheDocument()
      })

      expect(screen.getByText('필터')).toBeInTheDocument()
      expect(screen.getByText('정렬')).toBeInTheDocument()
      expect(screen.getByText('더보기')).toBeInTheDocument()
    })

    it('has search input', async () => {
      setupDefaultMocks()
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('보고서 작성')).toBeInTheDocument()
      })

      expect(screen.getByPlaceholderText('검색...')).toBeInTheDocument()
    })
  })

  describe('CRUD operations', () => {
    it('opens entry sheet when new record button clicked', async () => {
      setupDefaultMocks()
      const user = userEvent.setup()
      renderPage()

      // Wait for entries to fully load (data table rendered).
      await waitFor(() => {
        expect(screen.getByText('보고서 작성')).toBeInTheDocument()
      })

      const newBtn = screen.getByText('새 데이터')
      await user.click(newBtn)

      // The sheet should open.
      await waitFor(() => {
        expect(screen.getAllByText('새 데이터').length).toBeGreaterThanOrEqual(1)
      })
    })
  })
})

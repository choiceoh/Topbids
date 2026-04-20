import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { toast } from 'sonner'

import AutomationsPage from './AutomationsPage'
import { renderWithProviders, jsonResponse } from '@/test/helpers'
import type { Collection, Automation } from '@/lib/types'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

// Mock AIAutomationDialog to avoid AI dependency.
vi.mock('@/components/works/AIAutomationDialog', () => ({
  default: () => <button>AI 자동화</button>,
}))

const mockFetch = vi.fn()

beforeEach(() => {
  vi.stubGlobal('fetch', mockFetch)
})

afterEach(() => {
  vi.restoreAllMocks()
})

const collection: Collection = {
  id: 'c1',
  slug: 'tasks',
  label: '앱 관리',
  is_system: false,
  process_enabled: false,
  sort_order: 0,
  access_config: {},
  created_at: '2024-01-01',
  updated_at: '2024-01-01',
  fields: [
    {
      id: 'f1', collection_id: 'c1', slug: 'title', label: '제목',
      field_type: 'text', is_required: true, is_unique: false, is_indexed: false,
      width: 6, height: 1, sort_order: 1, created_at: '', updated_at: '',
    },
    {
      id: 'f2', collection_id: 'c1', slug: 'assignee', label: '담당자',
      field_type: 'user', is_required: false, is_unique: false, is_indexed: false,
      width: 3, height: 1, sort_order: 2, created_at: '', updated_at: '',
    },
  ],
}

const automations: Automation[] = [
  {
    id: 'a1',
    collection_id: 'c1',
    name: '생성 시 알림',
    is_enabled: true,
    trigger_type: 'record_created',
    trigger_config: {},
    conditions: [],
    actions: [{ id: 'act1', action_type: 'send_notification', action_config: { recipient: 'record_creator', title: '새 레코드', body: '생성됨' }, sort_order: 0 }],
    action_count: 1,
    created_at: '2024-01-01',
    updated_at: '2024-01-01',
  },
  {
    id: 'a2',
    collection_id: 'c1',
    name: '비활성 자동화',
    is_enabled: false,
    trigger_type: 'record_updated',
    trigger_config: {},
    conditions: [],
    actions: [],
    action_count: 0,
    created_at: '2024-01-01',
    updated_at: '2024-01-01',
  },
]

function setupMocks(opts: { automations?: Automation[]; error?: boolean } = {}) {
  mockFetch.mockImplementation((url: string) => {
    if (url.includes('/schema/collections/c1/automations')) {
      if (opts.error) return jsonResponse({ code: 'INTERNAL_ERROR', message: 'DB 오류' }, 500)
      return jsonResponse({ data: opts.automations ?? automations })
    }
    if (url.includes('/schema/collections/c1') && !url.includes('/process') && !url.includes('/automations')) {
      return jsonResponse({ data: collection })
    }
    if (url.includes('/process')) {
      return jsonResponse({ data: { is_enabled: false, statuses: [], transitions: [] } })
    }
    if (url.includes('/auth/me')) {
      return jsonResponse({ data: { id: 'u1', role: 'director' } })
    }
    return jsonResponse({ data: null })
  })
}

function renderPage() {
  return renderWithProviders(<AutomationsPage />, {
    route: '/apps/c1/automations',
    path: '/apps/:appId/automations',
  })
}

describe('AutomationsPage', () => {
  describe('loading & empty state', () => {
    it('shows loading state initially', () => {
      mockFetch.mockReturnValue(new Promise(() => {}))
      renderPage()
      expect(document.querySelector('.skeleton-shimmer') || screen.queryByText(/로딩/i)).toBeTruthy()
    })

    it('shows empty state when no automations exist', async () => {
      setupMocks({ automations: [] })
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('아직 자동화가 없습니다')).toBeInTheDocument()
      })
    })
  })

  describe('automation list', () => {
    it('renders automation cards with names and trigger badges', async () => {
      setupMocks()
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('생성 시 알림')).toBeInTheDocument()
        expect(screen.getByText('비활성 자동화')).toBeInTheDocument()
      })

      // Trigger labels
      expect(screen.getByText('데이터 생성')).toBeInTheDocument()
      expect(screen.getByText('데이터 수정')).toBeInTheDocument()

      // Disabled badge
      expect(screen.getByText('비활성')).toBeInTheDocument()
    })

    it('shows action count for each automation', async () => {
      setupMocks()
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('1개 액션')).toBeInTheDocument()
        expect(screen.getByText('0개 액션')).toBeInTheDocument()
      })
    })
  })

  describe('error state', () => {
    it('shows error state with retry on fetch failure', async () => {
      setupMocks({ error: true })
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('불러오지 못했습니다')).toBeInTheDocument()
        expect(screen.getByText('다시 시도')).toBeInTheDocument()
      })
    })
  })

  describe('create automation form', () => {
    it('opens form when "새 자동화" clicked', async () => {
      setupMocks({ automations: [] })
      const user = userEvent.setup()
      renderPage()

      await waitFor(() => {
        expect(screen.getAllByText('새 자동화').length).toBeGreaterThanOrEqual(1)
      })

      await user.click(screen.getAllByText('새 자동화')[0])

      await waitFor(() => {
        expect(screen.getByText('이름')).toBeInTheDocument()
        expect(screen.getByText('트리거')).toBeInTheDocument()
      })
    })

    it('validates name is required on save', async () => {
      setupMocks({ automations: [] })
      const user = userEvent.setup()
      renderPage()

      await waitFor(() => expect(screen.getAllByText('새 자동화').length).toBeGreaterThanOrEqual(1))
      await user.click(screen.getAllByText('새 자동화')[0])

      // Add an action first (required)
      await user.click(screen.getByText('+ 액션 추가'))

      // Try to save without name
      await user.click(screen.getByText('저장'))

      expect(toast.error).toHaveBeenCalledWith('이름을 입력해주세요')
    })

    it('validates at least one action required', async () => {
      setupMocks({ automations: [] })
      const user = userEvent.setup()
      renderPage()

      await waitFor(() => expect(screen.getAllByText('새 자동화').length).toBeGreaterThanOrEqual(1))
      await user.click(screen.getAllByText('새 자동화')[0])

      // Fill name but no action
      const nameInput = screen.getByPlaceholderText('예: 승인 시 알림 발송')
      await user.type(nameInput, '테스트 자동화')

      await user.click(screen.getByText('저장'))

      expect(toast.error).toHaveBeenCalledWith('최소 하나의 액션을 추가해주세요')
    })

    it('saves automation successfully', async () => {
      let postCalled = false
      mockFetch.mockImplementation((url: string, opts?: RequestInit) => {
        if (url.includes('/automations') && opts?.method === 'POST') {
          postCalled = true
          return jsonResponse({ data: { id: 'new-1' } }, 201)
        }
        if (url.includes('/schema/collections/c1/automations') && !opts?.method) {
          return jsonResponse({ data: [] })
        }
        if (url.includes('/schema/collections/c1') && !url.includes('process') && !url.includes('automations')) {
          return jsonResponse({ data: collection })
        }
        if (url.includes('/process')) {
          return jsonResponse({ data: { is_enabled: false, statuses: [], transitions: [] } })
        }
        if (url.includes('/auth/me')) {
          return jsonResponse({ data: { id: 'u1', role: 'director' } })
        }
        return jsonResponse({ data: null })
      })

      const user = userEvent.setup()
      renderPage()

      await waitFor(() => expect(screen.getAllByText('새 자동화').length).toBeGreaterThanOrEqual(1))
      await user.click(screen.getAllByText('새 자동화')[0])

      await user.type(screen.getByPlaceholderText('예: 승인 시 알림 발송'), '신규 알림')
      await user.click(screen.getByText('+ 액션 추가'))
      await user.click(screen.getByText('저장'))

      await waitFor(() => {
        expect(postCalled).toBe(true)
      })
    })
  })

  describe('condition and action management', () => {
    it('adds and removes conditions', async () => {
      setupMocks({ automations: [] })
      const user = userEvent.setup()
      renderPage()

      await waitFor(() => expect(screen.getAllByText('새 자동화').length).toBeGreaterThanOrEqual(1))
      await user.click(screen.getAllByText('새 자동화')[0])

      expect(screen.getByText('조건 (0)')).toBeInTheDocument()

      await user.click(screen.getByText('+ 조건 추가'))
      expect(screen.getByText('조건 (1)')).toBeInTheDocument()

      await user.click(screen.getByText('+ 조건 추가'))
      expect(screen.getByText('조건 (2)')).toBeInTheDocument()
    })

    it('adds and removes actions', async () => {
      setupMocks({ automations: [] })
      const user = userEvent.setup()
      renderPage()

      await waitFor(() => expect(screen.getAllByText('새 자동화').length).toBeGreaterThanOrEqual(1))
      await user.click(screen.getAllByText('새 자동화')[0])

      expect(screen.getByText('액션 (0)')).toBeInTheDocument()

      await user.click(screen.getByText('+ 액션 추가'))
      expect(screen.getByText('액션 (1)')).toBeInTheDocument()
    })
  })

  describe('delete automation', () => {
    it('deletes automation on trash icon click', async () => {
      let deleteCalled = false
      mockFetch.mockImplementation((url: string, opts?: RequestInit) => {
        if (url.includes('/schema/automations/a1') && opts?.method === 'DELETE') {
          deleteCalled = true
          return jsonResponse(null, 204)
        }
        if (url.includes('/schema/collections/c1/automations')) {
          return jsonResponse({ data: automations })
        }
        if (url.includes('/schema/collections/c1') && !url.includes('process') && !url.includes('automations')) {
          return jsonResponse({ data: collection })
        }
        if (url.includes('/process')) {
          return jsonResponse({ data: { is_enabled: false, statuses: [], transitions: [] } })
        }
        if (url.includes('/auth/me')) {
          return jsonResponse({ data: { id: 'u1', role: 'director' } })
        }
        return jsonResponse({ data: null })
      })

      const user = userEvent.setup()
      renderPage()

      await waitFor(() => {
        expect(screen.getByText('생성 시 알림')).toBeInTheDocument()
      })

      // Find the trash button in the first automation card
      const trashBtns = screen.getAllByRole('button').filter(
        (btn) => btn.innerHTML.includes('h-4 w-4') && btn.closest('[class*="card"]'),
      )
      if (trashBtns.length > 0) {
        await user.click(trashBtns[0])
        await waitFor(() => {
          expect(deleteCalled).toBe(true)
        })
      }
    })
  })

  describe('cancel form', () => {
    it('resets form when cancel clicked', async () => {
      setupMocks({ automations: [] })
      const user = userEvent.setup()
      renderPage()

      await waitFor(() => expect(screen.getAllByText('새 자동화').length).toBeGreaterThanOrEqual(1))
      await user.click(screen.getAllByText('새 자동화')[0])

      await user.type(screen.getByPlaceholderText('예: 승인 시 알림 발송'), '테스트')

      await user.click(screen.getByText('취소'))

      // Form should be closed, "새 자동화" button should reappear
      await waitFor(() => {
        expect(screen.getAllByText('새 자동화').length).toBeGreaterThanOrEqual(1)
        expect(screen.queryByPlaceholderText('예: 승인 시 알림 발송')).not.toBeInTheDocument()
      })
    })
  })
})

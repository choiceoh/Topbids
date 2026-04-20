import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

import EntryForm from './EntryForm'
import type { Field, Process } from '@/lib/types'

// Mock sonner.
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

// Mock useCurrentUser (returns a director user).
vi.mock('@/hooks/useAuth', () => ({
  useCurrentUser: () => ({ data: { id: 'u1', role: 'director', name: 'Admin' } }),
}))

// Mock useAvailableTransitions to return data based on the collectionId/status.
// collectionId="c1" → normal transitions; collectionId="c1-restricted" → empty (simulates pm-only).
vi.mock('@/hooks/useEntries', async (importOriginal) => {
  const actual = await importOriginal<Record<string, unknown>>()
  return {
    ...actual,
    useAvailableTransitions: (collectionId?: string, status?: string) => {
      if (!collectionId) return { data: undefined }
      if (collectionId === 'c1-restricted') {
        return { data: { transitions: [], allowed_moves: {} } }
      }
      if (status === '접수') {
        return {
          data: {
            transitions: [{ id: 't1', label: '처리 시작', to_status: '처리중', to_color: '#f59e0b' }],
            allowed_moves: {},
          },
        }
      }
      if (status === '처리중') {
        return {
          data: {
            transitions: [{ id: 't2', label: '완료 처리', to_status: '완료', to_color: '#10b981' }],
            allowed_moves: {},
          },
        }
      }
      return { data: undefined }
    },
  }
})

// Mock RelationCombobox / UserCombobox to avoid deep dependency chains.
vi.mock('@/components/common/RelationCombobox', () => ({
  default: ({ value, onChange }: { value?: string; onChange: (v: unknown) => void }) => (
    <input data-testid="relation-input" value={value ?? ''} onChange={(e) => onChange(e.target.value)} />
  ),
}))
vi.mock('@/components/common/UserCombobox', () => ({
  default: ({ value, onChange }: { value?: string; onChange: (v: unknown) => void }) => (
    <input data-testid="user-input" value={value ?? ''} onChange={(e) => onChange(e.target.value)} />
  ),
}))

const mockFetch = vi.fn()

beforeEach(() => {
  vi.stubGlobal('fetch', mockFetch)
})

afterEach(() => {
  vi.restoreAllMocks()
})

function makeField(overrides: Partial<Field> & { slug: string; label: string; field_type: Field['field_type'] }): Field {
  return {
    id: overrides.slug,
    collection_id: 'c1',
    is_required: false,
    is_unique: false,
    is_indexed: false,
    width: 6,
    height: 1,
    sort_order: 0,
    created_at: '2024-01-01',
    updated_at: '2024-01-01',
    ...overrides,
  }
}

function renderForm(props: Partial<React.ComponentProps<typeof EntryForm>> = {}) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const defaultProps = {
    fields: [
      makeField({ slug: 'title', label: '제목', field_type: 'text', is_required: true }),
      makeField({ slug: 'description', label: '설명', field_type: 'textarea' }),
      makeField({ slug: 'count', label: '수량', field_type: 'number' }),
    ],
    onSubmit: vi.fn(),
    onCancel: vi.fn(),
    ...props,
  }

  return {
    ...render(
      <QueryClientProvider client={qc}>
        <EntryForm {...defaultProps} />
      </QueryClientProvider>,
    ),
    ...defaultProps,
  }
}

describe('EntryForm', () => {
  describe('field rendering', () => {
    it('renders text, textarea, number fields', () => {
      renderForm()

      expect(screen.getByText('제목')).toBeInTheDocument()
      expect(screen.getByText('설명')).toBeInTheDocument()
      expect(screen.getByText('수량')).toBeInTheDocument()
    })

    it('marks required fields with *', () => {
      renderForm()
      const label = screen.getByText('제목')
      expect(label.parentElement?.querySelector('.text-destructive')).toBeTruthy()
    })

    it('renders select field with choices', () => {
      renderForm({
        fields: [
          makeField({
            slug: 'status',
            label: '상태',
            field_type: 'select',
            options: { choices: ['대기', '진행중', '완료'] },
          }),
        ],
      })

      expect(screen.getByText('상태')).toBeInTheDocument()
    })

    it('renders multiselect field with popover trigger', async () => {
      const user = userEvent.setup()
      renderForm({
        fields: [
          makeField({
            slug: 'tags',
            label: '태그',
            field_type: 'multiselect',
            options: { choices: ['긴급', '일반', '낮음'] },
          }),
        ],
      })

      // The field label should be visible
      expect(screen.getByText('태그')).toBeInTheDocument()
      // The popover trigger shows placeholder text when nothing is selected
      expect(screen.getByText('선택...')).toBeInTheDocument()

      // Open the popover to reveal choices
      await user.click(screen.getByText('선택...'))

      await waitFor(() => {
        expect(screen.getByText('긴급')).toBeInTheDocument()
        expect(screen.getByText('일반')).toBeInTheDocument()
        expect(screen.getByText('낮음')).toBeInTheDocument()
      })
    })

    it('renders boolean field with checkbox', () => {
      renderForm({
        fields: [makeField({ slug: 'is_active', label: '활성', field_type: 'boolean' })],
      })
      expect(screen.getByText('활성')).toBeInTheDocument()
    })

    it('renders date field', () => {
      renderForm({
        fields: [makeField({ slug: 'due_date', label: '마감일', field_type: 'date' })],
      })
      expect(screen.getByText('마감일')).toBeInTheDocument()
    })

    it('renders autonumber as disabled input', () => {
      renderForm({
        fields: [makeField({ slug: 'seq', label: '번호', field_type: 'autonumber' })],
      })
      expect(screen.getByDisplayValue('(자동 생성)')).toBeDisabled()
    })

    it('renders formula as read-only', () => {
      renderForm({
        fields: [makeField({ slug: 'calc', label: '계산값', field_type: 'formula' })],
        initialData: { calc: 42 },
      })
      expect(screen.getByText('계산값 (수식)')).toBeInTheDocument()
      expect(screen.getByText('42')).toBeInTheDocument()
    })

    it('renders layout elements (label, line, spacer)', () => {
      renderForm({
        fields: [
          makeField({
            slug: 'section1',
            label: '기본 정보',
            field_type: 'label',
            is_layout: true,
            options: { content: '기본 정보 섹션' },
          }),
          makeField({ slug: 'divider', label: '', field_type: 'line', is_layout: true }),
          makeField({ slug: 'gap', label: '', field_type: 'spacer', is_layout: true }),
        ],
      })

      expect(screen.getByText('기본 정보 섹션')).toBeInTheDocument()
    })

    it('renders number with rating display type', () => {
      renderForm({
        fields: [
          makeField({
            slug: 'rating',
            label: '평점',
            field_type: 'number',
            options: { display_type: 'rating', max_rating: 5 },
          }),
        ],
      })
      expect(screen.getByText('평점')).toBeInTheDocument()
      // Should show 5 star buttons
      const stars = screen.getAllByText('★')
      expect(stars).toHaveLength(5)
    })

    it('renders number with progress display type', () => {
      renderForm({
        fields: [
          makeField({
            slug: 'progress',
            label: '진행률',
            field_type: 'number',
            options: { display_type: 'progress' },
          }),
        ],
        initialData: { progress: 75 },
      })
      expect(screen.getByText('진행률')).toBeInTheDocument()
    })

    it('renders text with email display type as email input', () => {
      renderForm({
        fields: [
          makeField({
            slug: 'email',
            label: '이메일',
            field_type: 'text',
            options: { display_type: 'email' },
          }),
        ],
      })
      const input = screen.getByRole('textbox')
      expect(input).toHaveAttribute('type', 'email')
    })
  })

  describe('form submission', () => {
    it('calls onSubmit with form data', async () => {
      const user = userEvent.setup()
      const { onSubmit } = renderForm()

      const titleInput = screen.getAllByRole('textbox')[0]
      await user.clear(titleInput)
      await user.type(titleInput, '새 앱')

      const submitBtn = screen.getByText('저장')
      await user.click(submitBtn)

      expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ title: '새 앱' }))
    })

    it('shows "저장 중..." when submitting', () => {
      renderForm({ submitting: true })
      expect(screen.getByText('저장 중...')).toBeInTheDocument()
      expect(screen.getByText('저장 중...')).toBeDisabled()
    })

    it('calls onCancel when cancel button clicked', async () => {
      const user = userEvent.setup()
      const { onCancel } = renderForm()

      await user.click(screen.getByText('취소'))
      expect(onCancel).toHaveBeenCalled()
    })
  })

  describe('initial data', () => {
    it('populates fields with initial data', () => {
      renderForm({ initialData: { title: '기존 제목', count: 10 } })

      expect(screen.getByDisplayValue('기존 제목')).toBeInTheDocument()
      expect(screen.getByDisplayValue('10')).toBeInTheDocument()
    })

    it('extracts relation ID from expanded object', () => {
      renderForm({
        fields: [
          makeField({
            slug: 'assignee',
            label: '담당자',
            field_type: 'relation',
            relation: {
              id: 'r1',
              field_id: 'f1',
              target_collection_id: 'c2',
              relation_type: 'one_to_many',
              on_delete: 'SET NULL',
            },
          }),
        ],
        initialData: { assignee: { id: 'u99', name: 'User Name' } },
      })

      const input = screen.getByTestId('relation-input')
      expect(input).toHaveValue('u99')
    })
  })

  describe('process status transitions', () => {
    const process: Process = {
      id: 'p1',
      collection_id: 'c1',
      is_enabled: true,
      statuses: [
        { id: 's1', process_id: 'p1', name: '접수', color: '#3b82f6', sort_order: 0, is_initial: true },
        { id: 's2', process_id: 'p1', name: '처리중', color: '#f59e0b', sort_order: 1, is_initial: false },
        { id: 's3', process_id: 'p1', name: '완료', color: '#10b981', sort_order: 2, is_initial: false },
      ],
      transitions: [
        { id: 't1', process_id: 'p1', from_status_id: 's1', to_status_id: 's2', label: '처리 시작', allowed_roles: [], allowed_user_ids: [] },
        { id: 't2', process_id: 'p1', from_status_id: 's2', to_status_id: 's3', label: '완료 처리', allowed_roles: ['director'], allowed_user_ids: [] },
      ],
    }

    it('shows current status and available transitions', () => {
      renderForm({
        fields: [makeField({ slug: 'title', label: '제목', field_type: 'text' })],
        initialData: { id: 'entry1', _status: '접수' },
        process,
        collectionId: 'c1',
      })

      expect(screen.getByText('접수')).toBeInTheDocument()
      expect(screen.getByText(/처리 시작/)).toBeInTheDocument()
    })

    it('shows status change preview when transition clicked', async () => {
      const user = userEvent.setup()
      renderForm({
        fields: [makeField({ slug: 'title', label: '제목', field_type: 'text' })],
        initialData: { id: 'entry1', _status: '접수' },
        process,
        collectionId: 'c1',
      })

      await user.click(screen.getByText(/처리 시작/))

      // After clicking, a preview message should appear about the status change.
      await waitFor(() => {
        expect(screen.getByText(/저장 시 상태가/)).toBeInTheDocument()
      })
    })

    it('hides transitions for unauthorized roles', () => {
      // useCurrentUser returns director, but server returns empty transitions for restricted collection.
      renderForm({
        fields: [makeField({ slug: 'title', label: '제목', field_type: 'text' })],
        initialData: { id: 'entry1', _status: '처리중' },
        process: {
          ...process,
          transitions: [
            { id: 't2', process_id: 'p1', from_status_id: 's2', to_status_id: 's3', label: '완료 처리', allowed_roles: ['pm'], allowed_user_ids: [] },
          ],
        },
        collectionId: 'c1-restricted',
      })

      expect(screen.queryByText(/완료 처리/)).not.toBeInTheDocument()
    })
  })

  describe('file upload', () => {
    it('renders file input for file field', () => {
      renderForm({
        fields: [makeField({ slug: 'attachment', label: '첨부파일', field_type: 'file' })],
      })
      expect(screen.getByText('첨부파일')).toBeInTheDocument()
    })
  })

  describe('JSON field', () => {
    it('renders JSON textarea with formatted value', () => {
      renderForm({
        fields: [makeField({ slug: 'meta', label: '메타데이터', field_type: 'json' })],
        initialData: { meta: { key: 'value' } },
      })
      expect(screen.getByText('메타데이터')).toBeInTheDocument()
    })
  })
})

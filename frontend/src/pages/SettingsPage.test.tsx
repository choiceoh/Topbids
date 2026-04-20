import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import SettingsPage from './SettingsPage'
import { renderWithProviders, jsonResponse } from '@/test/helpers'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

const mockFetch = vi.fn()

beforeEach(() => {
  vi.stubGlobal('fetch', mockFetch)
})

afterEach(() => {
  vi.restoreAllMocks()
})

const directorUser = {
  id: 'u1', name: 'Admin', email: 'admin@test.com', role: 'director',
  phone: '010-1234-5678', is_active: true,
}

const viewerUser = {
  id: 'u2', name: 'Viewer', email: 'viewer@test.com', role: 'viewer',
  phone: '', is_active: true,
}

function setupMocks(user = directorUser) {
  mockFetch.mockImplementation((url: string) => {
    if (url.includes('/auth/me')) {
      return jsonResponse({ data: user })
    }
    if (url.includes('/webhooks')) {
      return jsonResponse({ data: [], total: 0, page: 1, limit: 20, total_pages: 1 })
    }
    return jsonResponse({ data: null })
  })
}

function renderPage() {
  return renderWithProviders(<SettingsPage />, {
    route: '/settings',
    path: '/settings',
  })
}

describe('SettingsPage', () => {
  it('renders page title and profile tab', async () => {
    setupMocks()
    renderPage()

    expect(screen.getByText('설정')).toBeInTheDocument()
    expect(screen.getByText('프로필')).toBeInTheDocument()
  })

  it('displays user profile info after load', async () => {
    setupMocks()
    renderPage()

    await waitFor(() => {
      expect(screen.getByDisplayValue('admin@test.com')).toBeInTheDocument()
    })

    expect(screen.getByDisplayValue('Admin')).toBeInTheDocument()
    expect(screen.getByDisplayValue('director')).toBeInTheDocument()
  })

  it('shows webhook tab for director', async () => {
    setupMocks(directorUser)
    renderPage()

    await waitFor(() => {
      expect(screen.getByText('웹훅')).toBeInTheDocument()
    })
  })

  it('hides webhook tab for non-director', async () => {
    setupMocks(viewerUser)
    renderPage()

    await waitFor(() => {
      expect(screen.getByDisplayValue('viewer@test.com')).toBeInTheDocument()
    })

    expect(screen.queryByText('웹훅')).not.toBeInTheDocument()
  })

  it('renders password change section', async () => {
    setupMocks()
    renderPage()

    await waitFor(() => {
      // "비밀번호 변경" appears as both heading and button — check for at least 1.
      expect(screen.getAllByText('비밀번호 변경').length).toBeGreaterThanOrEqual(1)
    })

    expect(screen.getByLabelText('현재 비밀번호')).toBeInTheDocument()
    expect(screen.getByLabelText('새 비밀번호')).toBeInTheDocument()
    expect(screen.getByLabelText('새 비밀번호 확인')).toBeInTheDocument()
  })

  it('disables save button when passwords do not match', async () => {
    setupMocks()
    const user = userEvent.setup()
    renderPage()

    await waitFor(() => {
      expect(screen.getByLabelText('현재 비밀번호')).toBeInTheDocument()
    })

    await user.type(screen.getByLabelText('현재 비밀번호'), 'oldpass')
    await user.type(screen.getByLabelText('새 비밀번호'), 'newpass123')
    await user.type(screen.getByLabelText('새 비밀번호 확인'), 'different')

    expect(screen.getByText('비밀번호가 일치하지 않습니다')).toBeInTheDocument()

    const changePwBtn = screen.getByRole('button', { name: '비밀번호 변경' })
    expect(changePwBtn).toBeDisabled()
  })

  it('email and role fields are disabled', async () => {
    setupMocks()
    renderPage()

    await waitFor(() => {
      expect(screen.getByDisplayValue('admin@test.com')).toBeInTheDocument()
    })

    expect(screen.getByDisplayValue('admin@test.com')).toBeDisabled()
    expect(screen.getByDisplayValue('director')).toBeDisabled()
  })

  it('can edit name field', async () => {
    setupMocks()
    const user = userEvent.setup()
    renderPage()

    await waitFor(() => {
      expect(screen.getByDisplayValue('Admin')).toBeInTheDocument()
    })

    const nameInput = screen.getByDisplayValue('Admin')
    await user.clear(nameInput)
    await user.type(nameInput, 'New Name')

    expect(screen.getByDisplayValue('New Name')).toBeInTheDocument()
  })
})

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import LoginPage from './LoginPage'
import { renderWithProviders, jsonResponse } from '@/test/helpers'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

// Mock useLogin — we need to mock the hook since it calls useNavigate.
const mockMutate = vi.fn()
vi.mock('@/hooks/useAuth', () => ({
  useLogin: () => ({
    mutate: mockMutate,
    isPending: false,
  }),
}))

const mockFetch = vi.fn()

beforeEach(() => {
  vi.stubGlobal('fetch', mockFetch)
  mockMutate.mockReset()
  mockFetch.mockImplementation(() => jsonResponse({ data: null }))
})

afterEach(() => {
  vi.restoreAllMocks()
})

function getEmailInput() {
  return document.querySelector('input[type="email"]') as HTMLInputElement
}

function getPasswordInput() {
  return document.querySelector('input[type="password"]') as HTMLInputElement
}

describe('LoginPage', () => {
  it('renders login form with title and inputs', () => {
    renderWithProviders(<LoginPage />)

    expect(screen.getByText('Topworks')).toBeInTheDocument()
    expect(screen.getByText('이메일')).toBeInTheDocument()
    expect(screen.getByText('비밀번호')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '로그인' })).toBeInTheDocument()
    expect(getEmailInput()).toBeInTheDocument()
    expect(getPasswordInput()).toBeInTheDocument()
  })

  it('submits form with email and password', async () => {
    const user = userEvent.setup()
    renderWithProviders(<LoginPage />)

    await user.type(getEmailInput(), 'test@example.com')
    await user.type(getPasswordInput(), 'password123')
    await user.click(screen.getByRole('button', { name: '로그인' }))

    await waitFor(() => {
      expect(mockMutate).toHaveBeenCalledWith(
        { email: 'test@example.com', password: 'password123' },
        expect.objectContaining({ onError: expect.any(Function) }),
      )
    })
  })

  it('does not submit with invalid email', async () => {
    const user = userEvent.setup()
    renderWithProviders(<LoginPage />)

    await user.type(getEmailInput(), 'invalid')
    await user.type(getPasswordInput(), 'password')
    await user.click(screen.getByRole('button', { name: '로그인' }))

    // Zod rejects the email — mutate should not be called.
    // Wait a tick to ensure form processing completes.
    await new Promise((r) => setTimeout(r, 50))
    expect(mockMutate).not.toHaveBeenCalled()
  })

  it('shows validation error for empty password', async () => {
    const user = userEvent.setup()
    renderWithProviders(<LoginPage />)

    await user.type(getEmailInput(), 'test@example.com')
    await user.click(screen.getByRole('button', { name: '로그인' }))

    await waitFor(() => {
      expect(screen.getByText('비밀번호를 입력하세요')).toBeInTheDocument()
    })
    expect(mockMutate).not.toHaveBeenCalled()
  })

  it('has correct input types', () => {
    renderWithProviders(<LoginPage />)

    expect(getEmailInput()).toHaveAttribute('type', 'email')
    expect(getPasswordInput()).toHaveAttribute('type', 'password')
  })
})

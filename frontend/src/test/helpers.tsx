import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { render, type RenderOptions } from '@testing-library/react'

export function createTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  })
}

interface WrapperOptions {
  route?: string
  path?: string
}

export function renderWithProviders(
  ui: ReactNode,
  { route = '/', path = '/', ...renderOptions }: WrapperOptions & Omit<RenderOptions, 'wrapper'> = {},
) {
  const queryClient = createTestQueryClient()

  const router = createMemoryRouter(
    [{ path, element: ui }],
    { initialEntries: [route] },
  )

  return {
    ...render(
      <QueryClientProvider client={queryClient}>
        <RouterProvider router={router} />
      </QueryClientProvider>,
      renderOptions,
    ),
    queryClient,
  }
}

// Mock fetch helper reused across tests.
export function jsonResponse(body: unknown, status = 200) {
  return Promise.resolve({
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? 'OK' : 'Error',
    headers: { get: () => null },
    json: () => Promise.resolve(body),
  })
}

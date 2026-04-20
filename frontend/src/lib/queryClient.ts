import { QueryClient } from '@tanstack/react-query'

import { ApiError } from './api'

/**
 * Singleton React Query client for the entire application.
 *
 * Configuration rationale (tuned for an internal admin tool with ~300 users):
 *
 * **Queries:**
 * - `staleTime: 30s` — schema and list reads can tolerate slight staleness;
 *   avoids redundant fetches during rapid navigation.
 * - `gcTime: 5min` — keeps recently visited collection data warm so
 *   back-navigation feels instant.
 * - `refetchOnWindowFocus: false` — users often switch between tabs mid-flow
 *   (e.g. editing a form); silent refetches would disrupt unsaved state.
 * - `retry` — skips 4xx errors (validation, auth, not-found will not self-fix);
 *   retries up to 2x for 5xx (transient server errors).
 *
 * **Mutations:**
 * - `retry: false` — never auto-retry writes; duplicate creates or updates
 *   would cause data corruption or double side-effects.
 */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      gcTime: 5 * 60_000,
      refetchOnWindowFocus: false,
      retry: (failureCount, error) => {
        if (error instanceof ApiError && error.status >= 400 && error.status < 500) {
          return false
        }
        return failureCount < 2
      },
    },
    mutations: {
      retry: false,
    },
  },
})

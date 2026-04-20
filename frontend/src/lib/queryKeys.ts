/**
 * Centralized React Query key factory (TkDodo pattern).
 *
 * Keys form a hierarchy: invalidating a parent key cascades to all children.
 * Example: invalidating queryKeys.entries.all invalidates ALL entry queries
 * across all collections.
 *
 * Convention:
 * - .all   -> base key for broad invalidation
 * - .list(params) -> parameterized list query
 * - .detail(id)   -> single entity query
 */

export const queryKeys = {
  /** Auth keys: invalidate `auth.all` to refetch both current user and user list. */
  auth: {
    all: ['auth'] as const,
    /** Current logged-in user profile. */
    me: () => [...queryKeys.auth.all, 'me'] as const,
    /** All platform users (admin user management). */
    users: () => [...queryKeys.auth.all, 'users'] as const,
  },

  /** Subsidiary (legal entity) keys. */
  subsidiaries: {
    all: ['subsidiaries'] as const,
    list: () => [...queryKeys.subsidiaries.all, 'list'] as const,
    detail: (id: string) => [...queryKeys.subsidiaries.all, 'detail', id] as const,
  },

  /** Department keys. */
  departments: {
    all: ['departments'] as const,
    list: () => [...queryKeys.departments.all, 'list'] as const,
    detail: (id: string) => [...queryKeys.departments.all, 'detail', id] as const,
  },

  /** Collection (app) keys. Invalidate `.all` after create/delete. */
  collections: {
    all: ['collections'] as const,
    list: () => [...queryKeys.collections.all, 'list'] as const,
    /** Single collection by ID (includes fields). */
    detail: (id: string) => [...queryKeys.collections.all, 'detail', id] as const,
    /** Single collection by slug (URL-based lookup). */
    bySlug: (slug: string) => [...queryKeys.collections.all, 'bySlug', slug] as const,
  },

  /**
   * Entry (row) keys, scoped per collection slug.
   * Invalidate `.collection(slug)` to refetch all queries for one collection.
   * Invalidate `.all` to refetch entries across ALL collections.
   */
  entries: {
    all: ['entries'] as const,
    /** All entry queries for a single collection. */
    collection: (slug: string) => [...queryKeys.entries.all, slug] as const,
    /** Paginated/filtered entry list for a collection. */
    list: (slug: string, query?: Record<string, unknown>) =>
      [...queryKeys.entries.all, slug, 'list', query ?? {}] as const,
    /** Single entry by collection slug and record ID. */
    detail: (slug: string, id: string) =>
      [...queryKeys.entries.all, slug, 'detail', id] as const,
  },

  /** Process (workflow) keys, scoped by collection ID. */
  process: {
    all: ['process'] as const,
    detail: (collectionId: string) =>
      [...queryKeys.process.all, collectionId] as const,
  },

  /** Comment keys, scoped by collection slug + record ID. */
  comments: {
    all: ['comments'] as const,
    list: (slug: string, recordId: string) =>
      [...queryKeys.comments.all, slug, recordId] as const,
  },

  /** Notification keys. Invalidate `.all` after marking read. */
  notifications: {
    all: ['notifications'] as const,
    list: () => [...queryKeys.notifications.all, 'list'] as const,
    unreadCount: () => [...queryKeys.notifications.all, 'unread'] as const,
  },

  /** Collection member keys, scoped by collection ID. */
  members: {
    all: ['members'] as const,
    list: (collectionId: string) => [...queryKeys.members.all, 'list', collectionId] as const,
  },

  /** Audit history keys, scoped by collection slug + record ID. */
  history: {
    all: ['history'] as const,
    record: (slug: string, recordId: string) =>
      [...queryKeys.history.all, slug, recordId] as const,
  },

  /** View layout keys, scoped by collection ID. */
  views: {
    all: ['views'] as const,
    list: (collectionId: string) =>
      [...queryKeys.views.all, collectionId, 'list'] as const,
  },

  /** Saved view (filter+sort preset) keys, scoped by collection ID. */
  savedViews: {
    all: ['savedViews'] as const,
    list: (collectionId: string) =>
      [...queryKeys.savedViews.all, collectionId, 'list'] as const,
  },

  /** DDL migration history keys, optionally scoped by collection ID. */
  migrations: {
    all: ['migrations'] as const,
    history: (collectionId?: string) =>
      [...queryKeys.migrations.all, 'history', collectionId ?? 'all'] as const,
  },

  /** Automation rule keys, scoped by collection ID. */
  automations: {
    all: ['automations'] as const,
    list: (collectionId: string) =>
      [...queryKeys.automations.all, 'list', collectionId] as const,
    detail: (id: string) =>
      [...queryKeys.automations.all, 'detail', id] as const,
    /** Execution history for a single automation. */
    runs: (id: string) =>
      [...queryKeys.automations.all, 'runs', id] as const,
  },

  /** Chart configuration keys, scoped by collection ID. */
  charts: {
    all: ['charts'] as const,
    list: (collectionId: string) =>
      [...queryKeys.charts.all, 'list', collectionId] as const,
  },

  /** Webhook event keys. */
  webhooks: {
    all: ['webhooks'] as const,
    list: (query?: Record<string, unknown>) =>
      [...queryKeys.webhooks.all, 'list', query ?? {}] as const,
    detail: (id: string) =>
      [...queryKeys.webhooks.all, 'detail', id] as const,
  },
} as const

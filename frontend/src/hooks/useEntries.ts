/**
 * CRUD hooks for dynamic table entries (records).
 *
 * This is the most complex hook file — it handles:
 * - Paginated listing with keepPreviousData for smooth pagination
 * - Optimistic updates for all mutations (create/update/delete/bulk)
 * - View-specific server-computed endpoints (calendar, kanban, gantt)
 * - Aggregation queries (totals, group-by)
 * - Duplicate detection (similar records)
 *
 * Optimistic update pattern:
 * 1. onMutate: cancel in-flight queries, snapshot cache, apply update
 * 2. onError: revert to snapshot
 * 3. onSettled: invalidate to refetch canonical data
 */

import { useMutation, useQuery, useQueryClient, keepPreviousData } from '@tanstack/react-query'

import { api } from '@/lib/api'
import { queryKeys } from '@/lib/queryKeys'
import type { AggregateResult, EntryRow, TotalsResult } from '@/lib/types'

// QueryParams describes the URL query string options accepted by the
// dynamic List endpoint. Kept generic so callers can build any combination.
export interface EntryListParams {
  page?: number
  limit?: number
  sort?: string // e.g. "-created_at" or "name,-due_date"
  expand?: string // comma-separated relation field slugs
  filters?: Record<string, string> // { status: "eq:active", capacity: "gte:100" }
  /** JSON-serialized FilterGroup for AND/OR filter groups */
  _filter?: string
}

export interface EntryListResult {
  data: EntryRow[]
  total: number
  page: number
  limit: number
  total_pages: number
}

function buildQueryString(params: EntryListParams): string {
  const search = new URLSearchParams()
  if (params.page) search.set('page', String(params.page))
  if (params.limit) search.set('limit', String(params.limit))
  if (params.sort) search.set('sort', params.sort)
  if (params.expand) search.set('expand', params.expand)
  if (params._filter) {
    search.set('_filter', params._filter)
  } else if (params.filters) {
    for (const [key, value] of Object.entries(params.filters)) {
      if (value) search.set(key, value)
    }
  }
  const qs = search.toString()
  return qs ? `?${qs}` : ''
}

/**
 * Fetch a paginated list of entries for a collection.
 *
 * Uses `keepPreviousData` so the table retains its content while
 * navigating between pages or changing filters (no empty flash).
 * The query key includes the full params object, so each unique
 * page/sort/filter combination is cached independently.
 */
export function useEntries(slug: string | undefined, params: EntryListParams = {}) {
  return useQuery({
    queryKey: queryKeys.entries.list(slug ?? '', params as Record<string, unknown>),
    queryFn: () =>
      api.getList<EntryRow>(`/data/${slug}${buildQueryString(params)}`),
    enabled: !!slug,
    placeholderData: keepPreviousData,
  })
}

/**
 * Fetch a single entry by ID. The optional `expand` parameter is a
 * comma-separated list of relation field slugs to resolve inline,
 * avoiding N+1 fetches for related records.
 */
export function useEntry(slug: string | undefined, id: string | undefined, expand?: string) {
  return useQuery({
    queryKey: queryKeys.entries.detail(slug ?? '', id ?? ''),
    queryFn: () =>
      api.get<EntryRow>(
        `/data/${slug}/${id}${expand ? `?expand=${expand}` : ''}`,
      ),
    enabled: !!slug && !!id,
  })
}

/**
 * Create a new entry with optimistic insert.
 *
 * A temporary entry with a `__temp_` prefixed ID and `_optimistic: true`
 * flag is prepended to all cached list queries for the collection.
 * The flag lets the UI render optimistic rows with a subtle visual
 * distinction (e.g., skeleton shimmer) until the server confirms.
 */
export function useCreateEntry(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      api.post<EntryRow>(`/data/${slug}`, body),
    onMutate: async (body) => {
      const collectionKey = queryKeys.entries.collection(slug)
      await qc.cancelQueries({ queryKey: collectionKey })
      const previousLists = qc.getQueriesData<EntryListResult>({
        queryKey: collectionKey,
      })
      const tempEntry = {
        ...body,
        id: `__temp_${Date.now()}`,
        created_at: new Date().toISOString(),
        _optimistic: true,
      }
      qc.setQueriesData<EntryListResult>(
        { queryKey: collectionKey },
        (old) => {
          if (!old?.data) return old
          return { ...old, data: [tempEntry, ...old.data], total: old.total + 1 }
        },
      )
      return { previousLists }
    },
    onError: (_err, _vars, context) => {
      if (context?.previousLists) {
        for (const [key, data] of context.previousLists) {
          qc.setQueryData(key, data)
        }
      }
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: queryKeys.entries.collection(slug) })
    },
  })
}

/**
 * Update a single entry with optimistic patch.
 *
 * Uses `setQueriesData` (plural) to simultaneously update ALL cached
 * list queries for the collection (every page/filter variant), ensuring
 * the change is visible regardless of which list view is mounted.
 * Also updates the detail cache on server success.
 */
export function useUpdateEntry(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: string; body: Record<string, unknown> }) =>
      api.patch<EntryRow>(`/data/${slug}/${id}`, body),
    onMutate: async ({ id, body }) => {
      const collectionKey = queryKeys.entries.collection(slug)
      await qc.cancelQueries({ queryKey: collectionKey })
      const previousLists = qc.getQueriesData<EntryListResult>({
        queryKey: collectionKey,
      })
      qc.setQueriesData<EntryListResult>(
        { queryKey: collectionKey },
        (old) => {
          if (!old?.data) return old
          return {
            ...old,
            data: old.data.map((row) =>
              String(row.id) === id ? { ...row, ...body } : row,
            ),
          }
        },
      )
      return { previousLists }
    },
    onSuccess: (updated, { id }) => {
      qc.setQueryData(queryKeys.entries.detail(slug, id), updated)
    },
    onError: (_err, _vars, context) => {
      if (context?.previousLists) {
        for (const [key, data] of context.previousLists) {
          qc.setQueryData(key, data)
        }
      }
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: queryKeys.entries.collection(slug) })
    },
  })
}

/**
 * Batch-update multiple entries in a single request.
 *
 * Builds a `Map<id, fields>` for O(1) lookup during the optimistic
 * cache update, then patches all matching rows across every cached
 * list query for the collection.
 */
export function useBatchUpdateEntry(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (updates: { id: string; fields: Record<string, unknown>; _version?: number }[]) =>
      api.patch<EntryRow[]>(`/data/${slug}/batch`, { updates }),
    onMutate: async (updates) => {
      const collectionKey = queryKeys.entries.collection(slug)
      await qc.cancelQueries({ queryKey: collectionKey })
      const previousLists = qc.getQueriesData<EntryListResult>({
        queryKey: collectionKey,
      })
      const updateMap = new Map(updates.map((u) => [u.id, u.fields]))
      qc.setQueriesData<EntryListResult>(
        { queryKey: collectionKey },
        (old) => {
          if (!old?.data) return old
          return {
            ...old,
            data: old.data.map((row) => {
              const fields = updateMap.get(String(row.id))
              return fields ? { ...row, ...fields } : row
            }),
          }
        },
      )
      return { previousLists }
    },
    onError: (_err, _vars, context) => {
      if (context?.previousLists) {
        for (const [key, data] of context.previousLists) {
          qc.setQueryData(key, data)
        }
      }
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: queryKeys.entries.collection(slug) })
    },
  })
}

/** Delete a single entry with optimistic removal from all cached lists. */
export function useDeleteEntry(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.del<{ status: string }>(`/data/${slug}/${id}`),
    onMutate: async (id) => {
      const collectionKey = queryKeys.entries.collection(slug)
      await qc.cancelQueries({ queryKey: collectionKey })
      const previousLists = qc.getQueriesData<EntryListResult>({
        queryKey: collectionKey,
      })
      qc.setQueriesData<EntryListResult>(
        { queryKey: collectionKey },
        (old) => {
          if (!old?.data) return old
          return {
            ...old,
            data: old.data.filter((row) => String(row.id) !== id),
            total: Math.max(0, old.total - 1),
          }
        },
      )
      return { previousLists }
    },
    onError: (_err, _vars, context) => {
      if (context?.previousLists) {
        for (const [key, data] of context.previousLists) {
          qc.setQueryData(key, data)
        }
      }
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: queryKeys.entries.collection(slug) })
    },
  })
}

/**
 * Bulk-delete multiple entries. Converts the ID array to a `Set` for
 * O(1) membership testing during the optimistic filter pass, which
 * matters when deleting from large cached lists.
 */
export function useBulkDeleteEntries(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (ids: string[]) => api.del<{ deleted: number }>(`/data/${slug}/bulk`, { ids }),
    onMutate: async (ids) => {
      const collectionKey = queryKeys.entries.collection(slug)
      await qc.cancelQueries({ queryKey: collectionKey })
      const previousLists = qc.getQueriesData<EntryListResult>({
        queryKey: collectionKey,
      })
      const idSet = new Set(ids)
      qc.setQueriesData<EntryListResult>(
        { queryKey: collectionKey },
        (old) => {
          if (!old?.data) return old
          return {
            ...old,
            data: old.data.filter((row) => !idSet.has(String(row.id))),
            total: Math.max(0, old.total - ids.length),
          }
        },
      )
      return { previousLists }
    },
    onError: (_err, _vars, context) => {
      if (context?.previousLists) {
        for (const [key, data] of context.previousLists) {
          qc.setQueryData(key, data)
        }
      }
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: queryKeys.entries.collection(slug) })
    },
  })
}

/**
 * Fetch server-side totals (sum/avg/min/max/count) for all numeric fields.
 * Accepts the same filter params as the list endpoint so totals reflect
 * the currently filtered view. Stale after 30s.
 */
export function useTotals(slug: string | undefined, filters?: Record<string, string>) {
  const search = new URLSearchParams()
  if (filters) {
    for (const [key, value] of Object.entries(filters)) {
      if (value) search.set(key, value)
    }
  }
  const qs = search.toString()

  return useQuery({
    queryKey: [...queryKeys.entries.all, slug, 'totals', filters],
    queryFn: () =>
      api.get<TotalsResult>(`/data/${slug}/totals${qs ? `?${qs}` : ''}`),
    enabled: !!slug,
    staleTime: 30_000,
  })
}

/**
 * Fetch grouped aggregate data (count/sum/avg/min/max) for a collection.
 * Params: `group` (field to group by), `fn` (aggregate function),
 * `field` (numeric field to aggregate). Used by chart panels and dashboards.
 */
export function useAggregate(
  slug: string | undefined,
  params: { group: string; fn?: string; field?: string },
) {
  const search = new URLSearchParams()
  search.set('group', params.group)
  if (params.fn) search.set('fn', params.fn)
  if (params.field) search.set('field', params.field)

  return useQuery({
    queryKey: [...queryKeys.entries.all, slug, 'aggregate', params],
    queryFn: () =>
      api.get<AggregateResult[]>(`/data/${slug}/aggregate?${search.toString()}`),
    enabled: !!slug && !!params.group,
  })
}

/** Fetch the total entry count for a single collection (lightweight, limit=1 request). */
export function useCollectionCount(slug: string | undefined) {
  return useQuery({
    queryKey: [...queryKeys.entries.all, slug, 'count'],
    queryFn: async () => {
      const res = await api.getList<EntryRow>(`/data/${slug}?limit=1`)
      return res.total
    },
    enabled: !!slug,
    staleTime: 60_000,
  })
}

/** Fetch server-computed default values for new entries (e.g., auto-increment, current user). */
export function useEntryDefaults(slug: string | undefined) {
  return useQuery({
    queryKey: [...queryKeys.entries.all, slug, 'defaults'],
    queryFn: () => api.get<Record<string, unknown>>(`/data/${slug}/defaults`),
    enabled: !!slug,
    staleTime: 120_000,
  })
}

export interface SimilarRecord {
  id: string
  value: string
  created_at: string
}

/**
 * Search for similar/duplicate records based on a text query.
 * Only enabled when the query is at least 2 characters. Used by the
 * duplicate-detection UI during entry creation.
 */
export function useSimilarRecords(slug: string | undefined, query: string, field?: string) {
  return useQuery({
    queryKey: [...queryKeys.entries.all, slug, 'similar', query, field],
    queryFn: () => {
      const params = new URLSearchParams({ q: query })
      if (field) params.set('field', field)
      return api.get<SimilarRecord[]>(`/data/${slug}/similar?${params}`)
    },
    enabled: !!slug && query.length >= 2,
    staleTime: 30_000,
  })
}

// ---------------------------------------------------------------------------
// View-specific endpoints (server-computed)
// ---------------------------------------------------------------------------

export interface CalendarSpan {
  entry: EntryRow
  label: string
  startCol: number
  colSpan: number
  track: number
  isStart: boolean
  isEnd: boolean
}

export interface CalendarWeek {
  start: string
  end: string
  days: string[]
  spans: CalendarSpan[]
  singles: Record<string, EntryRow[]>
}

export interface CalendarViewResult {
  year: number
  month: number
  weeks: CalendarWeek[]
}

/**
 * Fetch server-computed calendar layout for a collection.
 * The server returns pre-computed week rows with multi-day span
 * placement (track assignments), avoiding complex client-side layout math.
 */
export function useCalendarView(
  slug: string | undefined,
  params: {
    year: number
    month: number
    dateField: string
    endDateField?: string
    filters?: Record<string, string>
  },
) {
  const search = new URLSearchParams()
  search.set('year', String(params.year))
  search.set('month', String(params.month))
  search.set('date_field', params.dateField)
  if (params.endDateField) search.set('end_date_field', params.endDateField)
  if (params.filters) {
    for (const [key, value] of Object.entries(params.filters)) {
      if (value) search.set(key, value)
    }
  }

  return useQuery({
    queryKey: [...queryKeys.entries.all, slug, 'calendar', params],
    queryFn: () =>
      api.get<CalendarViewResult>(`/data/${slug}/calendar?${search.toString()}`),
    enabled: !!slug && !!params.dateField,
    staleTime: 30_000,
  })
}

export interface GanttRow {
  id: string
  title: string
  startDate: string
  endDate: string
  progress: number | null
  colorKey: string
  dependencies: string[]
  user?: string
  status?: string
}

export interface GanttMonth {
  label: string
  startIndex: number
  span: number
}

export interface GanttViewResult {
  rows: GanttRow[]
  range: { start: string; end: string; totalDays: number }
  months: GanttMonth[]
}

/**
 * Fetch server-computed Gantt chart data. Returns rows with start/end
 * dates, progress percentages, and dependency links, plus a global
 * date range and month grid for the timeline header.
 */
export function useGanttView(
  slug: string | undefined,
  params: {
    startField: string
    endField?: string
    filters?: Record<string, string>
  },
) {
  const search = new URLSearchParams()
  search.set('start_field', params.startField)
  if (params.endField) search.set('end_field', params.endField)
  if (params.filters) {
    for (const [key, value] of Object.entries(params.filters)) {
      if (value) search.set(key, value)
    }
  }

  return useQuery({
    queryKey: [...queryKeys.entries.all, slug, 'gantt', params],
    queryFn: () =>
      api.get<GanttViewResult>(`/data/${slug}/gantt?${search.toString()}`),
    enabled: !!slug && !!params.startField,
    staleTime: 30_000,
  })
}

export interface KanbanColumn {
  value: string
  label: string
  color?: string
  entries: EntryRow[]
}

export interface KanbanViewResult {
  columns: KanbanColumn[]
  allowed_moves?: Record<string, string[]>
}

/**
 * Fetch server-computed Kanban board data grouped by a status/select field.
 * Returns columns with their entries pre-sorted, plus `allowed_moves` when
 * a process workflow is active (restricts drag-and-drop between columns).
 */
export function useKanbanView(
  slug: string | undefined,
  params: {
    groupField: string
    filters?: Record<string, string>
  },
) {
  const search = new URLSearchParams()
  search.set('group_field', params.groupField)
  if (params.filters) {
    for (const [key, value] of Object.entries(params.filters)) {
      if (value) search.set(key, value)
    }
  }

  return useQuery({
    queryKey: [...queryKeys.entries.all, slug, 'kanban', params],
    queryFn: () =>
      api.get<KanbanViewResult>(`/data/${slug}/kanban?${search.toString()}`),
    enabled: !!slug && !!params.groupField,
    staleTime: 30_000,
  })
}

// ---------------------------------------------------------------------------
// My Tasks (cross-collection, process-based)
// ---------------------------------------------------------------------------

export interface MyTaskItem {
  id: string
  label: string
  status: string
  createdAt: string
  collectionId: string
  collectionLabel: string
  collectionSlug: string
  collectionIcon?: string
}

export function useMyTasks() {
  return useQuery({
    queryKey: ['myTasks'],
    queryFn: () => api.get<MyTaskItem[]>('/my-tasks'),
    staleTime: 30_000,
  })
}

// ---------------------------------------------------------------------------
// Global Calendar (cross-collection)
// ---------------------------------------------------------------------------

export interface GlobalCalendarEvent {
  id: string
  label: string
  date: string
  endDate?: string
  collectionId: string
  collectionLabel: string
  collectionSlug: string
  collectionIcon?: string
}

/** Fetch calendar events across all collections for the global calendar dashboard. */
export function useGlobalCalendarEvents(year: number, month: number) {
  return useQuery({
    queryKey: ['globalCalendar', year, month],
    queryFn: () =>
      api.get<GlobalCalendarEvent[]>(
        `/calendar/events?year=${year}&month=${month}`,
      ),
    staleTime: 30_000,
  })
}

// ---------------------------------------------------------------------------
// Relationship Graph
// ---------------------------------------------------------------------------

export interface GraphNode {
  id: string
  label: string
  icon?: string
  fieldCount: number
}

export interface GraphEdge {
  id: string
  sourceId: string
  targetId: string
  label: string
  relationType: string
}

export interface RelationshipGraphResult {
  nodes: GraphNode[]
  edges: GraphEdge[]
}

/** Fetch the collection relationship graph (nodes + edges) for visualization. */
export function useRelationshipGraphAPI() {
  return useQuery({
    queryKey: ['relationshipGraph'],
    queryFn: () => api.get<RelationshipGraphResult>('/schema/relationship-graph'),
    staleTime: 5 * 60 * 1000,
  })
}

// ---------------------------------------------------------------------------
// Available Transitions
// ---------------------------------------------------------------------------

export interface AvailableTransition {
  id: string
  label: string
  to_status: string
  to_color: string
  allowed_user_names?: string[]
  is_blocked?: boolean
  blocked_reason?: string
}

export interface TransitionsResult {
  transitions: AvailableTransition[]
  allowed_moves: Record<string, string[]>
}

/**
 * Fetch available workflow transitions for an entry's current status.
 * Returns the list of valid next statuses and any user restrictions
 * (allowed_user_names). Used by the status change UI to show only
 * permitted transitions.
 */
export function useAvailableTransitions(
  collectionId: string | undefined,
  status?: string,
) {
  const search = new URLSearchParams()
  if (status) search.set('status', status)
  const qs = search.toString()

  return useQuery({
    queryKey: ['transitions', collectionId, status],
    queryFn: () =>
      api.get<TransitionsResult>(
        `/schema/collections/${collectionId}/process/transitions${qs ? `?${qs}` : ''}`,
      ),
    enabled: !!collectionId,
    staleTime: 30_000,
  })
}

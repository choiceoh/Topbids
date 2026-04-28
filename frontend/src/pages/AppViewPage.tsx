/**
 * AppViewPage — Primary data viewer for a collection (app).
 *
 * This is the most complex page in the application. It manages:
 * - Multi-view rendering: list (default), status-kanban, calendar, form, gantt, chart
 * - Data fetching with pagination, sorting, filtering
 * - Inline cell editing with optimistic updates
 * - Bulk operations (delete, edit selected)
 * - CSV import/export
 * - Process workflow transitions
 * - Real-time updates via SSE
 *
 * State management:
 * - Server state: React Query (useEntries, useCollections, useProcess)
 * - UI state: useState (activeTab, filters, sort, selection, modals)
 * - Cell save feedback: Map<"rowId:colId", "saving"|"saved"> for visual indicators
 */
import type { ColumnDef, SortingState } from '@tanstack/react-table'
import {
  ArrowDownUp,
  Bookmark,
  BookmarkPlus,
  Calendar,
  Download,
  FileText,
  Filter,
  Mail,
  LayoutGrid,
  Loader2,
  Pencil,
  Power,
  PowerOff,
  Search,
  Trash2,
  Upload,
  X,
  Ellipsis,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'
import { toast } from 'sonner'

import ConfirmDialog from '@/components/common/ConfirmDialog'
import { ErrorBoundary } from '@/components/common/ErrorBoundary'
import HotkeyHelpDialog from '@/components/common/HotkeyHelpDialog'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { DataTable } from '@/components/common/DataTable'
import ErrorState from '@/components/common/ErrorState'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import RoleGate from '@/components/common/RoleGate'
import BulkEditPanel from '@/components/works/BulkEditPanel'
import CSVImportPreview from '@/components/works/CSVImportPreview'
import FilterBuilder from '@/components/works/FilterBuilder'
import FilterChips from '@/components/works/FilterChips'
import SortPanel, { type SortItem } from '@/components/works/SortPanel'
import CalendarView from '@/components/works/views/CalendarView'
import FormView from '@/components/works/views/FormView'
import KanbanView from '@/components/works/views/KanbanView'
import ViewGuide from '@/components/works/views/ViewGuide'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useHotkeys } from '@/hooks/useHotkeys'
import { useCollection } from '@/hooks/useCollections'
import {
  useBatchUpdateEntry,
  useBulkDeleteEntries,
  useCreateEntry,
  useEntries,
  useTotals,
  useUpdateEntry,
} from '@/hooks/useEntries'
import { useSavedViews, useCreateSavedView, useDeleteSavedView } from '@/hooks/useSavedViews'
import { canManageCollection, useCurrentUser } from '@/hooks/useAuth'
import { useConflictAwareUpdate } from '@/hooks/useConflictAwareUpdate'
import { useRetryToast } from '@/hooks/useRetryToast'
import { useUndoToast } from '@/hooks/useUndoToast'
import { api, ApiError, formatError } from '@/lib/api'
import { isLayoutType, TERM } from '@/lib/constants'
import { formatCell } from '@/lib/formatCell'
import { highlightText } from '@/lib/highlightText'
import type { FilterCondition, FilterGroup, SavedView } from '@/lib/types'
import { emptyFilterGroup, isFilterGroupEmpty, flattenFilterGroup, serializeFilterGroup } from '@/lib/types'
import { getDisplayType } from '@/lib/fieldGuards'

const DEFAULT_LIMIT = 20

/** Recursively remove a condition by ID from a FilterGroup */
function removeCondFromGroup(group: FilterGroup, condId: string): FilterGroup {
  return {
    ...group,
    conditions: group.conditions.filter((c) => c.id !== condId),
    groups: group.groups.map((sg) => removeCondFromGroup(sg, condId)),
  }
}

export default function AppViewPage() {
  const { appId } = useParams()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const activeTab = searchParams.get('tab') || 'list'
  const setActiveTab = useCallback((tab: string | number) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (String(tab) === 'list') next.delete('tab')
      else next.set('tab', String(tab))
      return next
    }, { replace: true })
  }, [setSearchParams])
  /** Current page number for server-side pagination (1-based). */
  const [page, setPage] = useState(1)
  /** Rows per page; user-selectable via DataTable page size dropdown. */
  const [limit, setLimit] = useState(DEFAULT_LIMIT)
  /** Column-header sorting state consumed by @tanstack/react-table. */
  const [sorting, setSorting] = useState<SortingState>([])
  /** ID of the entry pending deletion (drives the ConfirmDialog). */
  /** Hidden file input ref for CSV import trigger. */
  const fileInputRef = useRef<HTMLInputElement>(null)
  /** Count of rows imported in the last CSV upload (shown in toast). */
  const [importedCount, setImportedCount] = useState(0)
  /** Whether the keyboard shortcut help dialog is open. */
  const [hotkeyHelpOpen, setHotkeyHelpOpen] = useState(false)

  // Filter state — uses FilterGroup for AND/OR support
  const [filterGroup, setFilterGroup] = useState<FilterGroup>(emptyFilterGroup())
  // Derived flat list for backward compatibility
  const filterConditions = flattenFilterGroup(filterGroup)
  const hasActiveFilters = !isFilterGroupEmpty(filterGroup)

  // Sort panel state (separate from column header sorting)
  const [sortItems, setSortItems] = useState<SortItem[]>([])

  // Search state
  const [searchText, setSearchText] = useState('')
  const [searchInputValue, setSearchInputValue] = useState('')
  const searchDebounceRef = useRef<ReturnType<typeof setTimeout>>(null)
  useEffect(() => {
    return () => {
      if (searchDebounceRef.current) clearTimeout(searchDebounceRef.current)
    }
  }, [])

  // Process toggle (frontend-only)
  const [processVisible, setProcessVisible] = useState(true)

  // Saved views state
  const [activeView, setActiveView] = useState<SavedView | null>(null)
  const [savingView, setSavingView] = useState(false)
  const [newViewName, setNewViewName] = useState('')

  const { data: collection, isLoading: colLoading, isError: colError, error: colErr } =
    useCollection(appId)
  // Process/automation features are removed. Stubbed with is_enabled=false so
  // dead-code branches compile without refactor; they will be pruned during the
  // bid-domain UI pass.
  const process: import('@/lib/types').Process = {
    id: '', collection_id: '', is_enabled: false, statuses: [], transitions: [],
  }
  const { data: currentUser } = useCurrentUser()
  const canManage = canManageCollection(currentUser, collection?.created_by)

  // Let the backend auto-expand all relation fields.
  const expand = 'auto'

  const { data: savedViews } = useSavedViews(collection?.id)
  const createSavedView = useCreateSavedView(collection?.id ?? '')
  const deleteSavedView = useDeleteSavedView(collection?.id ?? '')

  // Build sort param — only one source is active at a time.
  const sortParam = useMemo(() => {
    if (sortItems.length > 0) {
      return sortItems.map((s) => `${s.desc ? '-' : ''}${s.field}`).join(',')
    }
    if (sorting.length === 0) return undefined
    return sorting.map((s) => `${s.desc ? '-' : ''}${s.id}`).join(',')
  }, [sorting, sortItems])

  // When column header sort changes, clear sort panel.
  const handleHeaderSortChange = useCallback((next: SortingState) => {
    setSorting(next)
    if (next.length > 0) setSortItems([])
  }, [])

  // When sort panel changes, clear column header sort.
  const handleSortPanelChange = useCallback((items: SortItem[]) => {
    setSortItems(items)
    if (items.length > 0) setSorting([])
    setPage(1)
  }, [])

  // Build filters from FilterGroup + search text.
  // Use JSON _filter param when the group has OR logic or nested groups;
  // fall back to legacy key-value params for simple AND-only flat conditions.
  const filters = useMemo(() => {
    const f: Record<string, string> = {}
    const needsJsonFilter = filterGroup.logic === 'or' || filterGroup.groups.length > 0
    const serialized = serializeFilterGroup(filterGroup)

    if (needsJsonFilter && serialized) {
      f._filter = serialized
    } else {
      for (const cond of filterConditions) {
        if (cond.operator === 'is_null') {
          f[cond.field] = 'is_null:'
        } else if (cond.value) {
          f[cond.field] = `${cond.operator}:${cond.value}`
        }
      }
    }
    if (searchText) {
      f.q = searchText
    }
    return Object.keys(f).length > 0 ? f : undefined
  }, [filterGroup, filterConditions, searchText])

  const {
    data: list,
    isLoading: entriesLoading,
    isError: entriesError,
    error: entriesErr,
    refetch,
  } = useEntries(collection?.slug, {
    page,
    limit,
    sort: sortParam,
    expand,
    filters,
  })

  const createEntry = useCreateEntry(collection?.slug ?? '')
  const updateEntry = useUpdateEntry(collection?.slug ?? '')

  const batchUpdateEntry = useBatchUpdateEntry(collection?.slug ?? '')
  const bulkDelete = useBulkDeleteEntries(collection?.slug ?? '')
  const undoToast = useUndoToast()
  const retryToast = useRetryToast()
  const onConflictError = useConflictAwareUpdate(refetch)

  // Saved view visibility override.
  const [viewVisibility, setViewVisibility] = useState<Record<string, boolean> | null>(null)
  const [viewVisKey, setViewVisKey] = useState(0)

  // Multi-select state for bulk operations.
  const [selectedRowIds, setSelectedRowIds] = useState<Set<string>>(new Set())
  const [selectAllFilteredMode, setSelectAllFilteredMode] = useState(false)
  const [bulkDeleteOpen, setBulkDeleteOpen] = useState(false)
  const [bulkEditOpen, setBulkEditOpen] = useState(false)

  // Select all filtered results across pages.
  const handleSelectAllFiltered = useCallback(async () => {
    if (!collection?.slug || !list?.total) return
    try {
      const params = new URLSearchParams()
      params.set('limit', String(list.total))
      params.set('fields', 'id')
      if (searchText) params.set('q', searchText)
      const res = await fetch(`/api/data/${collection.slug}?${params}`, { credentials: 'include' })
      if (!res.ok) return
      const json = await res.json()
      const allIds = new Set<string>((json.data ?? []).map((r: { id: string }) => r.id))
      setSelectedRowIds(allIds)
      setSelectAllFilteredMode(true)
    } catch { /* ignore */ }
  }, [collection?.slug, list?.total, searchText])

  // Detect views.
  const dateField = useMemo(
    () => collection?.fields?.find((f) => f.field_type === 'date' || f.field_type === 'datetime'),
    [collection],
  )
  // Keyboard shortcuts
  const searchInputRef = useRef<HTMLInputElement>(null)
  useHotkeys([
    { key: '?', handler: () => setHotkeyHelpOpen(true) },
    { key: 'mod+n', handler: () => navigate(`/apps/${appId}/entries/new`) },
    { key: 'mod+f', handler: () => searchInputRef.current?.focus() },
  ])

  // Formula fields are read-only in the grid.
  // Numeric fields for summary row.
  const numericFields = useMemo(
    () =>
      collection?.fields?.filter(
        (f) => f.field_type === 'number' || f.field_type === 'integer',
      ) ?? [],
    [collection],
  )

  // Per-column aggregate function state (default: sum).
  const aggFnStorageKey = appId ? `topbid:aggfn:${appId}` : null
  const [columnAggFn, setColumnAggFn] = useState<Record<string, string>>(() => {
    if (aggFnStorageKey) {
      try {
        const saved = localStorage.getItem(aggFnStorageKey)
        if (saved) return JSON.parse(saved)
      } catch { /* ignore */ }
    }
    return {}
  })

  const handleAggFnChange = useCallback(
    (slug: string, fn: string) => {
      setColumnAggFn((prev) => {
        const next = { ...prev, [slug]: fn }
        if (aggFnStorageKey) {
          try { localStorage.setItem(aggFnStorageKey, JSON.stringify(next)) } catch { /* ignore */ }
        }
        return next
      })
    },
    [aggFnStorageKey],
  )

  // Fetch server-side totals for all numeric fields (single query).
  const totalsSlug = numericFields.length > 0 ? collection?.slug : undefined
  const { data: totals } = useTotals(totalsSlug, filters)

  // Build summary row from server-side totals only.
  const summaryRow = useMemo(() => {
    if (numericFields.length === 0 || !list?.data?.length) return undefined
    const summary: Record<string, { label: string; value: string | number }> = {}
    const fnLabels: Record<string, string> = { sum: '합계', avg: '평균', count: '개수', min: '최소', max: '최대' }

    for (const f of numericFields) {
      const fn = columnAggFn[f.slug] || 'sum'
      const fnLabel = fnLabels[fn] || fn

      const serverField = totals?.[f.slug]
      const serverAgg = typeof serverField === 'object' && serverField !== null
        && 'sum' in serverField && 'avg' in serverField
        ? serverField as { sum: number; avg: number; min: number; max: number }
        : null
      const serverCount = typeof totals?._count === 'number' ? totals._count : undefined

      let displayValue: number | undefined

      switch (fn) {
        case 'sum':
          displayValue = serverAgg?.sum
          break
        case 'avg':
          displayValue = serverAgg?.avg
          break
        case 'count':
          displayValue = serverCount ?? list.total
          break
        case 'min':
          displayValue = serverAgg?.min
          break
        case 'max':
          displayValue = serverAgg?.max
          break
        default:
          displayValue = serverAgg?.sum
      }

      const label = displayValue != null
        ? `${fnLabel} ${displayValue.toLocaleString('ko', { maximumFractionDigits: fn === 'count' ? 0 : 1 })}`
        : `${fnLabel} -`
      summary[f.slug] = { label, value: displayValue ?? '-' }
    }
    return Object.keys(summary).length > 0 ? summary : undefined
  }, [numericFields, list, columnAggFn, totals])

  // Build columns from collection.fields.
  const columns = useMemo<ColumnDef<Record<string, unknown>>[]>(() => {
    if (!collection?.fields) return []
    const cols: ColumnDef<Record<string, unknown>>[] = []

    // Process status column (first if enabled and visible).
    if (processVisible && process?.is_enabled && process.statuses?.length) {
      cols.push({
        id: '_status',
        header: '상태',
        enableSorting: true,
        cell: ({ row }) => {
          const statusName = row.original._status as string
          if (!statusName) return <span className="text-muted-foreground">미설정</span>
          const statusDef = process.statuses.find((s) => s.name === statusName)
          return (
            <Badge
              style={{
                backgroundColor: statusDef?.color ?? '#6b7280',
                color: '#fff',
              }}
            >
              {statusName}
            </Badge>
          )
        },
      })
    }

    cols.push(
      ...collection.fields
        .filter((f) => !isLayoutType(f.field_type))
        .map((f) => ({
          id: f.slug,
          header: f.label,
          enableSorting: true,
          size: f.field_type === 'textarea' ? 250 : 150,
          cell: ({ row }: { row: { original: Record<string, unknown> } }) => {
            const v = row.original[f.slug]
            const dt = getDisplayType(f)

            // Render text display subtypes as clickable links
            if (f.field_type === 'text' && dt && v) {
              const s = String(v)
              if (dt === 'url') return <a href={s.startsWith('http') ? s : `https://${s}`} target="_blank" rel="noopener noreferrer" className="text-primary underline" onClick={(e) => e.stopPropagation()}>{searchText ? highlightText(s, searchText) : s}</a>
              if (dt === 'email') return <a href={`mailto:${s}`} className="text-primary underline" onClick={(e) => e.stopPropagation()}>{searchText ? highlightText(s, searchText) : s}</a>
              if (dt === 'phone') return <a href={`tel:${s}`} className="text-primary underline" onClick={(e) => e.stopPropagation()}>{searchText ? highlightText(s, searchText) : s}</a>
            }

            // Render progress bar inline
            if ((f.field_type === 'number' || f.field_type === 'integer') && dt === 'progress' && v != null) {
              const num = Number(v)
              return (
                <div className="flex items-center gap-2">
                  <div className="h-2 w-16 overflow-hidden rounded-full bg-muted">
                    <div className="h-full rounded-full bg-primary" style={{ width: `${Math.min(100, Math.max(0, num))}%` }} />
                  </div>
                  <span className="text-xs">{num}%</span>
                </div>
              )
            }

            const formatted = formatCell(v, f)
            if (searchText && typeof formatted === 'string' && ['text', 'textarea'].includes(f.field_type)) {
              return highlightText(formatted, searchText)
            }
            return formatted
          },
        })),
    )
    cols.push({
      id: 'created_at',
      header: '작성일',
      enableSorting: true,
      size: 100,
      cell: ({ row }) => {
        const v = row.original.created_at
        if (!v) return '-'
        return new Date(v as string).toLocaleDateString('ko')
      },
    })
    return cols
  }, [collection, process, processVisible, searchText])

  // Default: hide columns beyond the first 8 data fields, restored from localStorage if available.
  const colVisStorageKey = appId ? `topbid:colvis:${appId}` : null
  const initialColumnVisibility = useMemo<Record<string, boolean>>(() => {
    if (colVisStorageKey) {
      try {
        const saved = localStorage.getItem(colVisStorageKey)
        if (saved) return JSON.parse(saved)
      } catch { /* ignore */ }
    }
    if (!collection?.fields) return {}
    const dataFields = collection.fields.filter((f) => !isLayoutType(f.field_type))
    const vis: Record<string, boolean> = {}
    dataFields.forEach((f, i) => {
      if (i >= 8) vis[f.slug] = false
    })
    return vis
  }, [collection, colVisStorageKey])

  const currentVisibilityRef = useRef<Record<string, boolean>>(initialColumnVisibility)
  const handleColumnVisibilityChange = useCallback(
    (visibility: Record<string, boolean>) => {
      currentVisibilityRef.current = visibility
      if (colVisStorageKey) {
        try { localStorage.setItem(colVisStorageKey, JSON.stringify(visibility)) } catch { /* ignore */ }
      }
    },
    [colVisStorageKey],
  )

  // Column pinning persistence (same pattern as visibility).
  const colPinStorageKey = appId ? `topbid:colpin:${appId}` : null
  const initialColumnPinning = useMemo(() => {
    if (colPinStorageKey) {
      try {
        const saved = localStorage.getItem(colPinStorageKey)
        if (saved) return JSON.parse(saved)
      } catch { /* ignore */ }
    }
    return { left: [], right: [] }
  }, [colPinStorageKey])

  const handleColumnPinningChange = useCallback(
    (pinning: { left?: string[]; right?: string[] }) => {
      if (colPinStorageKey) {
        try { localStorage.setItem(colPinStorageKey, JSON.stringify(pinning)) } catch { /* ignore */ }
      }
    },
    [colPinStorageKey],
  )

  // Helper: find _version for a row from the current list data.
  const getRowVersion = useCallback(
    (rowId: string): number | undefined => {
      const row = list?.data?.find((r) => r.id === rowId)
      return row?._version as number | undefined
    },
    [list],
  )


  // Search with debounce.
  function handleSearchInput(value: string) {
    setSearchInputValue(value)
    if (searchDebounceRef.current) clearTimeout(searchDebounceRef.current)
    searchDebounceRef.current = setTimeout(() => {
      setSearchText(value)
      setPage(1)
    }, 300)
  }

  // Export query string builder (shared by CSV/PDF).
  function buildExportQS() {
    const params = new URLSearchParams()
    if (searchText) params.set('q', searchText)
    if (sortParam) params.set('sort', sortParam)
    for (const cond of filterConditions) {
      if (cond.operator === 'is_null') {
        params.set(cond.field, 'is_null:')
      } else if (cond.value) {
        params.set(cond.field, `${cond.operator}:${cond.value}`)
      }
    }
    return params.toString()
  }

  // CSV export.
  function handleCsvExport() {
    if (!collection) return
    const qs = buildExportQS()
    window.open(`/api/data/${collection.slug}/export.csv${qs ? `?${qs}` : ''}`, '_blank')
  }

  // PDF export.
  function handlePdfExport() {
    if (!collection) return
    const qs = buildExportQS()
    window.open(`/api/data/${collection.slug}/export.pdf${qs ? `?${qs}` : ''}`, '_blank')
  }

  // Email report.
  const [emailDialogOpen, setEmailDialogOpen] = useState(false)
  const [emailTo, setEmailTo] = useState('')
  const [emailMessage, setEmailMessage] = useState('')
  const [emailSending, setEmailSending] = useState(false)

  async function handleEmailReport() {
    if (!collection || !emailTo) return
    setEmailSending(true)
    try {
      const qs = buildExportQS()
      await api.post(`/data/${collection.slug}/email-report${qs ? `?${qs}` : ''}`, {
        to: emailTo,
        subject: `[Topworks] ${collection.label} 리포트`,
        message: emailMessage || `${collection.label} 데이터 리포트입니다.`,
      })
      toast.success(`${emailTo}로 리포트가 전송되었습니다`)
      setEmailDialogOpen(false)
      setEmailTo('')
      setEmailMessage('')
    } catch (err) {
      retryToast(err, handleEmailReport)
    } finally {
      setEmailSending(false)
    }
  }

  const [importingCSV, setImportingCSV] = useState(false)
  const [csvPreviewFile, setCsvPreviewFile] = useState<File | null>(null)
  const [csvPreviewOpen, setCsvPreviewOpen] = useState(false)

  const handleImportCSV = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    setCsvPreviewFile(file)
    setCsvPreviewOpen(true)
    e.target.value = ''
  }, [])

  const handleCSVConfirm = useCallback(async (file: File, columnMap: Record<string, string>) => {
    if (!collection) return
    setCsvPreviewOpen(false)
    setImportingCSV(true)
    const toastId = toast.loading('CSV 가져오는 중...')

    try {
      const formData = new FormData()
      formData.append('file', file)
      if (Object.keys(columnMap).length > 0) {
        formData.append('column_map', JSON.stringify(columnMap))
      }
      const res = await fetch(`/api/data/${collection.slug}/import`, {
        method: 'POST',
        body: formData,
        credentials: 'include',
      })
      if (!res.ok) {
        const body = await res.json().catch(() => ({ error: res.statusText }))
        throw new Error(body.error || body.message || res.statusText)
      }
      const result = await res.json()
      const count = result.data?.imported ?? 0
      toast.success(`${count}건 가져왔습니다`, { id: toastId })
      setImportedCount(count)
      await refetch()
      setTimeout(() => setImportedCount(0), 2000)
    } catch (err) {
      toast.error(formatError(err), { id: toastId })
    } finally {
      setImportingCSV(false)
      setCsvPreviewFile(null)
    }
  }, [collection, refetch])

  // Build synthetic field for process status kanban
  const processGroupField = useMemo(() => {
    if (!process?.is_enabled || !process.statuses?.length) return undefined
    return {
      id: '_status',
      collection_id: '',
      slug: '_status',
      label: '상태',
      field_type: 'select' as const,
      is_required: false,
      is_unique: false,
      is_indexed: false,
      width: 6,
      height: 1,
      sort_order: 0,
      created_at: '',
      updated_at: '',
      options: {
        choices: process.statuses.map((s) => s.name),
      },
    }
  }, [process])

  if (colLoading) return <LoadingState variant="table" />
  if (colError) return <ErrorState error={colErr} />
  if (!collection) return null

  const hasCalendar = !!dateField
  const hasProcessKanban = process?.is_enabled && (process.statuses?.length ?? 0) > 0

  function handleEntryClick(entry: Record<string, unknown>) {
    navigate(`/apps/${appId}/entries/${entry.id}`)
  }

  function handleGanttUpdate(entryId: string, updates: Record<string, unknown>) {
    const version = getRowVersion(entryId)
    if (version != null) updates._version = version
    updateEntry.mutate(
      { id: entryId, body: updates },
      {
        onSuccess: () => toast.success('일정이 변경되었습니다'),
        onError: (err) => onConflictError(err),
      },
    )
  }

  function handleFormViewSubmit(data: Record<string, unknown>, entryId?: string) {
    if (entryId) {
      const entry = list?.data.find((e) => String(e.id) === entryId)
      const version = entry?._version as number | undefined
      if (version != null) data._version = version
      updateEntry.mutate(
        { id: entryId, body: data },
        {
          onSuccess: () => toast.success('수정되었습니다'),
          onError: (err) => onConflictError(err),
        },
      )
    } else {
      createEntry.mutate(data, {
        onSuccess: () => toast.success('생성되었습니다'),
        onError: (err) => retryToast(err, () => handleFormViewSubmit(data)),
      })
    }
  }

  function handleProcessCardMove(entryId: string, newValue: string, oldValue: string) {
    const body: Record<string, unknown> = { _status: newValue }
    const version = getRowVersion(entryId)
    if (version != null) body._version = version
    updateEntry.mutate(
      { id: entryId, body },
      {
        onSuccess: () => {
          undoToast.push(
            '상태가 변경되었습니다',
            () => updateEntry.mutate({ id: entryId, body: { _status: oldValue } }),
            () => updateEntry.mutate({ id: entryId, body: { _status: newValue } }),
          )
        },
        onError: (err) => onConflictError(err),
      },
    )
  }



  function handleBulkStatusChange(status: string) {
    const ids = Array.from(selectedRowIds)
    if (ids.length === 0) return
    const updates = ids.map((id) => {
      const version = getRowVersion(id)
      return { id, fields: { _status: status }, _version: version }
    })
    const toastId = toast.loading(`${ids.length}건 상태 변경 중...`)
    batchUpdateEntry.mutate(updates, {
      onSuccess: () => {
        toast.success(`${ids.length}건의 상태가 "${status}"(으)로 변경되었습니다`, { id: toastId })
        setSelectedRowIds(new Set())
      },
      onError: (err) => {
        if (err instanceof ApiError && err.isConflict()) {
          toast.error('일부 항목이 이미 수정되었습니다. 새로고침합니다.', { id: toastId })
          refetch()
        } else {
          toast.error(formatError(err), { id: toastId })
        }
      },
    })
  }

  function handleBulkDelete() {
    const ids = Array.from(selectedRowIds)
    if (ids.length === 0) return
    bulkDelete.mutate(ids, {
      onSuccess: () => {
        toast.success(`${ids.length}건 삭제되었습니다`)
        setSelectedRowIds(new Set())
        setBulkDeleteOpen(false)
      },
      onError: (err) => retryToast(err, () => handleBulkDelete()),
    })
  }

  function handleBulkEdit(fieldSlug: string, value: unknown) {
    const ids = Array.from(selectedRowIds)
    if (ids.length === 0) return
    const updates = ids.map((id) => {
      const version = getRowVersion(id)
      return { id, fields: { [fieldSlug]: value }, _version: version }
    })
    batchUpdateEntry.mutate(updates, {
      onSuccess: () => {
        toast.success(`${ids.length}건 수정되었습니다`)
        setSelectedRowIds(new Set())
        setBulkEditOpen(false)
      },
      onError: (err) => onConflictError(err),
    })
  }

  function applyView(view: SavedView) {
    setActiveView(view)
    if (view.filter_config && Object.keys(view.filter_config).length > 0) {
      const config = view.filter_config as Record<string, unknown>
      // New format: filter_config has a "logic" key
      if ('logic' in config && 'conditions' in config && Array.isArray((config as Record<string, unknown>).conditions)) {
        const fg = config as unknown as FilterGroup
        // Re-assign IDs to prevent collisions
        let idx = 0
        function reId(g: FilterGroup): FilterGroup {
          return {
            ...g,
            id: `sv-g-${idx++}`,
            conditions: g.conditions.map((c) => ({ ...c, id: `sv-${idx++}` })),
            groups: (g.groups ?? []).map(reId),
          }
        }
        setFilterGroup(reId(fg))
      } else {
        // Legacy format: flat key-value map
        const restored: FilterCondition[] = Object.entries(config).map(
          ([key, value], i) => {
            const parts = key.split(':')
            const field = parts[0]
            const operator = parts.slice(1).join(':') || 'eq'
            return { id: `sv-${i}`, field, operator, value: String(value) }
          },
        )
        setFilterGroup({ ...emptyFilterGroup(), conditions: restored })
      }
    } else {
      setFilterGroup(emptyFilterGroup())
    }
    if (view.sort_config) {
      const items: SortItem[] = view.sort_config.split(',').filter(Boolean).map((s) => ({
        field: s.startsWith('-') ? s.slice(1) : s,
        desc: s.startsWith('-'),
      }))
      setSortItems(items)
    } else {
      setSortItems([])
    }
    // Restore visible fields if saved.
    if (view.visible_fields?.length && collection?.fields) {
      const vis: Record<string, boolean> = {}
      collection.fields.filter((f) => !isLayoutType(f.field_type)).forEach((f) => {
        vis[f.slug] = view.visible_fields!.includes(f.slug)
      })
      setViewVisibility(vis)
      setViewVisKey((k) => k + 1)
    }
    setPage(1)
  }

  function clearView() {
    setActiveView(null)
    setFilterGroup(emptyFilterGroup())
    setSortItems([])
    setViewVisibility(null)
    setViewVisKey((k) => k + 1)
    setPage(1)
  }

  function handleSaveView() {
    if (!newViewName.trim()) return
    // Save FilterGroup as the filter_config (supports AND/OR)
    let filterConfig: Record<string, unknown> = {}
    if (!isFilterGroupEmpty(filterGroup)) {
      if (filterGroup.logic === 'or' || filterGroup.groups.length > 0) {
        // Save as new FilterGroup format
        filterConfig = { logic: filterGroup.logic, conditions: filterGroup.conditions.map((c) => ({ field: c.field, operator: c.operator, value: c.value })), groups: filterGroup.groups }
      } else {
        // Save as legacy flat format for backward compat
        for (const c of filterConditions) {
          if (c.field && c.operator) {
            (filterConfig as Record<string, string>)[`${c.field}:${c.operator}`] = c.value
          }
        }
      }
    }
    const sortConfig = sortItems.length > 0
      ? sortItems.map((s) => `${s.desc ? '-' : ''}${s.field}`).join(',')
      : ''

    // Collect visible field slugs from current visibility state.
    const vis = currentVisibilityRef.current
    const visibleFields = collection?.fields
      ?.filter((f) => !isLayoutType(f.field_type))
      .filter((f) => vis[f.slug] !== false)
      .map((f) => f.slug)

    createSavedView.mutate(
      {
        name: newViewName.trim(),
        filter_config: filterConfig,
        sort_config: sortConfig,
        visible_fields: visibleFields,
        is_public: true,
      },
      {
        onSuccess: () => {
          toast.success('보기가 저장되었습니다')
          setNewViewName('')
          setSavingView(false)
        },
        onError: (err) => toast.error(formatError(err)),
      },
    )
  }

  // Toolbar rendered inside DataTable.
  const tableToolbar = (
    <>
    <div className="flex items-center gap-2 flex-wrap w-full">
      {selectedRowIds.size > 0 ? (
        /* ── 일괄 액션 모드 ── */
        <>
          <span className="text-sm font-medium">{selectAllFilteredMode ? `전체 ${selectedRowIds.size}건 선택` : `${selectedRowIds.size}건 선택`}</span>
          {process?.is_enabled && (process.statuses?.length ?? 0) > 0 && (
            <DropdownMenu>
              <DropdownMenuTrigger
                className="inline-flex items-center gap-1 rounded-md border border-input bg-background px-2 py-1 text-sm font-medium hover:bg-accent h-8"
              >
                상태 변경
              </DropdownMenuTrigger>
              <DropdownMenuContent>
                {process.statuses!.map((s) => (
                  <DropdownMenuItem
                    key={s.id}
                    onClick={() => handleBulkStatusChange(s.name)}
                  >
                    <span
                      className="mr-2 inline-block h-2.5 w-2.5 rounded-full"
                      style={{ backgroundColor: s.color }}
                    />
                    {s.name}
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          )}
          <RoleGate roles={['director', 'pm', 'engineer']}>
            <Button
              variant="outline"
              size="sm"
              className="h-8 gap-1"
              onClick={() => setBulkEditOpen(true)}
            >
              <Pencil className="h-3.5 w-3.5" />
              일괄 편집
            </Button>
          </RoleGate>
          {canManage && (
            <Button
              variant="destructive"
              size="sm"
              className="h-8 gap-1"
              onClick={() => setBulkDeleteOpen(true)}
            >
              <Trash2 className="h-3.5 w-3.5" />
              일괄 삭제
            </Button>
          )}
          <Button
            variant="ghost"
            size="sm"
            className="h-8"
            onClick={() => setSelectedRowIds(new Set())}
          >
            선택 해제
          </Button>
        </>
      ) : (
        /* ── 기본 툴바 ── */
        <>
      {/* ── Group 1: 데이터 조회 (검색·필터·정렬) ── */}
      <div className="relative w-full sm:w-auto order-first">
        <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          ref={searchInputRef}
          className="h-8 w-full sm:w-[200px] pl-8 text-sm"
          placeholder="검색..."
          value={searchInputValue}
          onChange={(e) => handleSearchInput(e.target.value)}
        />
        {searchInputValue && (
          <button
            type="button"
            className="absolute right-2 top-1/2 -translate-y-1/2"
            aria-label="검색 초기화"
            onClick={() => {
              setSearchInputValue('')
              setSearchText('')
              setPage(1)
            }}
          >
            <X className="h-3 w-3 text-muted-foreground" />
          </button>
        )}
      </div>
      {searchText && list && (
        <span className="text-xs text-muted-foreground whitespace-nowrap">
          {list.total}건 검색됨
        </span>
      )}

      <Popover>
        <PopoverTrigger
          className="inline-flex items-center gap-1 rounded-md border border-input bg-background px-3 py-1 text-sm font-medium hover:bg-accent h-8"
        >
            <Filter className="h-3.5 w-3.5" />
            필터
            {hasActiveFilters && (
              <Badge variant="secondary" className="ml-1 h-5 px-1.5 text-[10px]">
                {filterConditions.length}
              </Badge>
            )}
        </PopoverTrigger>
        <PopoverContent align="start" className="w-auto min-w-[420px] p-3">
          <div className="mb-2 text-sm font-medium">필터 조건</div>
          <FilterBuilder
            fields={collection.fields ?? []}
            filterGroup={filterGroup}
            slug={collection.slug}
            onFilterGroupChange={(g) => {
              setFilterGroup(g)
              setPage(1)
            }}
          />
          {hasActiveFilters && (
            <Button
              variant="ghost"
              size="sm"
              className="mt-2 text-destructive"
              onClick={() => {
                setFilterGroup(emptyFilterGroup())
                setPage(1)
              }}
            >
              전체 해제
            </Button>
          )}
        </PopoverContent>
      </Popover>

      <Popover>
        <PopoverTrigger
          className="inline-flex items-center gap-1 rounded-md border border-input bg-background px-3 py-1 text-sm font-medium hover:bg-accent h-8"
        >
            <ArrowDownUp className="h-3.5 w-3.5" />
            정렬
            {sortItems.length > 0 && (
              <Badge variant="secondary" className="ml-1 h-5 px-1.5 text-[10px]">
                {sortItems.length}
              </Badge>
            )}
        </PopoverTrigger>
        <PopoverContent align="start" className="w-auto min-w-[360px] p-3">
          <div className="mb-2 text-sm font-medium">정렬 설정</div>
          <SortPanel
            fields={collection.fields ?? []}
            sorts={sortItems}
            onChange={handleSortPanelChange}
          />
          {sortItems.length > 0 && (
            <Button
              variant="ghost"
              size="sm"
              className="mt-2 text-destructive"
              onClick={() => setSortItems([])}
            >
              전체 해제
            </Button>
          )}
        </PopoverContent>
      </Popover>

      {/* ── Group 2: 데이터 입출력 + 프로세스 ── */}
      <div className="flex items-center gap-2 shrink-0 sm:border-l sm:pl-3 sm:ml-1">
        {process?.is_enabled && (
          <Button
            variant={processVisible ? 'default' : 'outline'}
            size="sm"
            className="h-8 gap-1"
            onClick={() => setProcessVisible(!processVisible)}
          >
            {processVisible ? (
              <Power className="h-3.5 w-3.5" />
            ) : (
              <PowerOff className="h-3.5 w-3.5" />
            )}
            프로세스
          </Button>
        )}

        <DropdownMenu>
          <DropdownMenuTrigger
            className="inline-flex items-center gap-1 rounded-md border border-input bg-background px-3 py-1 text-sm font-medium hover:bg-accent h-8"
            disabled={importingCSV}
          >
            {importingCSV
              ? <Loader2 className="h-3.5 w-3.5 animate-spin" />
              : <Ellipsis className="h-3.5 w-3.5" />}
            더보기
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuItem onClick={handleCsvExport}>
              <Download className="h-3.5 w-3.5 mr-2" />
              CSV 내보내기
            </DropdownMenuItem>
            <DropdownMenuItem onClick={handlePdfExport}>
              <FileText className="h-3.5 w-3.5 mr-2" />
              PDF 내보내기
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => setEmailDialogOpen(true)}>
              <Mail className="h-3.5 w-3.5 mr-2" />
              이메일 리포트
            </DropdownMenuItem>
            <RoleGate roles={['director', 'pm', 'engineer']}>
              <DropdownMenuItem onClick={() => fileInputRef.current?.click()}>
                <Upload className="h-3.5 w-3.5 mr-2" />
                가져오기
              </DropdownMenuItem>
            </RoleGate>
          </DropdownMenuContent>
        </DropdownMenu>
        <input
          ref={fileInputRef}
          type="file"
          accept=".csv"
          className="hidden"
          onChange={handleImportCSV}
        />
      </div>

      {/* ── Group 3: 뷰 관리 ── */}
      {((savedViews && savedViews.length > 0) || hasActiveFilters || sortItems.length > 0) && (
        <div className="flex items-center gap-1.5 border-l pl-3 ml-1">
          {savedViews && savedViews.length > 0 && (
            <>
              <Bookmark className="h-3.5 w-3.5 text-muted-foreground" />
              {savedViews.map((v) => (
                <Badge
                  key={v.id}
                  variant={activeView?.id === v.id ? 'default' : 'outline'}
                  className="cursor-pointer gap-1 text-xs"
                  onClick={() => {
                    if (activeView?.id === v.id) {
                      clearView()
                    } else {
                      applyView(v)
                    }
                  }}
                >
                  {v.name}
                  {activeView?.id === v.id && (
                    <button
                      type="button"
                      className="ml-0.5 hover:text-destructive"
                      aria-label="보기 삭제"
                      onClick={(e) => {
                        e.stopPropagation()
                        deleteSavedView.mutate(v.id, {
                          onSuccess: () => {
                            toast.success('보기가 삭제되었습니다')
                            clearView()
                          },
                          onError: (err) => toast.error(formatError(err)),
                        })
                      }}
                    >
                      <X className="h-3 w-3" />
                    </button>
                  )}
                </Badge>
              ))}
            </>
          )}

          {/* Active filter/sort indicator */}
          {(hasActiveFilters || sortItems.length > 0) && (
            <div className="flex items-center gap-2 border-l pl-2 ml-1 text-xs text-muted-foreground">
              {hasActiveFilters && (
                <span>{filterConditions.length}개 조건 적용됨</span>
              )}
              {sortItems.length > 0 && (
                <span>{sortItems.length}개 정렬 적용됨</span>
              )}
            </div>
          )}

          {(hasActiveFilters || sortItems.length > 0) && !savingView && (
            <Popover>
              <PopoverTrigger
                className="inline-flex items-center gap-1 rounded-md border border-input bg-background px-3 py-1 text-sm font-medium hover:bg-accent h-8"
              >
                <BookmarkPlus className="h-3.5 w-3.5" />
                보기 저장
              </PopoverTrigger>
              <PopoverContent align="start" className="w-64 p-3">
                <div className="space-y-2">
                  <div className="text-sm font-medium">현재 필터/정렬을 보기로 저장</div>
                  <Input
                    className="h-8"
                    placeholder="보기 이름"
                    value={newViewName}
                    onChange={(e) => setNewViewName(e.target.value)}
                    onKeyDown={(e) => e.key === 'Enter' && handleSaveView()}
                  />
                  <Button
                    size="sm"
                    className="w-full"
                    disabled={!newViewName.trim() || createSavedView.isPending}
                    onClick={handleSaveView}
                  >
                    {createSavedView.isPending ? '저장 중...' : '저장'}
                  </Button>
                </div>
              </PopoverContent>
            </Popover>
          )}
        </div>
      )}
        </>
      )}
    </div>
    {selectedRowIds.size === 0 && (
    <FilterChips
      filterGroup={filterGroup}
      sortItems={sortItems}
      fields={collection.fields ?? []}
      onRemoveFilter={(id) => {
        setFilterGroup((prev) => removeCondFromGroup(prev, id))
        setPage(1)
      }}
      onRemoveSort={(index) => {
        setSortItems((prev) => prev.filter((_, i) => i !== index))
      }}
      onClearAll={() => {
        setFilterGroup(emptyFilterGroup())
        setSortItems([])
        setPage(1)
      }}
    />
    )}
    </>
  )

  return (
    <div>
      <PageHeader
        breadcrumb={[
          { label: '앱 목록', href: '/apps' },
          { label: collection.label },
        ]}
        title={collection.label}
        description={collection.description}
        actions={
          <>
            <Link to={`/apps/${collection.id}/interface`}>
              <Button variant="outline" className="gap-1">
                <LayoutGrid className="h-4 w-4" />
                인터페이스
              </Button>
            </Link>
            {canManage && (
              <Link to={`/apps/${collection.id}/settings`}>
                <Button variant="outline">설정</Button>
              </Link>
            )}
            <Button onClick={() => navigate(`/apps/${appId}/entries/new`)}>
              {TERM.newRecord}
            </Button>
          </>
        }
      />

      {entriesLoading && !list && <LoadingState variant="table" />}
      {entriesError && <ErrorState error={entriesErr} onRetry={() => refetch()} />}

      {list && (
        <Tabs value={activeTab} onValueChange={setActiveTab}>
          <TabsList className="mb-4 max-w-full overflow-x-auto scrollbar-none">
            <TabsTrigger value="list">목록</TabsTrigger>
            {hasProcessKanban && <TabsTrigger value="status-kanban">상태별</TabsTrigger>}
            {hasCalendar && (
              <TabsTrigger value="calendar" className="gap-1">
                <Calendar className="h-3.5 w-3.5" />
                캘린더
              </TabsTrigger>
            )}
            <TabsTrigger value="form" className="gap-1">
              <FileText className="h-3.5 w-3.5" />
              폼
            </TabsTrigger>
          </TabsList>

          <TabsContent value="list" className="mt-0">
            <ErrorBoundary key="list">
            <DataTable
              columns={columns}
              data={list.data}
              total={list.total}
              page={page}
              limit={limit}
              onPageChange={setPage}
              onLimitChange={setLimit}
              onSortChange={handleHeaderSortChange}
              onRowClick={handleEntryClick}
              emptyTitle={searchText || hasActiveFilters ? '검색 결과가 없습니다' : TERM.noRecords}
              emptyDescription={searchText || hasActiveFilters ? '검색어 또는 필터 조건을 변경해 보세요.' : TERM.noRecordsDesc}
              emptyVariant={searchText || hasActiveFilters ? 'no-results' : 'empty'}
              emptyAction={
                searchText || hasActiveFilters ? (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setSearchText('')
                      setSearchInputValue('')
                      setFilterGroup(emptyFilterGroup())
                    }}
                  >
                    필터 초기화
                  </Button>
                ) : (
                  <Button size="sm" onClick={() => navigate(`/apps/${appId}/entries/new`)}>
                    {TERM.newRecord}
                  </Button>
                )
              }
              summaryRow={summaryRow}
              summaryFn={columnAggFn}
              onSummaryFnChange={handleAggFnChange}
              toolbar={tableToolbar}
              key={viewVisKey}
              initialColumnVisibility={viewVisibility ?? initialColumnVisibility}
              onColumnVisibilityChange={handleColumnVisibilityChange}
              initialColumnPinning={initialColumnPinning}
              onColumnPinningChange={handleColumnPinningChange}
              highlightRows={importedCount}
              newRowId={null}
              selectable
              selectedRowIds={selectedRowIds}
              onSelectionChange={(ids) => { setSelectedRowIds(ids); setSelectAllFilteredMode(false) }}
              totalFiltered={list.total}
              onSelectAllFiltered={handleSelectAllFiltered}
            />
            {list.data.length === 0 && (
              <ViewGuide fields={collection.fields ?? []} />
            )}
            </ErrorBoundary>
          </TabsContent>

          {hasProcessKanban && processGroupField && (
            <TabsContent value="status-kanban" className="mt-0">
              <ErrorBoundary key="status-kanban">
              <KanbanView
                slug={collection.slug}
                groupField={processGroupField}
                fields={collection.fields ?? []}
                filters={filters}
                onCardClick={handleEntryClick}
                onCardMove={handleProcessCardMove}
                onAddEntry={() => navigate(`/apps/${appId}/entries/new`)}
              />
              </ErrorBoundary>
            </TabsContent>
          )}

          {hasCalendar && dateField && (
            <TabsContent value="calendar" className="mt-0">
              <ErrorBoundary key="calendar">
              <CalendarView
                slug={collection.slug}
                dateField={dateField}
                fields={collection.fields ?? []}
                filters={filters}
                onEntryClick={handleEntryClick}
                onEntryUpdate={handleGanttUpdate}
                onCreateEntry={(prefill) => {
                  const params = new URLSearchParams()
                  for (const [k, v] of Object.entries(prefill)) {
                    if (v != null) params.set(k, String(v))
                  }
                  navigate(`/apps/${appId}/entries/new?${params.toString()}`)
                }}
              />
              </ErrorBoundary>
            </TabsContent>
          )}


          <TabsContent value="form" className="mt-0">
            <ErrorBoundary key="form">
            <FormView
              fields={collection.fields ?? []}
              entries={list.data}
              onEntryClick={handleEntryClick}
              onEntrySubmit={handleFormViewSubmit}
              submitting={createEntry.isPending || updateEntry.isPending}
              process={process}
              slug={collection.slug}
              collectionId={collection.id}
              total={list.total}
            />
            </ErrorBoundary>
          </TabsContent>
        </Tabs>
      )}


      <CSVImportPreview
        open={csvPreviewOpen}
        onOpenChange={setCsvPreviewOpen}
        file={csvPreviewFile}
        fields={collection.fields ?? []}
        onConfirm={handleCSVConfirm}
      />

      <BulkEditPanel
        open={bulkEditOpen}
        onOpenChange={setBulkEditOpen}
        fields={collection.fields ?? []}
        collectionId={collection.id}
        selectedCount={selectedRowIds.size}
        onApply={handleBulkEdit}
        loading={batchUpdateEntry.isPending}
      />


      <ConfirmDialog
        open={bulkDeleteOpen}
        onOpenChange={(open) => !open && setBulkDeleteOpen(false)}
        title={`${selectedRowIds.size}건의 ${TERM.record}를 삭제하시겠습니까?`}
        description="삭제된 데이터는 휴지통에서 복구할 수 있습니다."
        variant="destructive"
        confirmLabel={`${selectedRowIds.size}건 삭제`}
        onConfirm={handleBulkDelete}
        loading={bulkDelete.isPending}
      />

      {/* Email report dialog */}
      <Dialog open={emailDialogOpen} onOpenChange={setEmailDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>이메일 리포트</DialogTitle>
            <DialogDescription>
              현재 필터가 적용된 데이터를 PDF로 생성하여 이메일로 전송합니다.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3 py-2">
            <div className="space-y-1.5">
              <Label htmlFor="email-to">받는 사람</Label>
              <Input
                id="email-to"
                type="email"
                placeholder="user@example.com"
                value={emailTo}
                onChange={(e) => setEmailTo(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="email-msg">메시지 (선택)</Label>
              <Textarea
                id="email-msg"
                placeholder="리포트에 대한 설명을 입력하세요"
                rows={3}
                value={emailMessage}
                onChange={(e) => setEmailMessage(e.target.value)}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEmailDialogOpen(false)}>
              취소
            </Button>
            <Button onClick={handleEmailReport} disabled={!emailTo || emailSending}>
              {emailSending ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : <Mail className="h-4 w-4 mr-2" />}
              전송
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <HotkeyHelpDialog open={hotkeyHelpOpen} onOpenChange={setHotkeyHelpOpen} />
    </div>
  )
}

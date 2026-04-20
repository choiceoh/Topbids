import { ArrowLeft, Copy, Loader2, Printer, Trash2 } from 'lucide-react'
import { useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router'
import { toast } from 'sonner'

import ConfirmDialog from '@/components/common/ConfirmDialog'
import ErrorState from '@/components/common/ErrorState'
import LoadingState from '@/components/common/LoadingState'
import EntryComments from '@/components/works/EntryComments'
import EntryForm from '@/components/works/EntryForm'
import EntryHistory from '@/components/works/EntryHistory'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useAIAvailable } from '@/contexts/AIAvailabilityContext'
import { useAIPrefill } from '@/hooks/useAI'
import { useCollection } from '@/hooks/useCollections'
import {
  useCreateEntry,
  useDeleteEntry,
  useEntry,
  useEntryDefaults,
  useUpdateEntry,
} from '@/hooks/useEntries'
import { useProcess } from '@/hooks/useProcess'
import { useConflictAwareUpdate } from '@/hooks/useConflictAwareUpdate'
import { useRetryToast } from '@/hooks/useRetryToast'
import { formatError } from '@/lib/api'
import { TERM } from '@/lib/constants'
import type { EntryRow } from '@/lib/types'

export default function EntryPage() {
  const { appId, entryId } = useParams()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const duplicateSourceId = searchParams.get('duplicate') ?? undefined
  const isEdit = !!entryId
  const isNew = !entryId

  // Data fetching
  const { data: collection, isLoading: colLoading, error: colErr } = useCollection(appId)
  const slug = collection?.slug
  const { data: process } = useProcess(appId)
  const { data: entryData, isLoading: entryLoading, refetch: refetchEntry } = useEntry(
    slug,
    entryId,
    'auto',
  )
  const { data: duplicateSource } = useEntry(
    isNew ? slug : undefined,
    duplicateSourceId,
    'auto',
  )
  const { data: entryDefaults } = useEntryDefaults(isNew && !duplicateSourceId ? slug : undefined)

  // Mutations
  const createEntry = useCreateEntry(slug ?? '')
  const updateEntry = useUpdateEntry(slug ?? '')
  const deleteEntry = useDeleteEntry(slug ?? '')
  const retryToast = useRetryToast()
  const onConflictError = useConflictAwareUpdate(refetchEntry)

  // Autosave state
  const [autosaveStatus, setAutosaveStatus] = useState<'idle' | 'saving' | 'saved'>('idle')
  const autosaveTimerRef = useRef<ReturnType<typeof setTimeout>>(null)

  // AI prefill (create mode only)
  const aiAvailable = useAIAvailable()
  const prefill = useAIPrefill(slug)
  const [aiPrompt, setAiPrompt] = useState('')
  const [prefillData, setPrefillData] = useState<Record<string, unknown> | null>(null)
  const [formKey, setFormKey] = useState(0)

  // Delete confirmation
  const [deleteOpen, setDeleteOpen] = useState(false)

  const goBack = () => navigate(`/apps/${appId}`)

  if (colLoading || (isEdit && entryLoading)) return <LoadingState />
  if (colErr) return <ErrorState error={colErr} />
  if (!collection) return null

  const fields = collection.fields ?? []

  // Build initial data for the form
  let initialData: Record<string, unknown> | undefined
  if (isEdit) {
    initialData = entryData
  } else if (duplicateSource) {
    // Strip identity/system fields for duplicate
    const { id: _id, _version, created_at: _ca, updated_at: _ua, _created_by, _optimistic, _updated_at, _created_at, ...rest } = duplicateSource as EntryRow
    initialData = rest
  } else if (entryDefaults && Object.keys(entryDefaults).length > 0) {
    initialData = entryDefaults
  }

  function handleSubmit(data: Record<string, unknown>) {
    if (isEdit && entryId) {
      const version = (entryData as EntryRow | undefined)?._version
      if (version != null) data._version = version
      setAutosaveStatus('saving')
      updateEntry.mutate(
        { id: entryId, body: data },
        {
          onSuccess: () => {
            toast.success('수정되었습니다')
            if (autosaveTimerRef.current) clearTimeout(autosaveTimerRef.current)
            autosaveTimerRef.current = setTimeout(() => {
              setAutosaveStatus('saved')
              autosaveTimerRef.current = setTimeout(() => setAutosaveStatus('idle'), 2000)
            }, 500)
          },
          onError: (err) => {
            setAutosaveStatus('idle')
            onConflictError(err, () => retryToast(err, () => handleSubmit(data)))
          },
        },
      )
    } else {
      createEntry.mutate(data, {
        onSuccess: () => {
          toast.success('생성되었습니다')
          goBack()
        },
        onError: (err) => retryToast(err, () => handleSubmit(data)),
      })
    }
  }

  function handleDuplicate() {
    if (!entryId) return
    navigate(`/apps/${appId}/entries/new?duplicate=${entryId}`)
  }

  function handleDelete() {
    if (!entryId) return
    deleteEntry.mutate(entryId, {
      onSuccess: () => {
        toast.success('삭제되었습니다')
        goBack()
      },
      onError: (err) => toast.error(formatError(err)),
    })
  }

  function handleAIPrefill() {
    if (!aiPrompt.trim() || prefill.isPending) return
    prefill.mutate(aiPrompt.trim(), {
      onSuccess: (res) => {
        setPrefillData({ ...initialData, ...res })
        setFormKey((k) => k + 1)
        setAiPrompt('')
      },
    })
  }

  const pageTitle = isEdit
    ? `${TERM.record} 편집`
    : duplicateSourceId
      ? '항목 복제'
      : TERM.newRecord

  return (
    <div className="mx-auto max-w-6xl px-4 py-6">
      {/* Header */}
      <div className="mb-6 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="sm" onClick={goBack} className="gap-1">
            <ArrowLeft className="h-4 w-4" />
            목록
          </Button>
          <div className="text-muted-foreground">/</div>
          <h1 className="text-lg font-semibold">{collection.label}</h1>
          <div className="text-muted-foreground">/</div>
          <span className="text-lg text-muted-foreground">{pageTitle}</span>
        </div>
        {isEdit && (
          <div className="flex items-center gap-1" data-print-hide>
            <Button
              variant="outline"
              size="sm"
              className="h-7 gap-1 text-xs"
              onClick={() => window.print()}
            >
              <Printer className="h-3.5 w-3.5" />
              인쇄
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="h-7 gap-1 text-xs"
              onClick={handleDuplicate}
            >
              <Copy className="h-3.5 w-3.5" />
              복제
            </Button>
            <Button
              variant="outline"
              size="sm"
              className="h-7 gap-1 text-xs text-destructive hover:text-destructive"
              onClick={() => setDeleteOpen(true)}
            >
              <Trash2 className="h-3.5 w-3.5" />
              삭제
            </Button>
          </div>
        )}
      </div>

      {/* Content */}
      <div className={isEdit ? 'flex gap-8 lg:flex-row flex-col' : ''}>
        {/* Form column */}
        <div className={isEdit ? 'min-w-0 flex-1' : 'mx-auto max-w-2xl'}>
          {/* AI Prefill (create mode) */}
          {isNew && aiAvailable && slug && (
            <div className="mb-4 flex gap-1">
              <Input
                value={aiPrompt}
                onChange={(e) => setAiPrompt(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') handleAIPrefill()
                }}
                placeholder="한 줄로 입력하세요..."
                className="h-8 text-sm"
                disabled={prefill.isPending}
              />
              <Button
                type="button"
                size="sm"
                className="h-8 shrink-0"
                disabled={!aiPrompt.trim() || prefill.isPending}
                onClick={handleAIPrefill}
              >
                {prefill.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : '채우기'}
              </Button>
            </div>
          )}

          <EntryForm
            key={isEdit ? entryId : formKey}
            fields={fields}
            initialData={prefillData ?? initialData}
            slug={slug}
            collectionId={collection.id}
            autosave={isEdit}
            autosaveStatus={isEdit ? autosaveStatus : undefined}
            onSubmit={handleSubmit}
            onCancel={goBack}
            submitting={createEntry.isPending || updateEntry.isPending}
            process={process}
          />
        </div>

        {/* Sidebar (edit mode only) */}
        {isEdit && entryId && slug && (
          <div className="w-full shrink-0 space-y-6 lg:w-80">
            <div>
              <h3 className="mb-3 text-sm font-medium">댓글</h3>
              <EntryComments slug={slug} recordId={entryId} />
            </div>
            <div>
              <h3 className="mb-3 text-sm font-medium">이력</h3>
              <EntryHistory slug={slug} recordId={entryId} fields={fields} />
            </div>
          </div>
        )}
      </div>

      {/* Delete confirmation */}
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title="데이터를 삭제하시겠습니까?"
        description="삭제된 데이터는 복구할 수 없습니다."
        variant="destructive"
        confirmLabel="삭제"
        loading={deleteEntry.isPending}
        onConfirm={handleDelete}
      />
    </div>
  )
}

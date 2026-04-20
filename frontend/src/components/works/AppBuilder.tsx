import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router'
import { toast } from 'sonner'

import ConfirmDialog from '@/components/common/ConfirmDialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Loader2, Sparkles, X } from 'lucide-react'
import {
  useAddField,
  useCollection,
  useCollections,
  useCreateCollection,
  useDeleteField,
  useUpdateCollection,
  useUpdateField,
} from '@/hooks/useCollections'
import { useUnsavedChanges } from '@/hooks/useUnsavedChanges'
import { useAIAvailable } from '@/contexts/AIAvailabilityContext'
import type { AIBuildResult } from '@/hooks/useAI'
import { useAIGenerateSlug } from '@/hooks/useAI'
import { formatError } from '@/lib/api'
import type { CreateCollectionReq, Field, FieldType } from '@/lib/types'
import ErrorState from '@/components/common/ErrorState'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'

import CoachMark from '@/components/common/CoachMark'

import AIBuildDialog from './AIBuildDialog'
import FieldPalette from './FieldPalette'
import { type FieldDraft } from './FieldPreview'
import FieldProperties from './FieldProperties'
import FormPreview from './FormPreview'

const BUILDER_COACH_STEPS = [
  {
    title: '항목 팔레트',
    description: '여기서 원하는 항목을 드래그하거나 클릭하여 입력화면에 추가하세요.',
    target: '[data-coach="palette"]',
  },
  {
    title: '입력화면 미리보기',
    description: '추가한 항목들이 실제 입력화면에 어떻게 보이는지 확인할 수 있습니다.',
    target: '[data-coach="preview"]',
  },
  {
    title: '항목 속성',
    description: '항목을 선택하면 여기서 이름, 필수 여부, 옵션 등을 설정할 수 있습니다.',
    target: '[data-coach="properties"]',
  },
]

function fieldToFieldDraft(f: Field): FieldDraft {
  return {
    id: f.id,
    slug: f.slug,
    label: f.label,
    field_type: f.field_type,
    is_required: f.is_required,
    is_unique: f.is_unique,
    is_indexed: f.is_indexed,
    default_value: f.default_value as string | undefined,
    width: f.width,
    height: f.height,
    options: f.options,
    relation: f.relation
      ? {
          target_collection_id: f.relation.target_collection_id,
          relation_type: f.relation.relation_type as 'one_to_one' | 'one_to_many' | 'many_to_many',
          on_delete: f.relation.on_delete,
        }
      : undefined,
  }
}

function hasFieldChanged(draft: FieldDraft, original: Field): boolean {
  return (
    draft.label !== original.label ||
    draft.field_type !== original.field_type ||
    draft.is_required !== original.is_required ||
    draft.is_unique !== original.is_unique ||
    draft.is_indexed !== original.is_indexed ||
    draft.width !== original.width ||
    draft.height !== original.height ||
    JSON.stringify(draft.options ?? {}) !== JSON.stringify(original.options ?? {})
  )
}

export default function AppBuilder({ appId }: { appId?: string }) {
  const navigate = useNavigate()
  const isEditMode = !!appId
  const fieldCounter = useRef(0)
  const [slug, setSlug] = useState('')
  const [slugManual, setSlugManual] = useState(false)
  const [label, setLabel] = useState('')
  const [description, setDescription] = useState('')
  const [fields, setFields] = useState<FieldDraft[]>([])
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)
  const initializedRef = useRef(false)
  const originalFieldsRef = useRef<Field[]>([])

  const { data: collections = [] } = useCollections()
  const {
    data: existingCollection,
    isLoading: isLoadingCollection,
    isError: isCollectionError,
    error: collectionError,
    refetch: refetchCollection,
  } = useCollection(appId)
  const createCollection = useCreateCollection()
  const updateCollection = useUpdateCollection(appId ?? '')
  const addField = useAddField(appId ?? '')
  const updateField = useUpdateField()
  const deleteField = useDeleteField()
  const aiAvailable = useAIAvailable()
  const generateSlug = useAIGenerateSlug()
  const [showAINudge, setShowAINudge] = useState(false)

  // Initialize state from existing collection in edit mode
  useEffect(() => {
    if (isEditMode && existingCollection && !initializedRef.current) {
      initializedRef.current = true
      setLabel(existingCollection.label)
      setSlug(existingCollection.slug)
      setSlugManual(true)
      setDescription(existingCollection.description || '')
      originalFieldsRef.current = existingCollection.fields ?? []
      setFields((existingCollection.fields ?? []).map(fieldToFieldDraft))
    }
  }, [isEditMode, existingCollection])

  useEffect(() => {
    if (!isEditMode && aiAvailable) {
      try {
        if (!localStorage.getItem('phaeton:ai-builder-nudge-dismissed')) setShowAINudge(true)
      } catch { /* ignore */ }
    }
  }, [isEditMode, aiAvailable])
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(null)

  const selectedField = fields.find((f) => f.id === selectedId) || null

  const isDirty = useMemo(() => {
    if (!isEditMode) {
      return label.trim() !== '' || slug.trim() !== '' || description.trim() !== '' || fields.length > 0
    }
    if (!existingCollection) return false
    return (
      label !== existingCollection.label ||
      description !== (existingCollection.description || '') ||
      JSON.stringify(fields) !== JSON.stringify((existingCollection.fields ?? []).map(fieldToFieldDraft))
    )
  }, [isEditMode, label, slug, description, fields, existingCollection])

  const blocker = useUnsavedChanges(isDirty)

  const requestSlug = useCallback((text: string) => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      generateSlug.mutate(text, {
        onSuccess: (res) => {
          setSlug((prev) => {
            // Only update if user hasn't manually edited
            if (!slugManual) return res.slug
            return prev
          })
        },
      })
    }, 500)
  }, [slugManual, generateSlug])

  function handleLabelChange(value: string) {
    setLabel(value)
    if (!slugManual && value.trim() && aiAvailable) {
      requestSlug(value.trim())
    }
    if (!slugManual && !value.trim()) {
      setSlug('')
    }
  }

  function handleSlugChange(value: string) {
    const sanitized = value.toLowerCase().replace(/[^a-z0-9_]/g, '')
    setSlug(sanitized)
    if (sanitized) setSlugManual(true)
    else setSlugManual(false)
  }

  function handleAIApply(result: AIBuildResult) {
    setSlug(result.slug)
    setSlugManual(false)
    setLabel(result.label)
    setDescription(result.description || '')
    const drafts: FieldDraft[] = result.fields.map((f) => ({
      id: `ai_${++fieldCounter.current}`,
      slug: f.slug,
      label: f.label,
      field_type: f.field_type as FieldType,
      is_required: f.is_required,
      is_unique: false,
      is_indexed: false,
      width: f.width || 6,
      height: f.height || 1,
      options: f.options,
    }))
    setFields(drafts)
    setSelectedId(null)
  }

  function handleAddField(fieldType: FieldType, presetOptions?: Record<string, unknown>, index?: number) {
    const id = `field_${++fieldCounter.current}`
    const draft: FieldDraft = {
      id,
      slug: '',
      label: '새 항목',
      field_type: fieldType,
      is_required: false,
      is_unique: false,
      is_indexed: false,
      width: 3,
      height: 1,
      options: presetOptions,
    }
    if (index !== undefined) {
      const updated = [...fields]
      updated.splice(index, 0, draft)
      setFields(updated)
    } else {
      setFields([...fields, draft])
    }
    setSelectedId(id)
  }

  function handleFieldChange(updated: FieldDraft) {
    setFields(fields.map((f) => (f.id === updated.id ? updated : f)))
  }

  function handleRemoveField(id: string) {
    setFields(fields.filter((f) => f.id !== id))
    if (selectedId === id) setSelectedId(null)
  }

  function handleSave() {
    setError('')
    if (!slug.trim() || !label.trim()) {
      setError('영문 ID와 이름은 필수입니다.')
      return
    }
    for (const f of fields) {
      if (!f.slug) {
        setError(`항목 "${f.label}"의 영문 ID가 비어 있습니다.`)
        return
      }
    }

    const body: CreateCollectionReq = {
      slug,
      label,
      description: description || undefined,
      fields: fields.map((f) => ({
        slug: f.slug,
        label: f.label,
        field_type: f.field_type,
        is_required: f.is_required,
        is_unique: f.is_unique,
        is_indexed: f.is_indexed,
        default_value: f.default_value,
        width: f.width,
        height: f.height,
        options: f.options,
        relation: f.relation,
      })),
    }

    createCollection.mutate(body, {
      onSuccess: (created) => {
        toast.success(`${created.label} 앱이 생성되었습니다`)
        navigate(`/apps/${created.id}`)
      },
      onError: (err) => {
        const msg = formatError(err)
        setError(msg)
        toast.error(msg)
      },
    })
  }

  async function handleEditSave() {
    setError('')
    if (!label.trim()) {
      setError('이름은 필수입니다.')
      return
    }
    for (const f of fields) {
      if (!f.slug) {
        setError(`항목 "${f.label}"의 영문 ID가 비어 있습니다.`)
        return
      }
    }

    setSaving(true)
    try {
      const originalFields = originalFieldsRef.current
      const originalById = new Map(originalFields.map((f) => [f.id, f]))
      const currentIds = new Set(fields.map((f) => f.id))

      // 1. Update collection metadata if changed
      if (
        label !== existingCollection?.label ||
        description !== (existingCollection?.description || '')
      ) {
        await updateCollection.mutateAsync({
          label: label.trim(),
          description: description.trim() || undefined,
        })
      }

      // 2. Delete removed fields
      for (const orig of originalFields) {
        if (!currentIds.has(orig.id)) {
          await deleteField.mutateAsync({ fieldId: orig.id, confirm: true })
        }
      }

      // 3. Add new fields (id starts with 'field_' or 'ai_')
      for (const draft of fields) {
        if (!originalById.has(draft.id)) {
          const input = {
            slug: draft.slug,
            label: draft.label,
            field_type: draft.field_type,
            is_required: draft.is_required,
            is_unique: draft.is_unique,
            is_indexed: draft.is_indexed,
            default_value: draft.default_value,
            options: draft.options,
            width: draft.width,
            height: draft.height,
            relation: draft.relation,
          }
          await addField.mutateAsync({ input, confirm: true })
        }
      }

      // 4. Update modified fields
      for (const draft of fields) {
        const orig = originalById.get(draft.id)
        if (orig && hasFieldChanged(draft, orig)) {
          await updateField.mutateAsync({
            fieldId: draft.id,
            input: {
              label: draft.label,
              field_type: draft.field_type,
              is_required: draft.is_required,
              is_unique: draft.is_unique,
              is_indexed: draft.is_indexed,
              options: draft.options,
              width: draft.width,
              height: draft.height,
            },
            confirm: true,
          })
        }
      }

      toast.success('앱이 수정되었습니다')
      navigate(`/apps/${appId}`)
    } catch (err) {
      const msg = formatError(err)
      setError(msg)
      toast.error(msg)
    } finally {
      setSaving(false)
    }
  }

  if (isEditMode && isLoadingCollection) return <LoadingState />
  if (isEditMode && isCollectionError) return <ErrorState error={collectionError} onRetry={() => refetchCollection()} />

  return (
    <div className="space-y-4">
      {isEditMode && existingCollection ? (
        <PageHeader
          breadcrumb={[
            { label: '앱 목록', href: '/apps' },
            { label: existingCollection.label, href: `/apps/${existingCollection.id}` },
            { label: '수정' },
          ]}
          title="앱 수정"
          description={`/${existingCollection.slug} 앱의 항목 구조를 변경합니다`}
        />
      ) : (
        <div className="flex items-center justify-between">
          <h1 className="text-2xl font-bold">새 앱 만들기</h1>
          <AIBuildDialog onApply={handleAIApply} />
        </div>
      )}

      {showAINudge && (
        <div className="flex items-center gap-2 rounded-md border bg-muted/50 px-3 py-2 text-sm text-muted-foreground animate-fade-in">
          <Sparkles className="h-4 w-4 shrink-0" />
          <span>AI에게 앱 구조를 설명하면 자동으로 만들어 줍니다</span>
          <button
            type="button"
            onClick={() => {
              setShowAINudge(false)
              try { localStorage.setItem('phaeton:ai-builder-nudge-dismissed', '1') } catch { /* ignore */ }
            }}
            className="ml-auto text-muted-foreground/60 hover:text-foreground"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      )}

      {error && <p className="text-sm text-destructive">{error}</p>}

      <div className="grid grid-cols-1 gap-4 md:grid-cols-[240px_1fr]">
        <div data-coach="palette" className="max-h-[calc(100vh-160px)] overflow-y-auto rounded-lg border p-3">
          <FieldPalette onAdd={handleAddField} />
          {selectedField && (
            <div data-coach="properties">
              <div className="my-3 border-t" />
              <FieldProperties field={selectedField} collections={collections} siblingFields={fields} onChange={handleFieldChange} />
            </div>
          )}
        </div>
        <div data-coach="preview" className="rounded-lg border p-3 space-y-4">
          <div className="space-y-3">
            <p className="text-sm font-medium text-muted-foreground">기본 정보</p>
            <div className="grid grid-cols-6 gap-2">
              <div className="col-span-2 space-y-1">
                <Label>앱 이름 (한글)</Label>
                <Input value={label} onChange={(e) => handleLabelChange(e.target.value)} placeholder="인허가 체크리스트" />
              </div>
              <div className="col-span-1 space-y-1">
                <Label>영문 ID</Label>
                <div className="relative">
                  <Input
                    value={slug}
                    onChange={(e) => handleSlugChange(e.target.value)}
                    placeholder="자동 생성됨"
                    readOnly={isEditMode}
                    className={isEditMode ? 'bg-muted' : ''}
                  />
                  {generateSlug.isPending && (
                    <Loader2 className="absolute top-1/2 right-3 h-4 w-4 -translate-y-1/2 animate-spin text-muted-foreground" />
                  )}
                </div>
              </div>
              <div className="col-span-3 space-y-1">
                <Label>설명</Label>
                <Input value={description} onChange={(e) => setDescription(e.target.value)} placeholder="앱 설명" />
              </div>
            </div>
          </div>
          <FormPreview
            fields={fields}
            selectedId={selectedId}
            onSelect={setSelectedId}
            onReorder={setFields}
            onRemove={handleRemoveField}
            onAdd={handleAddField}
            onFieldChange={handleFieldChange}
          />
        </div>
      </div>

      <div className="flex justify-end pb-16">
        <Button
          onClick={isEditMode ? handleEditSave : handleSave}
          disabled={(isEditMode ? saving : createCollection.isPending) || !label.trim() || !slug.trim()}
        >
          {(isEditMode ? saving : createCollection.isPending) ? '저장 중...' : '저장'}
        </Button>
      </div>

      <ConfirmDialog
        open={blocker.state === 'blocked'}
        onOpenChange={(open) => { if (!open) blocker.reset?.() }}
        title="저장하지 않고 나가시겠습니까?"
        description="작성 중인 내용이 저장되지 않습니다."
        confirmLabel="나가기"
        cancelLabel="계속 작성"
        onConfirm={() => blocker.proceed?.()}
      />

      <CoachMark storageKey="phaeton:coach:builder" steps={BUILDER_COACH_STEPS} />
    </div>
  )
}

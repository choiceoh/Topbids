/**
 * EntryForm — Dynamic form that renders input fields based on collection schema.
 *
 * Uses plain useState instead of react-hook-form because the form shape is
 * determined at runtime by the collection's field definitions.
 *
 * Features:
 * - Field-type-specific inputs (text, select, relation combobox, date picker, etc.)
 * - Server-side validation with inline error display
 * - Client-side required/range validation on blur
 * - Autosave mode (1.5s debounce) for edit forms
 * - Similar records detection for duplicate prevention
 * - Process workflow transition buttons
 * - Layout fields (label, line, spacer) for visual grouping
 */
import { Check } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'

import RelationCombobox from '@/components/common/RelationCombobox'
import RelationMultiCombobox from '@/components/common/RelationMultiCombobox'
import UserCombobox from '@/components/common/UserCombobox'
import SpreadsheetInput from './SpreadsheetInput'
import { useAvailableTransitions, useSimilarRecords } from '@/hooks/useEntries'
import { X } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { DatePicker } from '@/components/ui/date-picker'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { api } from '@/lib/api'
import { formatError } from '@/lib/api/errors'
import { isLayoutType } from '@/lib/constants'
import type { Field, Process, SubColumn } from '@/lib/types'
import { extractRelationId, extractRelationIds, getChoices, getDisplayType, getFieldOptions, getVisibilityRules, isExpandedRecord } from '@/lib/fieldGuards'

interface Props {
  fields: Field[]
  initialData?: Record<string, unknown>
  onSubmit: (data: Record<string, unknown>) => void
  onCancel: () => void
  submitting?: boolean
  process?: Process
  slug?: string
  /** Collection UUID — used to fetch server-side transitions. */
  collectionId?: string
  /** When true, debounce-saves on every field change (edit mode). */
  autosave?: boolean
  /** Visual autosave state shown in the form footer. */
  autosaveStatus?: 'idle' | 'saving' | 'saved'
}

// EntryForm renders a dynamic form built from a collection's fields.
// State is held in plain useState because the form shape is decided at
// runtime — react-hook-form's typed Path<T> assumes a static schema.
//
// Validation is done server-side: required fields are still marked with *
// and submitted, but the actual error display comes from the API response
// (toast in the parent).
export default function EntryForm({
  fields,
  initialData,
  onSubmit,
  onCancel,
  submitting,
  process,
  slug,
  collectionId,
  autosave,
  autosaveStatus,
}: Props) {
  const [data, setData] = useState<Record<string, unknown>>(initialData ?? {})
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [validated, setValidated] = useState<Set<string>>(new Set())
  const [shakeKey, setShakeKey] = useState(0)
  // Similar records detection: track the first text field value with debounce.
  const isNew = !initialData?.id

  // First editable field slug — used to autoFocus on new entry creation.
  const firstEditableSlug = isNew
    ? fields.find((f) => !isLayoutType(f.field_type) && !['formula', 'autonumber', 'lookup', 'rollup'].includes(f.field_type))?.slug
    : undefined
  const firstTextField = isNew ? fields.find((f) => f.field_type === 'text') : undefined
  const [similarQuery, setSimilarQuery] = useState('')
  const similarDebounceRef = useRef<ReturnType<typeof setTimeout>>(null)
  const { data: similarRecords } = useSimilarRecords(
    isNew ? slug : undefined,
    similarQuery,
    firstTextField?.slug,
  )

  const firstTextFieldValue = firstTextField ? String(data[firstTextField.slug] ?? '') : ''
  useEffect(() => {
    if (!firstTextField || !isNew) return
    if (similarDebounceRef.current) clearTimeout(similarDebounceRef.current)
    similarDebounceRef.current = setTimeout(() => setSimilarQuery(firstTextFieldValue), 800)
    return () => { if (similarDebounceRef.current) clearTimeout(similarDebounceRef.current) }
  }, [firstTextFieldValue, firstTextField, isNew])

  // Autosave: debounce submit when data changes (edit mode only).
  const autosaveRef = useRef<ReturnType<typeof setTimeout>>(null)
  const initialRef = useRef(initialData)
  useEffect(() => {
    if (!autosave || isNew) return
    // Skip if data hasn't changed from initial.
    const changed = Object.keys(data).some((k) => data[k] !== initialRef.current?.[k])
    if (!changed) return
    if (autosaveRef.current) clearTimeout(autosaveRef.current)
    autosaveRef.current = setTimeout(() => onSubmit(data), 1500)
    return () => { if (autosaveRef.current) clearTimeout(autosaveRef.current) }
  }, [data, autosave, isNew, onSubmit])

  function setValue(name: string, value: unknown) {
    setData((prev) => ({ ...prev, [name]: value }))
    if (errors[name]) setErrors((prev) => { const next = { ...prev }; delete next[name]; return next })
  }

  function handleBlur(field: Field) {
    const val = data[field.slug]
    const isEmpty = val === undefined || val === null || val === '' || (Array.isArray(val) && val.length === 0)
    if (field.is_required && !isLayoutType(field.field_type) && isEmpty) {
      setErrors((prev) => ({ ...prev, [field.slug]: '필수 항목입니다' }))
      setValidated((prev) => { const next = new Set(prev); next.delete(field.slug); return next })
      return
    }
    // Number range validation
    if ((field.field_type === 'number' || field.field_type === 'integer') && val != null && val !== '') {
      const num = Number(val)
      const numOpts = getFieldOptions(field, 'number')
      const minVal = numOpts?.min
      const maxVal = numOpts?.max
      if (minVal != null && num < minVal) {
        setErrors((prev) => ({ ...prev, [field.slug]: `최소 ${minVal} 이상이어야 합니다` }))
        setValidated((prev) => { const next = new Set(prev); next.delete(field.slug); return next })
        return
      } else if (maxVal != null && num > maxVal) {
        setErrors((prev) => ({ ...prev, [field.slug]: `최대 ${maxVal} 이하여야 합니다` }))
        setValidated((prev) => { const next = new Set(prev); next.delete(field.slug); return next })
        return
      }
    }
    if (!isEmpty) {
      setValidated((prev) => new Set(prev).add(field.slug))
    }
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const newErrors: Record<string, string> = {}
    for (const field of fields) {
      if (!field.is_required || isLayoutType(field.field_type)) continue
      const val = data[field.slug]
      if (val === undefined || val === null || val === '' || (Array.isArray(val) && val.length === 0)) {
        newErrors[field.slug] = '필수 항목입니다'
      }
    }
    if (Object.keys(newErrors).length > 0) {
      setErrors(newErrors)
      setShakeKey((k) => k + 1)
      return
    }
    onSubmit(data)
  }

  // Process status transitions — fetched from the server (role-filtered).
  const currentStatus = initialData?._status as string | undefined
  const { data: transitionsData } = useAvailableTransitions(
    process?.is_enabled && currentStatus && initialData?.id ? collectionId : undefined,
    currentStatus,
  )
  const availableTransitions = transitionsData?.transitions ?? []

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      {/* Status transition UI */}
      {process?.is_enabled && currentStatus && (
        <div className="rounded-lg border bg-muted/30 p-3">
          <div className="mb-2 flex items-center gap-2 text-sm">
            <span className="text-muted-foreground">현재 상태:</span>
            <span
              className="inline-block rounded px-2 py-0.5 text-xs font-medium text-white"
              style={{
                backgroundColor:
                  process.statuses.find((s) => s.name === currentStatus)?.color ?? '#6b7280',
              }}
            >
              {currentStatus}
            </span>
          </div>
          {availableTransitions.length > 0 && (
            <TooltipProvider>
            <div className="flex flex-wrap gap-2">
              {availableTransitions.map((t, i) => (
                <div key={i} className="flex flex-col items-start">
                  {t.is_blocked ? (
                    <Tooltip>
                      <TooltipTrigger
                        className="cursor-not-allowed"
                        onClick={(e) => e.preventDefault()}
                      >
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          className="gap-1 pointer-events-none opacity-50"
                          disabled
                          tabIndex={-1}
                        >
                          {t.label}
                          <span className="text-muted-foreground">→</span>
                          <span
                            className="inline-block rounded px-1.5 py-0.5 text-xs text-white"
                            style={{ backgroundColor: t.to_color || '#6b7280' }}
                          >
                            {t.to_status}
                          </span>
                        </Button>
                      </TooltipTrigger>
                      {t.blocked_reason && (
                        <TooltipContent>{t.blocked_reason}</TooltipContent>
                      )}
                    </Tooltip>
                  ) : (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className="gap-1"
                      onClick={() => setValue('_status', t.to_status)}
                    >
                      {t.label}
                      <span className="text-muted-foreground">→</span>
                      <span
                        className="inline-block rounded px-1.5 py-0.5 text-xs text-white"
                        style={{ backgroundColor: t.to_color || '#6b7280' }}
                      >
                        {t.to_status}
                      </span>
                    </Button>
                  )}
                  {!t.is_blocked && t.allowed_user_names && t.allowed_user_names.length > 0 && (
                    <span className="mt-0.5 text-[10px] text-muted-foreground">
                      {t.allowed_user_names.join(', ')} 승인 가능
                    </span>
                  )}
                </div>
              ))}
            </div>
            </TooltipProvider>
          )}
          {typeof data._status === 'string' && data._status !== currentStatus && (
            <p className="mt-2 text-xs text-muted-foreground">
              저장 시 상태가 <strong>{String(data._status)}</strong>(으)로 변경됩니다.
            </p>
          )}
        </div>
      )}

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-6">
        {fields.map((field) => {
          // Conditional visibility: evaluate visibility_rules against current data.
          const visRules = getVisibilityRules(field)
          if (visRules && visRules.length > 0) {
            const allPass = visRules.every((rule) => {
              const fieldVal = data[rule.field_slug]
              const strVal = fieldVal != null ? String(fieldVal) : ''
              switch (rule.operator) {
                case 'eq': return strVal === (rule.value ?? '')
                case 'neq': return strVal !== (rule.value ?? '')
                case 'is_empty': return !strVal
                case 'is_not_empty': return !!strVal
                default: return true
              }
            })
            if (!allPass) return null
          }

          if (isLayoutType(field.field_type)) {
            return (
              <div key={field.id} className="col-span-full">
                <LayoutElement field={field} />
              </div>
            )
          }
          // Formula fields: show computed value as read-only.
          if (field.field_type === 'formula') {
            const span = field.width || 6
            const smSpan: Record<number, string> = {
              1: 'sm:col-span-1', 2: 'sm:col-span-2', 3: 'sm:col-span-3',
              4: 'sm:col-span-4', 5: 'sm:col-span-5', 6: 'sm:col-span-6',
            }
            const val = data[field.slug]
            return (
              <div key={field.id} className={`col-span-full ${smSpan[span] ?? 'sm:col-span-6'}`}>
                <Label className="text-muted-foreground">{field.label} (수식)</Label>
                <div className="mt-1 rounded-md bg-muted px-3 py-2 text-sm">
                  {val != null ? String(val) : '-'}
                </div>
              </div>
            )
          }
          const span = field.width || 6
          const smSpan: Record<number, string> = {
            1: 'sm:col-span-1', 2: 'sm:col-span-2', 3: 'sm:col-span-3',
            4: 'sm:col-span-4', 5: 'sm:col-span-5', 6: 'sm:col-span-6',
          }
          const fieldId = `field-${field.slug}`
          const errorId = `field-${field.slug}-error`
          const hasError = !!errors[field.slug]
          return (
            <div
              key={field.id}
              className={`col-span-full ${smSpan[span] ?? 'sm:col-span-6'}`}
            >
              <Label htmlFor={fieldId}>
                {field.label}
                {field.is_required && <span className="ml-1 text-destructive">*</span>}
              </Label>
              <div className="mt-1">
                <FieldInput
                  field={field}
                  value={extractValue(data[field.slug], field)}
                  onChange={(v) => setValue(field.slug, v)}
                  onBlur={() => handleBlur(field)}
                  autoFocus={field.slug === firstEditableSlug}
                  id={fieldId}
                  errorId={hasError ? errorId : undefined}
                  hasError={hasError}
                  isRequired={field.is_required}
                />
                {hasError ? (
                  <p key={shakeKey} id={errorId} role="alert" className="mt-1 text-xs text-destructive animate-shake animate-fade-in">{errors[field.slug]}</p>
                ) : validated.has(field.slug) ? (
                  <p className="mt-1 flex items-center gap-1 text-xs text-emerald-600 animate-fade-in">
                    <Check className="h-3 w-3" />
                    확인됨
                  </p>
                ) : null}
              </div>
            </div>
          )
        })}
      </div>
      {similarRecords && similarRecords.length > 0 && (
        <div className="rounded-md border border-dashed p-2 text-xs text-muted-foreground">
          <span>유사한 항목:</span>
          {similarRecords.slice(0, 3).map((r) => (
            <span key={r.id} className="ml-2">
              &ldquo;{r.value}&rdquo;
              <span className="ml-1 opacity-60">
                ({new Date(r.created_at).toLocaleDateString('ko')})
              </span>
            </span>
          ))}
        </div>
      )}
      <div className="flex items-center justify-end gap-2 pt-2 sticky bottom-0 bg-background pb-2 sm:static sm:pb-0">
        {autosave && autosaveStatus && autosaveStatus !== 'idle' && (
          <span className="mr-auto text-xs text-muted-foreground animate-fade-in">
            {autosaveStatus === 'saving' ? '저장 중...' : autosaveStatus === 'saved' ? (
              <span className="flex items-center gap-1 text-emerald-600">
                <Check className="h-3 w-3" />
                저장됨
              </span>
            ) : null}
          </span>
        )}
        <Button type="button" variant="outline" onClick={onCancel}>
          취소
        </Button>
        <Button type="submit" disabled={submitting}>
          {submitting ? '저장 중...' : '저장'}
        </Button>
      </div>
    </form>
  )
}

// extractValue normalises a value coming from the server. Relation fields
// can arrive either as a UUID string (no expand) or as a nested object
// (with expand). The form always works with the UUID.
function extractValue(value: unknown, field: Field): unknown {
  if (value == null) return value
  if (field.field_type === 'relation') {
    // M:N: value is an array of UUIDs or expanded objects.
    if (field.relation?.relation_type === 'many_to_many' && Array.isArray(value)) {
      return extractRelationIds(value)
    }
    // 1:1/1:N: value is a UUID or expanded object.
    if (typeof value === 'object' && !Array.isArray(value)) {
      return extractRelationId(value)
    }
  }
  if (field.field_type === 'user' && isExpandedRecord(value)) {
    return value.id
  }
  return value
}

function LayoutElement({ field }: { field: Field }) {
  switch (field.field_type) {
    case 'label':
      return (
        <p className="text-sm text-muted-foreground">
          {getFieldOptions(field, 'label')?.content || field.label}
        </p>
      )
    case 'line':
      return <hr className="my-2" />
    case 'spacer':
      return <div style={{ height: getFieldOptions(field, 'spacer')?.height || 24 }} />
    default:
      return null
  }
}

function FieldInput({
  field,
  value,
  onChange,
  onBlur,
  autoFocus,
  id,
  errorId,
  hasError,
  isRequired,
}: {
  field: Field
  value: unknown
  onChange: (v: unknown) => void
  onBlur?: () => void
  autoFocus?: boolean
  id?: string
  errorId?: string
  hasError?: boolean
  isRequired?: boolean
}) {
  const h = field.height || 1
  const ariaProps = {
    id,
    'aria-invalid': hasError || undefined,
    'aria-describedby': errorId || undefined,
    'aria-required': isRequired || undefined,
  }

  switch (field.field_type) {
    case 'textarea':
      return (
        <Textarea
          {...ariaProps}
          value={(value as string) || ''}
          onChange={(e) => onChange(e.target.value)}
          onBlur={onBlur}
          rows={getFieldOptions(field, 'textarea')?.rows || Math.max(4, h * 2)}
          required={field.is_required}
          autoFocus={autoFocus}
        />
      )
    case 'time':
      return (
        <Input
          {...ariaProps}
          type="time"
          value={(value as string) || ''}
          onChange={(e) => onChange(e.target.value)}
          onBlur={onBlur}
          required={field.is_required}
        />
      )
    case 'text': {
      const textDisplay = getDisplayType(field)
      if (h > 1) {
        return (
          <Textarea
            {...ariaProps}
            value={(value as string) || ''}
            onChange={(e) => onChange(e.target.value)}
            onBlur={onBlur}
            required={field.is_required}
            rows={h * 2}
          />
        )
      }
      const inputType = textDisplay === 'email' ? 'email'
        : textDisplay === 'url' ? 'url'
        : textDisplay === 'phone' ? 'tel'
        : 'text'
      return (
        <Input
          {...ariaProps}
          type={inputType}
          value={(value as string) || ''}
          onChange={(e) => onChange(e.target.value)}
          onBlur={onBlur}
          required={field.is_required}
          autoFocus={autoFocus}
        />
      )
    }
    case 'number':
    case 'integer': {
      const displayType = getDisplayType(field)
      const numOpts2 = getFieldOptions(field, 'number')
      if (displayType === 'rating') {
        const max = numOpts2?.max_rating || 5
        const current = (value as number) || 0
        return (
          <div className="flex items-center gap-1 pt-1">
            {Array.from({ length: max }, (_, i) => (
              <button
                key={i}
                type="button"
                className={`text-lg ${i < current ? 'text-yellow-500' : 'text-muted-foreground/30'}`}
                onClick={() => onChange(i + 1 === current ? 0 : i + 1)}
              >
                ★
              </button>
            ))}
          </div>
        )
      }
      if (displayType === 'progress') {
        const num = (value as number) ?? 0
        return (
          <div className="space-y-1">
            <Input
              {...ariaProps}
              type="number"
              min={0}
              max={100}
              value={value === null || value === undefined ? '' : num}
              onChange={(e) => onChange(e.target.value === '' ? null : Number(e.target.value))}
              onBlur={onBlur}
              required={field.is_required}
            />
            <div className="h-2 w-full overflow-hidden rounded-full bg-muted">
              <div
                className="h-full rounded-full bg-primary transition-all"
                style={{ width: `${Math.min(100, Math.max(0, num))}%` }}
              />
            </div>
          </div>
        )
      }
      const prefix = displayType === 'currency'
        ? numOpts2?.currency_code === 'USD' ? '$' : '₩'
        : undefined
      const suffix = displayType === 'percent' ? '%' : undefined
      return (
        <div className="relative">
          {prefix && (
            <span className="absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
              {prefix}
            </span>
          )}
          <Input
            {...ariaProps}
            type="number"
            value={value === null || value === undefined ? '' : (value as number)}
            onChange={(e) => onChange(e.target.value === '' ? null : Number(e.target.value))}
            onBlur={onBlur}
            required={field.is_required}
            className={prefix ? 'pl-8' : suffix ? 'pr-8' : ''}
          />
          {suffix && (
            <span className="absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
              {suffix}
            </span>
          )}
        </div>
      )
    }
    case 'date':
      return (
        <DatePicker
          value={(value as string)?.slice(0, 10) || undefined}
          onChange={(v) => onChange(v ?? null)}
          placeholder="날짜 선택"
        />
      )
    case 'datetime':
      return (
        <Input
          {...ariaProps}
          type="datetime-local"
          value={(value as string)?.slice(0, 16) || ''}
          onChange={(e) => onChange(e.target.value)}
          onBlur={onBlur}
          required={field.is_required}
        />
      )
    case 'boolean':
      return (
        <div className="flex items-center gap-2 pt-1">
          <Checkbox id={id} aria-required={isRequired || undefined} checked={!!value} onCheckedChange={(c) => onChange(!!c)} />
        </div>
      )
    case 'select': {
      const choices = getChoices(field)
      const display = getFieldOptions(field, 'select')?.display
      if (display === 'radio') {
        return (
          <div className="space-y-1">
            {choices.map((c) => (
              <label key={c} className="flex items-center gap-2 text-sm">
                <input
                  type="radio"
                  name={field.slug}
                  value={c}
                  checked={value === c}
                  onChange={() => onChange(c)}
                />
                {c}
              </label>
            ))}
          </div>
        )
      }
      return (
        <Select value={(value as string) || ''} onValueChange={onChange}>
          <SelectTrigger {...ariaProps} onBlur={onBlur}>
            <SelectValue placeholder="항목 선택" />
          </SelectTrigger>
          <SelectContent>
            {choices.map((c) => (
              <SelectItem key={c} value={c}>
                {c}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )
    }
    case 'multiselect': {
      const choices = getChoices(field)
      const selected = (value as string[]) || []
      return (
        <Popover>
          <PopoverTrigger
            className="flex min-h-9 w-full flex-wrap items-center gap-1 rounded-md border border-input bg-transparent px-3 py-1.5 text-sm shadow-xs"
          >
            {selected.length === 0 && (
              <span className="text-muted-foreground">선택...</span>
            )}
            {selected.map((v) => (
              <Badge
                key={v}
                variant="secondary"
                className="gap-0.5"
                onClick={(e) => {
                  e.stopPropagation()
                  onChange(selected.filter((x) => x !== v))
                }}
              >
                {v}
                <X className="h-3 w-3 cursor-pointer" />
              </Badge>
            ))}
          </PopoverTrigger>
          <PopoverContent align="start" className="max-h-60 overflow-y-auto p-1">
            {choices.map((c) => (
              <label
                key={c}
                className="flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-accent"
              >
                <Checkbox
                  checked={selected.includes(c)}
                  onCheckedChange={(checked) => {
                    if (checked) onChange([...selected, c])
                    else onChange(selected.filter((x) => x !== c))
                  }}
                />
                {c}
              </label>
            ))}
          </PopoverContent>
        </Popover>
      )
    }
    case 'relation':
      if (!field.relation?.target_collection_id) {
        return <Input disabled value="(관계 대상 미설정)" />
      }
      if (field.relation?.relation_type === 'many_to_many') {
        return (
          <RelationMultiCombobox
            targetCollectionId={field.relation.target_collection_id}
            value={(value as string[]) || []}
            onChange={onChange}
          />
        )
      }
      return (
        <RelationCombobox
          targetCollectionId={field.relation.target_collection_id}
          value={value as string | undefined}
          onChange={onChange}
        />
      )
    case 'autonumber':
      return (
        <Input
          value={value != null ? String(value) : '(자동 생성)'}
          disabled
          className="bg-muted"
        />
      )
    case 'formula':
    case 'lookup':
    case 'rollup':
      return (
        <Input
          value={value != null ? String(value) : '(자동 계산)'}
          disabled
          className="bg-muted"
        />
      )
    case 'user':
      return <UserCombobox value={value as string | undefined} onChange={onChange} />
    case 'file':
      return <FileInput value={value as string | undefined} onChange={onChange} />
    case 'table':
      return <TableAreaInput field={field} value={value} onChange={onChange} />
    case 'spreadsheet':
      return <SpreadsheetInput field={field} value={value} onChange={onChange} />
    case 'json':
      return (
        <Textarea
          value={typeof value === 'string' ? value : JSON.stringify(value || {}, null, 2)}
          onChange={(e) => {
            try {
              onChange(JSON.parse(e.target.value))
            } catch {
              onChange(e.target.value)
            }
          }}
          onBlur={onBlur}
          rows={Math.max(4, h * 2)}
        />
      )
    default:
      return (
        <Input value={(value as string) || ''} onChange={(e) => onChange(e.target.value)} onBlur={onBlur} autoFocus={autoFocus} />
      )
  }
}

// -- TableAreaInput: inline repeating table within the form --


function TableAreaInput({
  field,
  value,
  onChange,
}: {
  field: Field
  value: unknown
  onChange: (v: unknown) => void
}) {
  const subColumns: SubColumn[] = getFieldOptions(field, 'table')?.sub_columns || [
    { key: 'col1', label: '항목', type: 'text' },
    { key: 'col2', label: '값', type: 'text' },
  ]
  const rows = Array.isArray(value) ? (value as Record<string, unknown>[]) : []

  function updateRow(rowIdx: number, key: string, val: unknown) {
    const next = rows.map((r, i) => (i === rowIdx ? { ...r, [key]: val } : r))
    onChange(next)
  }

  function addRow() {
    const empty: Record<string, unknown> = {}
    for (const col of subColumns) {
      empty[col.key] = col.type === 'number' ? null : ''
    }
    onChange([...rows, empty])
  }

  function removeRow(idx: number) {
    onChange(rows.filter((_, i) => i !== idx))
  }

  return (
    <div className="space-y-2">
      <div className="rounded-md border overflow-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b bg-muted/50">
              <th className="px-2 py-1.5 text-left text-xs font-medium text-muted-foreground w-8">#</th>
              {subColumns.map((col) => (
                <th key={col.key} className="px-2 py-1.5 text-left text-xs font-medium text-muted-foreground">
                  {col.label}
                </th>
              ))}
              <th className="px-2 py-1.5 w-8" />
            </tr>
          </thead>
          <tbody>
            {rows.map((row, rowIdx) => (
              <tr key={rowIdx} className="border-b last:border-b-0">
                <td className="px-2 py-1 text-xs text-muted-foreground">{rowIdx + 1}</td>
                {subColumns.map((col) => (
                  <td key={col.key} className="px-1 py-0.5">
                    {col.type === 'select' ? (
                      <select
                        className="h-7 w-full rounded border border-input bg-transparent px-1 text-sm"
                        value={(row[col.key] as string) || ''}
                        onChange={(e) => updateRow(rowIdx, col.key, e.target.value)}
                      >
                        <option value="">선택</option>
                        {(col.choices || []).map((c) => (
                          <option key={c} value={c}>{c}</option>
                        ))}
                      </select>
                    ) : (
                      <Input
                        type={col.type === 'number' ? 'number' : 'text'}
                        className="h-7 text-sm"
                        value={row[col.key] != null ? String(row[col.key]) : ''}
                        onChange={(e) =>
                          updateRow(
                            rowIdx,
                            col.key,
                            col.type === 'number'
                              ? e.target.value === '' ? null : Number(e.target.value)
                              : e.target.value,
                          )
                        }
                      />
                    )}
                  </td>
                ))}
                <td className="px-1 py-0.5">
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-7 w-7 p-0 text-muted-foreground hover:text-destructive"
                    onClick={() => removeRow(rowIdx)}
                  >
                    ×
                  </Button>
                </td>
              </tr>
            ))}
            {rows.length === 0 && (
              <tr>
                <td colSpan={subColumns.length + 2} className="px-2 py-3 text-center text-xs text-muted-foreground">
                  행을 추가하세요
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      <Button type="button" variant="outline" size="sm" onClick={addRow}>
        + 행 추가
      </Button>
    </div>
  )
}

function FileInput({
  value,
  onChange,
}: {
  value: string | undefined
  onChange: (v: unknown) => void
}) {
  const [uploading, setUploading] = useState(false)

  async function handleFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    e.target.value = ''
    if (file.size > 50 * 1024 * 1024) {
      toast.error('파일은 50MB 이하여야 합니다')
      return
    }
    setUploading(true)
    try {
      const result = await api.upload(file)
      onChange(result.url)
    } catch (err) {
      toast.error(formatError(err))
    } finally {
      setUploading(false)
    }
  }

  return (
    <div className="space-y-1">
      <Input type="file" onChange={handleFile} disabled={uploading} />
      {uploading && <p className="text-xs text-muted-foreground">업로드 중...</p>}
      {value && !uploading && (
        <a
          href={value}
          target="_blank"
          rel="noopener noreferrer"
          className="text-xs text-primary underline"
        >
          {value.split('/').pop()}
        </a>
      )}
    </div>
  )
}

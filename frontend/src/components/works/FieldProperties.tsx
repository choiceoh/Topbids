/**
 * FieldProperties — Schema editor panel for configuring a single field.
 *
 * Rendered in the right panel of AppBuilderPage when a field is selected.
 *
 * Features:
 * - Field type selector with sensible defaults per type
 * - Type-specific option panels (select choices, relation config, formula editor, etc.)
 * - AI-powered slug generation from the field label via useAIGenerateSlug
 * - Collision detection: appends _2, _3, ... suffix when a slug already exists
 * - Basic / Advanced tab split to avoid overwhelming new users
 * - Lookup and rollup configuration with relation field chaining
 */
import { useCallback, useRef, useState } from 'react'
import { GripVertical, Loader2, Plus, X } from 'lucide-react'

import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { Textarea } from '@/components/ui/textarea'
import {
  FIELD_TYPE_LABELS,
  HEIGHT_OPTIONS,
  isComputedType,
  isLayoutType,
  NUMBER_DISPLAY_TYPES,
  ON_DELETE_OPTIONS,
  RELATION_TYPE_OPTIONS,
  ROLLUP_FUNCTIONS,
  TEXT_DISPLAY_TYPES,
  VALIDATION_OPTIONS,
  WIDTH_OPTIONS,
} from '@/lib/constants'
import type { Collection, Field, FieldType } from '@/lib/types'

import { useAIAvailable } from '@/contexts/AIAvailabilityContext'
import { useAIGenerateSlug } from '@/hooks/useAI'

import type { FieldDraft } from './FieldPreview'
import FormulaEditor from './FormulaEditor'

interface Props {
  field: FieldDraft | null
  collections: Collection[]
  siblingFields?: FieldDraft[]
  onChange: (field: FieldDraft) => void
  /** Collection slug — needed for formula preview API. */
  collectionSlug?: string
  /** All fields in the collection — needed for formula field reference hints. */
  allFields?: FieldDraft[]
}

export default function FieldProperties({ field, collections, siblingFields, onChange, collectionSlug, allFields }: Props) {
  const aiAvailable = useAIAvailable()
  const generateSlug = useAIGenerateSlug()
  const slugDebounceRef = useRef<ReturnType<typeof setTimeout>>(null)
  const [tab, setTab] = useState<'basic' | 'advanced'>('basic')

  const requestAutoSlug = useCallback((label: string, currentField: FieldDraft) => {
    if (slugDebounceRef.current) clearTimeout(slugDebounceRef.current)
    if (!label.trim() || !aiAvailable || currentField.slug) return
    slugDebounceRef.current = setTimeout(() => {
      generateSlug.mutate(label.trim(), {
        onSuccess: (res) => {
          let slug = res.slug
          const taken = new Set((siblingFields ?? []).filter((f) => f.id !== currentField.id).map((f) => f.slug))
          if (taken.has(slug)) {
            let n = 2
            while (taken.has(`${slug}_${n}`)) n++
            slug = `${slug}_${n}`
          }
          onChange({ ...currentField, label, slug })
        },
      })
    }, 500)
  }, [aiAvailable, generateSlug, onChange, siblingFields])

  if (!field) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        항목을 선택하세요
      </div>
    )
  }

  function update(patch: Partial<FieldDraft>) {
    onChange({ ...field!, ...patch })
  }

  function updateOption(key: string, value: unknown) {
    onChange({ ...field!, options: { ...field!.options, [key]: value } })
  }

  const opts = field.options || {}
  const selectChoices = (opts.choices as string[]) || []
  const isLayout = isLayoutType(field.field_type)
  const isText = field.field_type === 'text' || field.field_type === 'textarea'
  const isNumeric = field.field_type === 'number' || field.field_type === 'integer'
  const isSelect = field.field_type === 'select' || field.field_type === 'multiselect'
  const isRelation = field.field_type === 'relation'
  const isBoolean = field.field_type === 'boolean'
  const isDate = field.field_type === 'date' || field.field_type === 'datetime' || field.field_type === 'time'
  const isComputed = isComputedType(field.field_type)
  const isFormula = field.field_type === 'formula'
  const isLookup = field.field_type === 'lookup'
  const isRollup = field.field_type === 'rollup'

  // For lookup/rollup: find relation fields among sibling fields
  const relationFields = (siblingFields ?? []).filter(
    (f) => f.field_type === 'relation',
  )

  // For lookup/rollup: find target collection fields
  const selectedRelationField = relationFields.find(
    (f) => f.slug === (opts.relation_field as string),
  )
  const targetCollectionId = selectedRelationField?.relation?.target_collection_id
  const targetCollection = collections.find((c) => c.id === targetCollectionId)
  const targetFields = targetCollection?.fields?.filter(
    (f: Field) => !isLayoutType(f.field_type) && !isComputedType(f.field_type),
  ) ?? []

  // Group variant options
  type VariantOption = { value: FieldType; label: string }
  const TEXT_VARIANTS: VariantOption[] = [
    { value: 'text', label: '텍스트' },
    { value: 'textarea', label: '긴 글' },
  ]
  const NUMBER_VARIANTS: VariantOption[] = [
    { value: 'number', label: '숫자' },
    { value: 'integer', label: '정수' },
  ]
  const DATE_VARIANTS: VariantOption[] = [
    { value: 'date', label: '날짜' },
    { value: 'time', label: '시간' },
    { value: 'datetime', label: '일시' },
  ]
  const SELECT_VARIANTS: VariantOption[] = [
    { value: 'select', label: '단일 선택' },
    { value: 'multiselect', label: '다중 선택' },
  ]

  const variantGroup: VariantOption[] | null = isText
    ? TEXT_VARIANTS
    : isNumeric
      ? NUMBER_VARIANTS
      : isDate
        ? DATE_VARIANTS
        : isSelect
          ? SELECT_VARIANTS
          : null

  // Layout fields: show minimal properties only
  if (isLayout) {
    return (
      <div className="space-y-4 overflow-y-auto">
        <div className="flex items-center gap-2">
          <span className="rounded bg-muted px-1.5 py-0.5 text-xs font-medium">
            {FIELD_TYPE_LABELS[field.field_type]}
          </span>
          <h3 className="text-sm font-medium">속성</h3>
        </div>
        <section className="space-y-2">
          <Label className="text-xs font-semibold text-muted-foreground">이름</Label>
          <Input value={field.label} onChange={(e) => {
            update({ label: e.target.value })
            requestAutoSlug(e.target.value, { ...field, label: e.target.value })
          }} />
        </section>
        <Separator />
        <section className="space-y-2">
          <Label className="text-xs font-semibold text-muted-foreground">영문 ID</Label>
          <div className="relative">
            <Input
              value={field.slug}
              onChange={(e) => update({ slug: e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, '') })}
              placeholder="자동 생성됨"
            />
            {generateSlug.isPending && (
              <Loader2 className="absolute top-1/2 right-3 h-4 w-4 -translate-y-1/2 animate-spin text-muted-foreground" />
            )}
          </div>
        </section>
        {field.field_type === 'label' && (
          <>
            <Separator />
            <section className="space-y-2">
              <Label className="text-xs font-semibold text-muted-foreground">표시 텍스트</Label>
              <Textarea
                rows={3}
                value={(opts.content as string) || ''}
                onChange={(e) => updateOption('content', e.target.value)}
                placeholder="폼에 표시할 안내 문구"
              />
            </section>
          </>
        )}
        {field.field_type === 'spacer' && (
          <>
            <Separator />
            <section className="space-y-2">
              <Label className="text-xs font-semibold text-muted-foreground">높이 (px)</Label>
              <Input
                type="number"
                value={(opts.height as number) || 24}
                onChange={(e) => updateOption('height', Number(e.target.value) || 24)}
              />
            </section>
          </>
        )}
      </div>
    )
  }

  // Formula fields: show dedicated editor instead of standard properties.
  if (isFormula) {
    const formulaFields = (allFields ?? []).filter(
      (f) => !['label', 'line', 'spacer', 'formula'].includes(f.field_type),
    )
    return (
      <div className="space-y-4 overflow-y-auto">
        <div className="flex items-center gap-2">
          <span className="rounded bg-muted px-1.5 py-0.5 text-xs font-medium">
            {FIELD_TYPE_LABELS[field.field_type]}
          </span>
          <h3 className="text-sm font-medium">속성</h3>
        </div>

        <section className="space-y-2">
          <Label className="text-xs font-semibold text-muted-foreground">이름</Label>
          <Input value={field.label} onChange={(e) => {
            update({ label: e.target.value })
            requestAutoSlug(e.target.value, { ...field, label: e.target.value })
          }} />
        </section>

        <Separator />

        <FormulaEditor
          expression={(opts.expression as string) || ''}
          resultType={(opts.result_type as string) || 'number'}
          precision={opts.precision as number | undefined}
          slug={collectionSlug}
          fields={Array.isArray(formulaFields) ? (formulaFields as unknown as Field[]) : []}
          onChange={(expr) => updateOption('expression', expr)}
          onResultTypeChange={(rt) => updateOption('result_type', rt)}
          onPrecisionChange={(p) => updateOption('precision', p)}
        />

        <Separator />

        <section className="space-y-2">
          <Label className="text-xs font-semibold text-muted-foreground">그리드 크기</Label>
          <div className="grid grid-cols-2 gap-2">
            <div className="space-y-1">
              <Label className="text-xs">폭</Label>
              <Select
                value={String(field.width)}
                onValueChange={(v) => update({ width: Number(v) })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {WIDTH_OPTIONS.map((o) => (
                    <SelectItem key={o.value} value={String(o.value)}>
                      {o.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label className="text-xs">높이</Label>
              <Select
                value={String(field.height)}
                onValueChange={(v) => update({ height: Number(v) })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {HEIGHT_OPTIONS.map((o) => (
                    <SelectItem key={o.value} value={String(o.value)}>
                      {o.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
        </section>

        <Separator />

        <section className="space-y-2">
          <Label className="text-xs font-semibold text-muted-foreground">코드</Label>
          <div className="relative">
            <Input
              value={field.slug}
              onChange={(e) => update({ slug: e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, '') })}
              placeholder="자동 생성됨"
            />
            {generateSlug.isPending && (
              <Loader2 className="absolute top-1/2 right-3 h-4 w-4 -translate-y-1/2 animate-spin text-muted-foreground" />
            )}
          </div>
        </section>
      </div>
    )
  }

  return (
    <div className="overflow-y-auto">
      {/* ── 헤더: 타입 뱃지 + 탭 전환 ── */}
      <div className="mb-3 flex items-center gap-2">
        <span className="rounded bg-muted px-1.5 py-0.5 text-xs font-medium">
          {FIELD_TYPE_LABELS[field.field_type]}
        </span>
        <div className="flex items-center gap-1 rounded-lg border p-1">
          <button
            type="button"
            className={`rounded-md px-3 py-1 text-sm font-medium transition-colors ${
              tab === 'basic'
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:bg-muted'
            }`}
            onClick={() => setTab('basic')}
          >
            속성
          </button>
          <button
            type="button"
            className={`rounded-md px-3 py-1 text-sm font-medium transition-colors ${
              tab === 'advanced'
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:bg-muted'
            }`}
            onClick={() => setTab('advanced')}
          >
            고급
          </button>
        </div>
      </div>

      {/* ══════════════ 속성 탭 ══════════════ */}
      {tab === 'basic' && (
        <div className="space-y-3">
          {/* 이름 */}
          <section className="space-y-2">
            <Label className="text-xs font-semibold text-muted-foreground">이름</Label>
            <Input
              value={field.label}
              onChange={(e) => {
                update({ label: e.target.value })
                requestAutoSlug(e.target.value, { ...field, label: e.target.value })
              }}
              placeholder="항목 이름"
            />
          </section>

          {/* 유형 전환 */}
          {variantGroup && (
            <section className="space-y-2">
              <Label className="text-xs font-semibold text-muted-foreground">유형</Label>
              <Select
                value={field.field_type}
                onValueChange={(v) => update({ field_type: v as FieldType })}
              >
                <SelectTrigger>
                  <SelectValue>{variantGroup.find(v => v.value === field.field_type)?.label}</SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {variantGroup.map((o) => (
                    <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </section>
          )}

          {/* 필수 */}
          {!isComputed && (
            <section className="space-y-2">
              <div className="flex items-center gap-2">
                <Checkbox
                  id="required"
                  checked={field.is_required}
                  onCheckedChange={(c) => update({ is_required: !!c })}
                />
                <Label htmlFor="required" className="text-xs">필수 입력 컴포넌트</Label>
              </div>
            </section>
          )}

          {/* 폭/높이 */}
          <section className="space-y-2">
            <div className="grid grid-cols-2 gap-2">
              <div className="space-y-1">
                <Label className="text-xs font-semibold text-muted-foreground">폭</Label>
                <Select
                  value={String(field.width)}
                  onValueChange={(v) => update({ width: Number(v) })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {WIDTH_OPTIONS.map((o) => (
                      <SelectItem key={o.value} value={String(o.value)}>
                        {o.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label className="text-xs font-semibold text-muted-foreground">높이</Label>
                <Select
                  value={String(field.height)}
                  onValueChange={(v) => update({ height: Number(v) })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {HEIGHT_OPTIONS.map((o) => (
                      <SelectItem key={o.value} value={String(o.value)}>
                        {o.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          </section>

          {/* 선택 옵션 — select/multiselect */}
          {isSelect && (
            <section className="space-y-2">
              <Label className="text-xs font-semibold text-muted-foreground">선택 옵션 (줄바꿈)</Label>
              <Textarea
                rows={4}
                value={selectChoices.join('\n')}
                onChange={(e) => {
                  const choices = e.target.value.split('\n').map((s) => s.trim()).filter(Boolean)
                  updateOption('choices', choices)
                }}
                placeholder="옵션1&#10;옵션2&#10;옵션3"
              />
            </section>
          )}

          {/* 테이블/스프레드시트 열 설정 */}
          {(field.field_type === 'table' || field.field_type === 'spreadsheet') && (
            <SubColumnEditor
              columns={(opts.sub_columns as SubColumnDef[]) || []}
              onChange={(cols) => updateOption('sub_columns', cols)}
            />
          )}

          {/* 스프레드시트 초기 행 수 */}
          {field.field_type === 'spreadsheet' && (
            <section className="space-y-1">
              <Label className="text-xs font-semibold text-muted-foreground">초기 행 수</Label>
              <Input
                type="number"
                min={1}
                max={100}
                className="h-7 w-24 text-sm"
                value={(opts.initial_rows as number) || 5}
                onChange={(e) => updateOption('initial_rows', Math.max(1, Math.min(100, Number(e.target.value) || 5)))}
              />
            </section>
          )}

          {/* 연결 설정 */}
          {isRelation && (
            <section className="space-y-3 rounded-md border bg-muted/30 p-3">
              <Label className="text-xs font-semibold text-muted-foreground">연결 설정</Label>
              <div className="space-y-1">
                <Label className="text-xs">대상 앱</Label>
                <Select
                  value={field.relation?.target_collection_id || ''}
                  onValueChange={(v) =>
                    v &&
                    update({
                      relation: {
                        target_collection_id: v,
                        relation_type: field.relation?.relation_type || 'one_to_many',
                        on_delete: field.relation?.on_delete || 'SET NULL',
                      },
                    })
                  }
                >
                  <SelectTrigger>
                    <SelectValue placeholder="선택" />
                  </SelectTrigger>
                  <SelectContent>
                    {collections.map((c) => (
                      <SelectItem key={c.id} value={c.id}>{c.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label className="text-xs">연결 유형</Label>
                <Select
                  value={field.relation?.relation_type || 'one_to_many'}
                  onValueChange={(v) =>
                    v &&
                    field.relation &&
                    update({
                      relation: { ...field.relation, relation_type: v as 'one_to_one' | 'one_to_many' | 'many_to_many' },
                    })
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {RELATION_TYPE_OPTIONS.map((o) => (
                      <SelectItem key={o.value} value={o.value} label={o.label}>
                        <div className="flex flex-col items-start whitespace-normal">
                          <span>{o.label}</span>
                          <span className="text-xs text-muted-foreground">{o.description}</span>
                        </div>
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </section>
          )}

          {/* 참조값 설정 */}
          {isLookup && (
            <section className="space-y-3 rounded-md border bg-muted/30 p-3">
              <Label className="text-xs font-semibold text-muted-foreground">참조값 설정</Label>
              <div className="space-y-1">
                <Label className="text-xs">연결 항목</Label>
                <Select
                  value={(opts.relation_field as string) || ''}
                  onValueChange={(v) => updateOption('relation_field', v)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="연결 항목 선택" />
                  </SelectTrigger>
                  <SelectContent>
                    {relationFields.map((f) => (
                      <SelectItem key={f.slug} value={f.slug}>{f.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label className="text-xs">참조할 항목</Label>
                <Select
                  value={(opts.target_field as string) || ''}
                  onValueChange={(v) => updateOption('target_field', v)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="항목 선택" />
                  </SelectTrigger>
                  <SelectContent>
                    {targetFields.map((f: Field) => (
                      <SelectItem key={f.slug} value={f.slug}>{f.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </section>
          )}

          {/* 요약 계산 설정 */}
          {isRollup && (
            <section className="space-y-3 rounded-md border bg-muted/30 p-3">
              <Label className="text-xs font-semibold text-muted-foreground">요약 계산 설정</Label>
              <div className="space-y-1">
                <Label className="text-xs">연결 항목</Label>
                <Select
                  value={(opts.relation_field as string) || ''}
                  onValueChange={(v) => updateOption('relation_field', v)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="연결 항목 선택" />
                  </SelectTrigger>
                  <SelectContent>
                    {relationFields.map((f) => (
                      <SelectItem key={f.slug} value={f.slug}>{f.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label className="text-xs">계산할 항목</Label>
                <Select
                  value={(opts.target_field as string) || ''}
                  onValueChange={(v) => updateOption('target_field', v)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="항목 선택" />
                  </SelectTrigger>
                  <SelectContent>
                    {targetFields.map((f: Field) => (
                      <SelectItem key={f.slug} value={f.slug}>{f.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label className="text-xs">계산 방식</Label>
                <Select
                  value={(opts.function as string) || 'SUM'}
                  onValueChange={(v) => updateOption('function', v)}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {ROLLUP_FUNCTIONS.map((o) => (
                      <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </section>
          )}
        </div>
      )}

      {/* ══════════════ 고급 탭 ══════════════ */}
      {tab === 'advanced' && (
        <div className="space-y-3">
          {/* 이름숨기기 */}
          <section className="space-y-2">
            <div className="flex items-center gap-2">
              <Checkbox
                id="hide_label"
                checked={!!opts.hide_label}
                onCheckedChange={(c) => updateOption('hide_label', !!c)}
              />
              <Label htmlFor="hide_label" className="text-xs">이름숨기기</Label>
            </div>
          </section>

          {/* 설명 */}
          <section className="space-y-2">
            <Label className="text-xs font-semibold text-muted-foreground">설명</Label>
            <Input
              value={field.description || ''}
              onChange={(e) => update({ description: e.target.value })}
              placeholder="설명을 입력해주세요."
            />
            <div className="flex items-center gap-2">
              <Checkbox
                id="show_tooltip"
                checked={!!opts.show_tooltip}
                onCheckedChange={(c) => updateOption('show_tooltip', !!c)}
              />
              <Label htmlFor="show_tooltip" className="text-xs">툴팁으로 표현</Label>
            </div>
          </section>

          {/* 유니크/인덱스 */}
          {!isComputed && (
            <section className="space-y-2">
              <div className="flex items-center gap-2">
                <Checkbox
                  id="unique"
                  checked={field.is_unique}
                  onCheckedChange={(c) => update({ is_unique: !!c })}
                />
                <Label htmlFor="unique" className="text-xs">중복 입력값 등록 불가</Label>
              </div>
              <div className="flex items-center gap-2">
                <Checkbox
                  id="indexed"
                  checked={field.is_indexed}
                  onCheckedChange={(c) => update({ is_indexed: !!c })}
                />
                <Label htmlFor="indexed" className="text-xs">인덱스 (검색 최적화)</Label>
              </div>
            </section>
          )}

          {/* 기본값 */}
          {!isRelation && !isComputed && (
            <section className="space-y-2">
              <Label className="text-xs font-semibold text-muted-foreground">기본값</Label>
              {isBoolean ? (
                <Select
                  value={field.default_value ?? ''}
                  onValueChange={(v) => update({ default_value: v || undefined })}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="없음" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="true">참 (true)</SelectItem>
                    <SelectItem value="false">거짓 (false)</SelectItem>
                  </SelectContent>
                </Select>
              ) : isSelect ? (
                <Select
                  value={field.default_value ?? ''}
                  onValueChange={(v) => update({ default_value: v || undefined })}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="없음" />
                  </SelectTrigger>
                  <SelectContent>
                    {selectChoices.map((c) => (
                      <SelectItem key={c} value={c}>{c}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <Input
                  value={field.default_value ?? ''}
                  onChange={(e) => update({ default_value: e.target.value || undefined })}
                  placeholder="기본값 없음"
                  type={isNumeric ? 'number' : isDate ? 'date' : 'text'}
                />
              )}
            </section>
          )}

          {/* 최소/최대 입력수 (text) */}
          {isText && (
            <section className="space-y-2">
              <div className="grid grid-cols-2 gap-2">
                <div className="space-y-1">
                  <Label className="text-xs font-semibold text-muted-foreground">최소 입력수</Label>
                  <Input
                    type="number"
                    min={0}
                    value={(opts.min_length as number) ?? 0}
                    onChange={(e) => updateOption('min_length', Number(e.target.value))}
                  />
                </div>
                <div className="space-y-1">
                  <Label className="text-xs font-semibold text-muted-foreground">최대 입력수</Label>
                  <Input
                    type="number"
                    min={0}
                    value={(opts.max_length as number) ?? 100}
                    onChange={(e) => updateOption('max_length', Number(e.target.value))}
                  />
                </div>
              </div>
            </section>
          )}

          {/* 표시 형식 (number) */}
          {isNumeric && (
            <section className="space-y-2">
              <Label className="text-xs font-semibold text-muted-foreground">표시 형식</Label>
              <Select
                value={(opts.display_type as string) || 'plain'}
                onValueChange={(v) => updateOption('display_type', v === 'plain' ? undefined : v)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {NUMBER_DISPLAY_TYPES.map((o) => (
                    <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {opts.display_type === 'currency' && (
                <Input
                  value={(opts.currency_code as string) || 'KRW'}
                  onChange={(e) => updateOption('currency_code', e.target.value.toUpperCase())}
                  placeholder="KRW"
                  maxLength={3}
                />
              )}
            </section>
          )}

          {/* 최소/최대값 (number) */}
          {isNumeric && (
            <section className="space-y-2">
              <div className="grid grid-cols-2 gap-2">
                <div className="space-y-1">
                  <Label className="text-xs font-semibold text-muted-foreground">최소값</Label>
                  <Input
                    type="number"
                    value={(opts.min_value as number) ?? ''}
                    onChange={(e) => updateOption('min_value', e.target.value ? Number(e.target.value) : undefined)}
                    placeholder="제한 없음"
                  />
                </div>
                <div className="space-y-1">
                  <Label className="text-xs font-semibold text-muted-foreground">최대값</Label>
                  <Input
                    type="number"
                    value={(opts.max_value as number) ?? ''}
                    onChange={(e) => updateOption('max_value', e.target.value ? Number(e.target.value) : undefined)}
                    placeholder="제한 없음"
                  />
                </div>
              </div>
            </section>
          )}

          {/* 표시 방식 (select) */}
          {field.field_type === 'select' && (
            <section className="space-y-2">
              <Label className="text-xs font-semibold text-muted-foreground">표시 방식</Label>
              <Select
                value={(opts.display as string) || 'dropdown'}
                onValueChange={(v) => updateOption('display', v)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="dropdown">드롭다운</SelectItem>
                  <SelectItem value="radio">라디오 버튼</SelectItem>
                </SelectContent>
              </Select>
            </section>
          )}

          {/* 입력 너비 조절 */}
          <section className="space-y-2">
            <Label className="text-xs font-semibold text-muted-foreground">입력 너비 조절</Label>
            <div className="flex items-center gap-2">
              <Input
                type="number"
                min={0}
                className="w-20"
                value={(opts.input_width as number) ?? 100}
                onChange={(e) => updateOption('input_width', Number(e.target.value))}
              />
              <div className="flex items-center gap-1 rounded-md border p-0.5">
                <button
                  type="button"
                  className={`rounded px-2 py-0.5 text-xs transition-colors ${
                    (opts.input_width_unit || '%') === 'px'
                      ? 'bg-primary text-primary-foreground'
                      : 'text-muted-foreground hover:bg-muted'
                  }`}
                  onClick={() => updateOption('input_width_unit', 'px')}
                >
                  PX
                </button>
                <button
                  type="button"
                  className={`rounded px-2 py-0.5 text-xs transition-colors ${
                    (opts.input_width_unit || '%') === '%'
                      ? 'bg-primary text-primary-foreground'
                      : 'text-muted-foreground hover:bg-muted'
                  }`}
                  onClick={() => updateOption('input_width_unit', '%')}
                >
                  %
                </button>
              </div>
            </div>
            {(opts.input_width_unit || '%') === '%' && (
              <p className="text-xs text-muted-foreground">* 퍼센트(%) 입력시 비율로 지정</p>
            )}
          </section>

          {/* 표시 형식 (text) */}
          {field.field_type === 'text' && (
            <section className="space-y-2">
              <Label className="text-xs font-semibold text-muted-foreground">표시 형식</Label>
              <Select
                value={(opts.display_type as string) || 'plain'}
                onValueChange={(v) => {
                  const dt = v === 'plain' ? undefined : v
                  const validation = dt && dt !== 'plain' ? dt : opts.validation
                  onChange({ ...field!, options: { ...opts, display_type: dt, validation } })
                }}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TEXT_DISPLAY_TYPES.map((o) => (
                    <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </section>
          )}

          {/* 입력값 유효성 체크 (text) */}
          {isText && (
            <section className="space-y-2">
              <Label className="text-xs font-semibold text-muted-foreground">입력값 유효성 체크</Label>
              <Select
                value={(opts.validation as string) || 'none'}
                onValueChange={(v) => updateOption('validation', v === 'none' ? undefined : v)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {VALIDATION_OPTIONS.map((o) => (
                    <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {opts.validation === 'regex' && (
                <Input
                  value={(opts.validation_regex as string) || ''}
                  onChange={(e) => updateOption('validation_regex', e.target.value)}
                  placeholder="정규식 패턴"
                />
              )}
            </section>
          )}

          {/* 연결 삭제 동작 */}
          {isRelation && field.relation && (
            <section className="space-y-2">
              <Label className="text-xs font-semibold text-muted-foreground">삭제 시 동작</Label>
              <Select
                value={field.relation.on_delete || 'SET NULL'}
                onValueChange={(v) =>
                  v &&
                  field.relation &&
                  update({ relation: { ...field.relation, on_delete: v } })
                }
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {ON_DELETE_OPTIONS.map((o) => (
                    <SelectItem key={o.value} value={o.value} label={o.label}>
                      <div className="flex flex-col items-start whitespace-normal">
                        <span>{o.label}</span>
                        <span className="text-xs text-muted-foreground">{o.description}</span>
                      </div>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </section>
          )}

          {/* 수식 설정 (formula) */}
          {isFormula && (
            <section className="space-y-3 rounded-md border bg-muted/30 p-3">
              <Label className="text-xs font-semibold text-muted-foreground">수식 설정</Label>
              <div className="space-y-1">
                <Label className="text-xs">수식 표현식</Label>
                <Textarea
                  rows={3}
                  value={(opts.expression as string) || ''}
                  onChange={(e) => updateOption('expression', e.target.value)}
                  placeholder="{unit_price} * {quantity}"
                />
                <p className="text-xs text-muted-foreground">
                  항목명을 {'{'}중괄호{'}'}로 감싸서 참조합니다. +, -, *, / 연산자와 괄호를 사용할 수 있습니다.
                </p>
              </div>
            </section>
          )}

          {/* 조건부 표시 */}
          {!isComputed && siblingFields && siblingFields.length > 0 && (
            <section className="space-y-2">
              <Label className="text-xs font-semibold text-muted-foreground">조건부 표시</Label>
              <p className="text-xs text-muted-foreground">
                다른 항목의 값에 따라 이 항목을 표시하거나 숨깁니다.
              </p>
              <VisibilityRuleEditor
                rules={(opts.visibility_rules as VisibilityRule[]) || []}
                siblingFields={siblingFields.filter((f) => f.slug !== field.slug && !isLayoutType(f.field_type) && !isComputedType(f.field_type))}
                onChange={(rules) => updateOption('visibility_rules', rules.length > 0 ? rules : undefined)}
              />
            </section>
          )}

          {/* 코드 (Slug) */}
          <section className="space-y-2">
            <Label className="text-xs font-semibold text-muted-foreground">코드</Label>
            <div className="relative">
              <Input
                value={field.slug}
                onChange={(e) => update({ slug: e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, '') })}
                placeholder="자동 생성됨"
              />
              {generateSlug.isPending && (
                <Loader2 className="absolute top-1/2 right-3 h-4 w-4 -translate-y-1/2 animate-spin text-muted-foreground" />
              )}
            </div>
            <p className="text-xs text-muted-foreground">
              * 자동 계산 컴포넌트와 REST API 에서 사용됩니다. 영문, 숫자, 밑줄(_)만 입력 가능
            </p>
          </section>
        </div>
      )}
    </div>
  )
}

// -- SubColumnEditor: visual column editor for table field --

interface SubColumnDef {
  key: string
  label: string
  type: 'text' | 'number' | 'select'
  choices?: string[]
}

const SUB_COL_TYPES = [
  { value: 'text', label: '텍스트' },
  { value: 'number', label: '숫자' },
  { value: 'select', label: '선택' },
] as const

let subColCounter = 0

function SubColumnEditor({
  columns,
  onChange,
}: {
  columns: SubColumnDef[]
  onChange: (cols: SubColumnDef[]) => void
}) {
  function addColumn() {
    subColCounter += 1
    onChange([...columns, { key: `col_${subColCounter}`, label: '', type: 'text' }])
  }

  function updateColumn(idx: number, patch: Partial<SubColumnDef>) {
    const next = columns.map((c, i) => {
      if (i !== idx) return c
      const updated = { ...c, ...patch }
      // auto-generate key from label
      if (patch.label !== undefined) {
        updated.key = patch.label
          .toLowerCase()
          .replace(/[^a-z0-9가-힣]/g, '_')
          .replace(/_+/g, '_')
          .replace(/^_|_$/g, '') || c.key
      }
      // clear choices when switching away from select
      if (patch.type && patch.type !== 'select') {
        delete updated.choices
      }
      return updated
    })
    onChange(next)
  }

  function removeColumn(idx: number) {
    onChange(columns.filter((_, i) => i !== idx))
  }

  function moveColumn(from: number, to: number) {
    if (to < 0 || to >= columns.length) return
    const next = [...columns]
    const [moved] = next.splice(from, 1)
    next.splice(to, 0, moved)
    onChange(next)
  }

  return (
    <section className="space-y-2">
      <Label className="text-xs font-semibold text-muted-foreground">테이블 열 설정</Label>
      <div className="space-y-1.5">
        {columns.map((col, idx) => (
          <div key={col.key + idx} className="space-y-1">
            <div className="flex items-center gap-1">
              <button
                type="button"
                className="cursor-grab text-muted-foreground hover:text-foreground"
                title="위로"
                onPointerDown={() => moveColumn(idx, idx - 1)}
              >
                <GripVertical className="h-3.5 w-3.5" />
              </button>
              <Input
                className="h-7 text-sm flex-1"
                placeholder="열 이름"
                value={col.label}
                onChange={(e) => updateColumn(idx, { label: e.target.value })}
              />
              <Select
                value={col.type}
                onValueChange={(v) => updateColumn(idx, { type: v as SubColumnDef['type'] })}
              >
                <SelectTrigger className="h-7 w-24 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SUB_COL_TYPES.map((t) => (
                    <SelectItem key={t.value} value={t.value}>{t.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <button
                type="button"
                className="text-muted-foreground hover:text-destructive"
                onClick={() => removeColumn(idx)}
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </div>
            {col.type === 'select' && (
              <Input
                className="ml-5 h-7 text-xs"
                placeholder="옵션 (쉼표 구분: 대기, 진행, 완료)"
                value={(col.choices || []).join(', ')}
                onChange={(e) =>
                  updateColumn(idx, {
                    choices: e.target.value.split(',').map((s) => s.trim()).filter(Boolean),
                  })
                }
              />
            )}
          </div>
        ))}
      </div>
      <button
        type="button"
        className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        onClick={addColumn}
      >
        <Plus className="h-3.5 w-3.5" /> 열 추가
      </button>
    </section>
  )
}

// -- Visibility Rule Editor --

interface VisibilityRule {
  field_slug: string
  operator: 'eq' | 'neq' | 'is_empty' | 'is_not_empty'
  value?: string
}

const VISIBILITY_OPERATORS: { value: VisibilityRule['operator']; label: string }[] = [
  { value: 'eq', label: '값이 같을 때' },
  { value: 'neq', label: '값이 다를 때' },
  { value: 'is_empty', label: '비어있을 때' },
  { value: 'is_not_empty', label: '값이 있을 때' },
]

function VisibilityRuleEditor({
  rules,
  siblingFields,
  onChange,
}: {
  rules: VisibilityRule[]
  siblingFields: { slug: string; label: string; field_type: string; options?: Record<string, unknown> }[]
  onChange: (rules: VisibilityRule[]) => void
}) {
  function addRule() {
    onChange([...rules, { field_slug: '', operator: 'eq' as const, value: '' }])
  }

  function updateRule(index: number, patch: Partial<VisibilityRule>) {
    onChange(rules.map((r, i) => (i === index ? { ...r, ...patch } as VisibilityRule : r)))
  }

  function removeRule(index: number) {
    onChange(rules.filter((_, i) => i !== index))
  }

  return (
    <div className="space-y-2">
      {rules.map((rule, i) => {
        const targetField = siblingFields.find((f) => f.slug === rule.field_slug)
        const needsValue = rule.operator === 'eq' || rule.operator === 'neq'
        const choices = targetField?.field_type === 'select'
          ? (targetField.options?.choices as string[]) || []
          : []

        return (
          <div key={i} className="space-y-1.5 rounded-md border p-2">
            <div className="flex items-center gap-1">
              <Select value={rule.field_slug} onValueChange={(v) => updateRule(i, { field_slug: v ?? '' })}>
                <SelectTrigger className="h-7 text-xs">
                  <SelectValue placeholder="항목 선택" />
                </SelectTrigger>
                <SelectContent>
                  {siblingFields.map((f) => (
                    <SelectItem key={f.slug} value={f.slug}>{f.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <button
                type="button"
                className="shrink-0 text-xs text-muted-foreground hover:text-destructive"
                onClick={() => removeRule(i)}
              >
                ×
              </button>
            </div>
            <Select value={rule.operator} onValueChange={(v) => updateRule(i, { operator: v as VisibilityRule['operator'] })}>
              <SelectTrigger className="h-7 text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {VISIBILITY_OPERATORS.map((o) => (
                  <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            {needsValue && (
              choices.length > 0 ? (
                <Select value={rule.value || ''} onValueChange={(v) => updateRule(i, { value: v ?? '' })}>
                  <SelectTrigger className="h-7 text-xs">
                    <SelectValue placeholder="값 선택" />
                  </SelectTrigger>
                  <SelectContent>
                    {choices.map((c) => (
                      <SelectItem key={c} value={c}>{c}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <Input
                  className="h-7 text-xs"
                  value={rule.value || ''}
                  onChange={(e) => updateRule(i, { value: e.target.value })}
                  placeholder="비교할 값"
                />
              )
            )}
          </div>
        )
      })}
      <button
        type="button"
        className="text-xs text-primary hover:underline"
        onClick={addRule}
      >
        + 조건 추가
      </button>
    </div>
  )
}

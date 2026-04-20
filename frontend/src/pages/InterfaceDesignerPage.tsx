import { useCallback, useMemo, useState } from 'react'
import { useParams } from 'react-router'
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
  Legend,
  LineChart,
  Line,
} from 'recharts'
import {
  BarChart3,
  GripVertical,
  Hash,
  LayoutGrid,
  Plus,
  Settings2,
  Trash2,
  Type,
  Table2,
  X,
} from 'lucide-react'
import { toast } from 'sonner'

import ErrorState from '@/components/common/ErrorState'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { useCollection } from '@/hooks/useCollections'
import { useAggregate, useCollectionCount, useEntries } from '@/hooks/useEntries'
import type { Field } from '@/lib/types'

// ── Types ──

type WidgetType = 'stat_card' | 'bar_chart' | 'pie_chart' | 'line_chart' | 'data_table' | 'text'

interface WidgetConfig {
  id: string
  type: WidgetType
  title: string
  width: 1 | 2 | 3 | 4   // grid columns (out of 4)
  // stat_card
  stat_fn?: 'count' | 'sum' | 'avg' | 'min' | 'max'
  stat_field?: string
  // charts
  group_field?: string
  value_fn?: 'count' | 'sum' | 'avg'
  value_field?: string
  // data_table
  table_fields?: string[]
  table_limit?: number
  // text
  text_content?: string
}

interface InterfaceConfig {
  widgets: WidgetConfig[]
}

const COLORS = [
  '#1f2937', '#374151', '#4b5563', '#6b7280',
  '#9ca3af', '#d1d5db', '#111827', '#334155',
]

const WIDGET_TYPES: { type: WidgetType; label: string; icon: typeof Hash }[] = [
  { type: 'stat_card', label: '숫자 카드', icon: Hash },
  { type: 'bar_chart', label: '막대 차트', icon: BarChart3 },
  { type: 'pie_chart', label: '파이 차트', icon: LayoutGrid },
  { type: 'line_chart', label: '라인 차트', icon: BarChart3 },
  { type: 'data_table', label: '데이터 테이블', icon: Table2 },
  { type: 'text', label: '텍스트', icon: Type },
]

function generateId() {
  return `w_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`
}

// ── Main Page ──

export default function InterfaceDesignerPage() {
  const { appId } = useParams()
  const { data: collection, isLoading, isError, error } = useCollection(appId)

  const storageKey = appId ? `phaeton:interface:${appId}` : null
  const [config, setConfig] = useState<InterfaceConfig>(() => {
    if (storageKey) {
      try {
        const saved = localStorage.getItem(storageKey)
        if (saved) return JSON.parse(saved)
      } catch { /* ignore */ }
    }
    return { widgets: [] }
  })

  const [editMode, setEditMode] = useState(false)
  const [editingWidget, setEditingWidget] = useState<string | null>(null)
  const [dragIdx, setDragIdx] = useState<number | null>(null)

  const saveConfig = useCallback(
    (next: InterfaceConfig) => {
      setConfig(next)
      if (storageKey) {
        try { localStorage.setItem(storageKey, JSON.stringify(next)) } catch { /* ignore */ }
      }
    },
    [storageKey],
  )

  const addWidget = useCallback(
    (type: WidgetType) => {
      const widget: WidgetConfig = {
        id: generateId(),
        type,
        title: WIDGET_TYPES.find((t) => t.type === type)?.label ?? '',
        width: type === 'data_table' ? 4 : type === 'text' ? 4 : 1,
      }
      saveConfig({ widgets: [...config.widgets, widget] })
      setEditingWidget(widget.id)
    },
    [config, saveConfig],
  )

  const updateWidget = useCallback(
    (id: string, patch: Partial<WidgetConfig>) => {
      const widgets = config.widgets.map((w) => (w.id === id ? { ...w, ...patch } : w))
      saveConfig({ widgets })
    },
    [config, saveConfig],
  )

  const removeWidget = useCallback(
    (id: string) => {
      saveConfig({ widgets: config.widgets.filter((w) => w.id !== id) })
      if (editingWidget === id) setEditingWidget(null)
    },
    [config, saveConfig, editingWidget],
  )

  // Drag and drop reorder.
  const handleDragStart = useCallback((idx: number) => {
    setDragIdx(idx)
  }, [])

  const handleDragOver = useCallback((e: React.DragEvent, idx: number) => {
    e.preventDefault()
    if (dragIdx === null || dragIdx === idx) return
    setConfig((prev) => {
      const widgets = [...prev.widgets]
      const [moved] = widgets.splice(dragIdx, 1)
      widgets.splice(idx, 0, moved)
      return { widgets }
    })
    setDragIdx(idx)
  }, [dragIdx])

  const handleDragEnd = useCallback(() => {
    setDragIdx(null)
    // Persist reorder.
    if (storageKey) {
      try { localStorage.setItem(storageKey, JSON.stringify(config)) } catch { /* ignore */ }
    }
  }, [config, storageKey])

  if (isLoading) return <LoadingState />
  if (isError) return <ErrorState error={error} />
  if (!collection) return null

  const fields = collection.fields ?? []
  const selectFields = fields.filter((f) => f.field_type === 'select')
  const numericFields = fields.filter((f) => f.field_type === 'number' || f.field_type === 'integer')
  const dateFields = fields.filter((f) => f.field_type === 'date' || f.field_type === 'datetime')
  const allDataFields = fields.filter((f) => !['label', 'line', 'spacer'].includes(f.field_type))

  const editWidget = editingWidget ? config.widgets.find((w) => w.id === editingWidget) : null

  return (
    <div>
      <PageHeader
        breadcrumb={[
          { label: '앱 목록', href: '/apps' },
          { label: collection.label, href: `/apps/${collection.id}` },
          { label: '인터페이스' },
        ]}
        title="인터페이스 디자이너"
        description="위젯을 추가하여 커스텀 대시보드를 구성합니다"
        actions={
          <Button
            variant={editMode ? 'default' : 'outline'}
            onClick={() => {
              setEditMode(!editMode)
              if (editMode) {
                setEditingWidget(null)
                toast.success('인터페이스가 저장되었습니다')
              }
            }}
          >
            {editMode ? '편집 완료' : '편집'}
          </Button>
        }
      />

      <div className="flex gap-6">
        {/* Main grid */}
        <div className="flex-1">
          {editMode && (
            <div className="mb-4 flex flex-wrap gap-2">
              {WIDGET_TYPES.map(({ type, label, icon: Icon }) => (
                <Button
                  key={type}
                  variant="outline"
                  size="sm"
                  className="gap-1.5"
                  onClick={() => addWidget(type)}
                >
                  <Icon className="h-3.5 w-3.5" />
                  <Plus className="h-3 w-3" />
                  {label}
                </Button>
              ))}
            </div>
          )}

          {config.widgets.length === 0 && (
            <div className="flex flex-col items-center justify-center rounded-lg border-2 border-dashed py-16 text-muted-foreground">
              <LayoutGrid className="mb-3 h-10 w-10" />
              <p className="text-sm">위젯을 추가하여 대시보드를 구성하세요</p>
              {!editMode && (
                <Button variant="outline" size="sm" className="mt-3" onClick={() => setEditMode(true)}>
                  편집 시작
                </Button>
              )}
            </div>
          )}

          <div className="grid grid-cols-4 gap-4">
            {config.widgets.map((widget, idx) => (
              <div
                key={widget.id}
                className={`rounded-lg border bg-card ${editMode ? 'cursor-move' : ''} ${editingWidget === widget.id ? 'ring-2 ring-primary' : ''}`}
                style={{ gridColumn: `span ${widget.width}` }}
                draggable={editMode}
                onDragStart={() => handleDragStart(idx)}
                onDragOver={(e) => handleDragOver(e, idx)}
                onDragEnd={handleDragEnd}
              >
                {/* Widget header */}
                <div className="flex items-center justify-between border-b px-3 py-2">
                  <div className="flex items-center gap-1.5 text-sm font-medium">
                    {editMode && <GripVertical className="h-3.5 w-3.5 text-muted-foreground" />}
                    {widget.title}
                  </div>
                  {editMode && (
                    <div className="flex items-center gap-1">
                      <button
                        type="button"
                        className="rounded p-1 hover:bg-accent"
                        onClick={() => setEditingWidget(editingWidget === widget.id ? null : widget.id)}
                      >
                        <Settings2 className="h-3.5 w-3.5" />
                      </button>
                      <button
                        type="button"
                        className="rounded p-1 hover:bg-destructive/10 hover:text-destructive"
                        onClick={() => removeWidget(widget.id)}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  )}
                </div>
                {/* Widget body */}
                <div className="p-3">
                  <WidgetRenderer
                    widget={widget}
                    slug={collection.slug}
                    fields={fields}
                  />
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Properties panel */}
        {editMode && editWidget && (
          <div className="w-72 shrink-0 rounded-lg border bg-card p-4">
            <div className="mb-3 flex items-center justify-between">
              <h3 className="text-sm font-medium">위젯 설정</h3>
              <button
                type="button"
                className="rounded p-1 hover:bg-accent"
                onClick={() => setEditingWidget(null)}
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <WidgetPropertyEditor
              widget={editWidget}
              fields={fields}
              selectFields={selectFields}
              numericFields={numericFields}
              dateFields={dateFields}
              allDataFields={allDataFields}
              onChange={(patch) => updateWidget(editWidget.id, patch)}
            />
          </div>
        )}
      </div>
    </div>
  )
}

// ── Widget Property Editor ──

function WidgetPropertyEditor({
  widget,
  selectFields,
  numericFields,
  dateFields,
  allDataFields,
  onChange,
}: {
  widget: WidgetConfig
  fields: Field[]
  selectFields: Field[]
  numericFields: Field[]
  dateFields: Field[]
  allDataFields: Field[]
  onChange: (patch: Partial<WidgetConfig>) => void
}) {
  return (
    <div className="space-y-3">
      <div className="space-y-1">
        <Label className="text-xs">제목</Label>
        <Input value={widget.title} onChange={(e) => onChange({ title: e.target.value })} />
      </div>

      <div className="space-y-1">
        <Label className="text-xs">폭 (1-4칸)</Label>
        <Select value={String(widget.width)} onValueChange={(v) => onChange({ width: Number(v) as 1 | 2 | 3 | 4 })}>
          <SelectTrigger><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="1">1칸 (25%)</SelectItem>
            <SelectItem value="2">2칸 (50%)</SelectItem>
            <SelectItem value="3">3칸 (75%)</SelectItem>
            <SelectItem value="4">4칸 (100%)</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {widget.type === 'stat_card' && (
        <>
          <div className="space-y-1">
            <Label className="text-xs">집계 함수</Label>
            <Select value={widget.stat_fn || 'count'} onValueChange={(v) => onChange({ stat_fn: v as WidgetConfig['stat_fn'] })}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="count">건수</SelectItem>
                <SelectItem value="sum">합계</SelectItem>
                <SelectItem value="avg">평균</SelectItem>
                <SelectItem value="min">최소</SelectItem>
                <SelectItem value="max">최대</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {widget.stat_fn && widget.stat_fn !== 'count' && (
            <div className="space-y-1">
              <Label className="text-xs">대상 필드</Label>
              <Select value={widget.stat_field || ''} onValueChange={(v) => onChange({ stat_field: v ?? undefined })}>
                <SelectTrigger><SelectValue placeholder="필드 선택" /></SelectTrigger>
                <SelectContent>
                  {numericFields.map((f) => (
                    <SelectItem key={f.slug} value={f.slug}>{f.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
        </>
      )}

      {(widget.type === 'bar_chart' || widget.type === 'pie_chart' || widget.type === 'line_chart') && (
        <>
          <div className="space-y-1">
            <Label className="text-xs">그룹 필드</Label>
            <Select value={widget.group_field || ''} onValueChange={(v) => onChange({ group_field: v ?? undefined })}>
              <SelectTrigger><SelectValue placeholder="필드 선택" /></SelectTrigger>
              <SelectContent>
                {selectFields.map((f) => (
                  <SelectItem key={f.slug} value={f.slug}>{f.label}</SelectItem>
                ))}
                {dateFields.map((f) => (
                  <SelectItem key={f.slug} value={f.slug}>{f.label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1">
            <Label className="text-xs">값 함수</Label>
            <Select value={widget.value_fn || 'count'} onValueChange={(v) => onChange({ value_fn: v as 'count' | 'sum' | 'avg' })}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="count">건수</SelectItem>
                <SelectItem value="sum">합계</SelectItem>
                <SelectItem value="avg">평균</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {widget.value_fn && widget.value_fn !== 'count' && (
            <div className="space-y-1">
              <Label className="text-xs">값 필드</Label>
              <Select value={widget.value_field || ''} onValueChange={(v) => onChange({ value_field: v ?? undefined })}>
                <SelectTrigger><SelectValue placeholder="필드 선택" /></SelectTrigger>
                <SelectContent>
                  {numericFields.map((f) => (
                    <SelectItem key={f.slug} value={f.slug}>{f.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
        </>
      )}

      {widget.type === 'data_table' && (
        <>
          <div className="space-y-1">
            <Label className="text-xs">표시할 필드 (쉼표 구분)</Label>
            <Input
              value={(widget.table_fields || []).join(', ')}
              onChange={(e) =>
                onChange({ table_fields: e.target.value.split(',').map((s) => s.trim()).filter(Boolean) })
              }
              placeholder="모든 필드"
            />
            <p className="text-[10px] text-muted-foreground">
              사용 가능: {allDataFields.map((f) => f.slug).join(', ')}
            </p>
          </div>
          <div className="space-y-1">
            <Label className="text-xs">표시 건수</Label>
            <Input
              type="number"
              min={1}
              max={50}
              value={widget.table_limit || 5}
              onChange={(e) => onChange({ table_limit: Number(e.target.value) || 5 })}
            />
          </div>
        </>
      )}

      {widget.type === 'text' && (
        <div className="space-y-1">
          <Label className="text-xs">내용</Label>
          <Textarea
            rows={4}
            value={widget.text_content || ''}
            onChange={(e) => onChange({ text_content: e.target.value })}
            placeholder="안내 문구, 메모 등"
          />
        </div>
      )}
    </div>
  )
}

// ── Widget Renderers ──

function WidgetRenderer({
  widget,
  slug,
  fields,
}: {
  widget: WidgetConfig
  slug: string
  fields: Field[]
}) {
  switch (widget.type) {
    case 'stat_card':
      return <StatCardWidget widget={widget} slug={slug} />
    case 'bar_chart':
      return <BarChartWidget widget={widget} slug={slug} />
    case 'pie_chart':
      return <PieChartWidget widget={widget} slug={slug} />
    case 'line_chart':
      return <LineChartWidget widget={widget} slug={slug} />
    case 'data_table':
      return <DataTableWidget widget={widget} slug={slug} fields={fields} />
    case 'text':
      return <TextWidget widget={widget} />
    default:
      return <p className="text-xs text-muted-foreground">알 수 없는 위젯</p>
  }
}

function StatCardWidget({ widget, slug }: { widget: WidgetConfig; slug: string }) {
  const fn = widget.stat_fn || 'count'
  const { data: count } = useCollectionCount(fn === 'count' ? slug : undefined)
  const { data: aggData } = useAggregate(
    fn !== 'count' && widget.stat_field ? slug : undefined,
    { group: '_created_by', fn, field: widget.stat_field || '' },
  )

  let displayValue: number | string = '-'
  if (fn === 'count') {
    displayValue = count ?? '-'
  } else if (aggData?.length) {
    const total = aggData.reduce((acc, r) => acc + r.value, 0)
    if (fn === 'avg') {
      displayValue = (total / aggData.length).toLocaleString('ko', { maximumFractionDigits: 1 })
    } else {
      displayValue = total.toLocaleString('ko')
    }
  }

  const fnLabel: Record<string, string> = { count: '건수', sum: '합계', avg: '평균', min: '최소', max: '최대' }

  return (
    <div>
      <div className="text-xs text-muted-foreground">{fnLabel[fn]}</div>
      <div className="mt-1 text-3xl font-semibold">{typeof displayValue === 'number' ? displayValue.toLocaleString('ko') : displayValue}</div>
    </div>
  )
}

function BarChartWidget({ widget, slug }: { widget: WidgetConfig; slug: string }) {
  const { data } = useAggregate(
    widget.group_field ? slug : undefined,
    {
      group: widget.group_field || '',
      fn: widget.value_fn || 'count',
      field: widget.value_field || '',
    },
  )
  if (!data?.length) return <p className="text-xs text-muted-foreground">데이터 없음</p>

  return (
    <ResponsiveContainer width="100%" height={200}>
      <BarChart data={data} margin={{ top: 4, right: 8, bottom: 4, left: 0 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
        <XAxis dataKey="group" tick={{ fontSize: 11 }} tickLine={false} axisLine={{ stroke: '#e5e7eb' }} />
        <YAxis tick={{ fontSize: 11 }} tickLine={false} axisLine={false} width={50} />
        <Tooltip contentStyle={{ fontSize: 12, border: '1px solid #e5e7eb', borderRadius: 6 }} />
        <Bar dataKey="value" radius={[4, 4, 0, 0]}>
          {data.map((_, i) => (
            <Cell key={i} fill={COLORS[i % COLORS.length]} />
          ))}
        </Bar>
      </BarChart>
    </ResponsiveContainer>
  )
}

function PieChartWidget({ widget, slug }: { widget: WidgetConfig; slug: string }) {
  const { data } = useAggregate(
    widget.group_field ? slug : undefined,
    {
      group: widget.group_field || '',
      fn: widget.value_fn || 'count',
      field: widget.value_field || '',
    },
  )
  if (!data?.length) return <p className="text-xs text-muted-foreground">데이터 없음</p>

  return (
    <ResponsiveContainer width="100%" height={200}>
      <PieChart>
        <Pie
          data={data}
          dataKey="value"
          nameKey="group"
          cx="50%"
          cy="50%"
          outerRadius={70}
          strokeWidth={1}
          stroke="#fff"
          label={({ name, percent }: { name?: string | null; percent?: number }) =>
            `${name ?? ''} ${((percent ?? 0) * 100).toFixed(0)}%`
          }
          labelLine={false}
        >
          {data.map((_, i) => (
            <Cell key={i} fill={COLORS[i % COLORS.length]} />
          ))}
        </Pie>
        <Tooltip contentStyle={{ fontSize: 12, border: '1px solid #e5e7eb', borderRadius: 6 }} />
        <Legend iconSize={8} wrapperStyle={{ fontSize: 11 }} />
      </PieChart>
    </ResponsiveContainer>
  )
}

function LineChartWidget({ widget, slug }: { widget: WidgetConfig; slug: string }) {
  const { data } = useAggregate(
    widget.group_field ? slug : undefined,
    {
      group: widget.group_field || '',
      fn: widget.value_fn || 'count',
      field: widget.value_field || '',
    },
  )
  if (!data?.length || data.length < 2) return <p className="text-xs text-muted-foreground">데이터 부족</p>

  const sorted = [...data].sort((a, b) => a.group.localeCompare(b.group))

  return (
    <ResponsiveContainer width="100%" height={200}>
      <LineChart data={sorted} margin={{ top: 4, right: 8, bottom: 4, left: 0 }}>
        <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
        <XAxis dataKey="group" tick={{ fontSize: 11 }} tickLine={false} axisLine={{ stroke: '#e5e7eb' }} />
        <YAxis tick={{ fontSize: 11 }} tickLine={false} axisLine={false} width={50} />
        <Tooltip contentStyle={{ fontSize: 12, border: '1px solid #e5e7eb', borderRadius: 6 }} />
        <Line type="monotone" dataKey="value" stroke="#1f2937" strokeWidth={2} dot={{ fill: '#1f2937', r: 3 }} />
      </LineChart>
    </ResponsiveContainer>
  )
}

function DataTableWidget({
  widget,
  slug,
  fields,
}: {
  widget: WidgetConfig
  slug: string
  fields: Field[]
}) {
  const limit = widget.table_limit || 5
  const { data: list } = useEntries(slug, { limit })

  const visibleFields = useMemo(() => {
    if (widget.table_fields?.length) {
      return widget.table_fields
        .map((s) => fields.find((f) => f.slug === s))
        .filter((f): f is Field => !!f)
    }
    return fields.filter((f) => !['label', 'line', 'spacer'].includes(f.field_type)).slice(0, 5)
  }, [widget.table_fields, fields])

  if (!list?.data?.length) return <p className="text-xs text-muted-foreground">데이터 없음</p>

  return (
    <div className="overflow-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b">
            {visibleFields.map((f) => (
              <th key={f.slug} className="px-2 py-1.5 text-left text-xs font-medium text-muted-foreground">
                {f.label}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {list.data.map((row, i) => (
            <tr key={i} className="border-b last:border-0">
              {visibleFields.map((f) => (
                <td key={f.slug} className="px-2 py-1.5 text-xs">
                  {row[f.slug] != null ? String(row[f.slug]) : '-'}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function TextWidget({ widget }: { widget: WidgetConfig }) {
  return (
    <div className="whitespace-pre-wrap text-sm text-muted-foreground">
      {widget.text_content || '텍스트를 입력하세요'}
    </div>
  )
}

import { useMemo, useState } from 'react'
import { Loader2, Sparkles, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { formatError } from '@/lib/api/errors'
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

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useAIAvailable } from '@/contexts/AIAvailabilityContext'
import { useAIBuildChart } from '@/hooks/useAI'
import { useCharts, useCreateChart, useDeleteChart } from '@/hooks/useCharts'
import { useAggregate, useCollectionCount } from '@/hooks/useEntries'
import type { Collection, Field } from '@/lib/types'

const COLORS = [
  '#1f2937', '#374151', '#4b5563', '#6b7280',
  '#9ca3af', '#d1d5db', '#111827', '#334155',
  '#475569', '#64748b', '#94a3b8', '#cbd5e1',
]

interface Props {
  appId: string
  collection: Collection
}

export default function ChartTabContent({ appId, collection }: Props) {
  const aiAvailable = useAIAvailable()
  const buildChart = useAIBuildChart(appId)
  const createChart = useCreateChart(appId)
  const deleteChart = useDeleteChart(appId)
  const { data: savedCharts } = useCharts(appId)
  const [chartPrompt, setChartPrompt] = useState('')

  const fields = collection.fields ?? []
  const selectFields = fields.filter((f) => f.field_type === 'select')
  const numericFields = fields.filter(
    (f) => f.field_type === 'number' || f.field_type === 'integer',
  )
  const dateField = fields.find(
    (f) => f.field_type === 'date' || f.field_type === 'datetime',
  )

  function handleBuildChart() {
    if (!chartPrompt.trim() || buildChart.isPending) return
    buildChart.mutate(chartPrompt.trim(), {
      onSuccess: (res) => {
        createChart.mutate({
          name: res.name,
          chart_type: res.chart_type,
          config: res.config,
        }, {
          onSuccess: () => {
            toast.success('차트가 추가되었습니다')
            setChartPrompt('')
          },
          onError: (err) => toast.error(formatError(err)),
        })
      },
    })
  }

  return (
    <div className="space-y-6">
      {/* Summary cards */}
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <SummaryCard slug={collection.slug} label="전체 건수" />
        {numericFields.slice(0, 3).map((f) => (
          <NumericSummaryCard
            key={f.id}
            slug={collection.slug}
            field={f}
          />
        ))}
      </div>

      {/* AI chart builder */}
      {aiAvailable && (
        <div className="flex gap-2">
          <Input
            value={chartPrompt}
            onChange={(e) => setChartPrompt(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleBuildChart()}
            placeholder="차트를 설명하세요 (예: 담당자별 완료 건수 막대 차트)"
            className="max-w-md"
            disabled={buildChart.isPending || createChart.isPending}
          />
          <Button
            size="sm"
            disabled={!chartPrompt.trim() || buildChart.isPending || createChart.isPending}
            onClick={handleBuildChart}
            className="gap-1"
          >
            {buildChart.isPending || createChart.isPending ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Sparkles className="h-4 w-4" />
            )}
            차트 추가
          </Button>
        </div>
      )}

      {/* Saved charts */}
      {savedCharts?.data && savedCharts.data.length > 0 && (
        <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
          {savedCharts.data.map((chart) => (
            <SavedChartCard
              key={chart.id}
              chart={chart}
              slug={collection.slug}
              onDelete={() => deleteChart.mutate(chart.id)}
            />
          ))}
        </div>
      )}

      {/* Auto-generated charts */}
      <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
        {selectFields.map((sf) => (
          <ChartCard
            key={sf.id}
            slug={collection.slug}
            selectField={sf}
            numericField={numericFields[0]}
          />
        ))}
        {dateField && (
          <TimelineCard
            slug={collection.slug}
            dateField={dateField}
          />
        )}
      </div>
    </div>
  )
}

function SummaryCard({ slug, label }: { slug: string; label: string }) {
  const { data: count, isLoading } = useCollectionCount(slug)
  return (
    <div className="rounded-lg border bg-card p-4">
      <div className="text-xs font-medium text-muted-foreground">{label}</div>
      <div className="mt-1 text-2xl font-semibold">
        {isLoading ? '-' : (count ?? 0).toLocaleString('ko')}
      </div>
    </div>
  )
}

function NumericSummaryCard({ slug, field }: { slug: string; field: Field }) {
  const { data: sumData } = useAggregate(slug, {
    group: '_created_by',
    fn: 'sum',
    field: field.slug,
  })
  const { data: avgData } = useAggregate(slug, {
    group: '_created_by',
    fn: 'avg',
    field: field.slug,
  })

  const total = useMemo(
    () => sumData?.reduce((acc, r) => acc + r.value, 0) ?? 0,
    [sumData],
  )
  const avg = useMemo(
    () => {
      if (!avgData?.length) return 0
      return avgData.reduce((acc, r) => acc + r.value, 0) / avgData.length
    },
    [avgData],
  )

  return (
    <div className="rounded-lg border bg-card p-4">
      <div className="text-xs font-medium text-muted-foreground">
        {field.label}
      </div>
      <div className="mt-1 text-2xl font-semibold">
        {total.toLocaleString('ko', { maximumFractionDigits: 0 })}
      </div>
      <div className="text-xs text-muted-foreground">
        평균 {avg.toLocaleString('ko', { maximumFractionDigits: 1 })}
      </div>
    </div>
  )
}

function ChartCard({
  slug,
  selectField,
  numericField,
}: {
  slug: string
  selectField: Field
  numericField?: Field
}) {
  const { data: countData } = useAggregate(slug, {
    group: selectField.slug,
    fn: 'count',
  })
  const { data: sumData } = useAggregate(
    numericField ? slug : undefined,
    {
      group: selectField.slug,
      fn: 'sum',
      field: numericField?.slug ?? '',
    },
  )

  const barData = sumData?.length ? sumData : countData
  const barLabel = sumData?.length
    ? `${selectField.label} x ${numericField?.label} (합계)`
    : `${selectField.label} (건수)`

  if (!countData?.length) return null

  return (
    <>
      <div className="rounded-lg border bg-card p-4">
        <div className="mb-3 text-sm font-medium">{barLabel}</div>
        <ResponsiveContainer width="100%" height={260}>
          <BarChart data={barData} margin={{ top: 4, right: 8, bottom: 4, left: 0 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
            <XAxis dataKey="group" tick={{ fontSize: 11 }} tickLine={false} axisLine={{ stroke: '#e5e7eb' }} />
            <YAxis tick={{ fontSize: 11 }} tickLine={false} axisLine={false} width={50} />
            <Tooltip contentStyle={{ fontSize: 12, border: '1px solid #e5e7eb', borderRadius: 6 }} />
            <Bar dataKey="value" radius={[4, 4, 0, 0]}>
              {(barData ?? []).map((_, i) => (
                <Cell key={i} fill={COLORS[i % COLORS.length]} />
              ))}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
      </div>

      <div className="rounded-lg border bg-card p-4">
        <div className="mb-3 text-sm font-medium">{selectField.label} 분포</div>
        <ResponsiveContainer width="100%" height={260}>
          <PieChart>
            <Pie
              data={countData}
              dataKey="value"
              nameKey="group"
              cx="50%"
              cy="50%"
              outerRadius={90}
              strokeWidth={1}
              stroke="#fff"
              label={({ name, percent }: { name?: string; percent?: number }) => `${name ?? ''} ${((percent ?? 0) * 100).toFixed(0)}%`}
              labelLine={false}
            >
              {countData.map((_, i) => (
                <Cell key={i} fill={COLORS[i % COLORS.length]} />
              ))}
            </Pie>
            <Tooltip contentStyle={{ fontSize: 12, border: '1px solid #e5e7eb', borderRadius: 6 }} formatter={(value: unknown) => Number(value).toLocaleString('ko')} />
            <Legend iconSize={8} wrapperStyle={{ fontSize: 11 }} />
          </PieChart>
        </ResponsiveContainer>
      </div>
    </>
  )
}

function TimelineCard({ slug, dateField }: { slug: string; dateField: Field }) {
  const { data } = useAggregate(slug, {
    group: dateField.slug,
    fn: 'count',
  })

  if (!data?.length || data.length < 2) return null

  const sorted = [...data].sort((a, b) => a.group.localeCompare(b.group))

  return (
    <div className="rounded-lg border bg-card p-4 md:col-span-2">
      <div className="mb-3 text-sm font-medium">
        {dateField.label} 기준 추이
      </div>
      <ResponsiveContainer width="100%" height={260}>
        <LineChart data={sorted} margin={{ top: 4, right: 8, bottom: 4, left: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
          <XAxis dataKey="group" tick={{ fontSize: 11 }} tickLine={false} axisLine={{ stroke: '#e5e7eb' }} />
          <YAxis tick={{ fontSize: 11 }} tickLine={false} axisLine={false} width={50} />
          <Tooltip contentStyle={{ fontSize: 12, border: '1px solid #e5e7eb', borderRadius: 6 }} />
          <Line
            type="monotone"
            dataKey="value"
            stroke="#1f2937"
            strokeWidth={2}
            dot={{ fill: '#1f2937', r: 3 }}
            activeDot={{ r: 5 }}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}

function SavedChartCard({
  chart,
  slug,
  onDelete,
}: {
  chart: { id: string; name: string; chart_type: string; config: Record<string, unknown> }
  slug: string
  onDelete: () => void
}) {
  const groupField = String(chart.config.group_field ?? '')
  const valueField = String(chart.config.value_field ?? '')
  const aggregation = String(chart.config.aggregation ?? 'count')

  const { data, isLoading } = useAggregate(slug, {
    group: groupField,
    fn: aggregation !== 'count' ? aggregation : undefined,
    field: valueField || undefined,
  })

  const chartData = useMemo(() => {
    if (!data) return []
    return data.map((d) => ({
      group: d.group || '(없음)',
      value: d.value ?? 0,
    }))
  }, [data])

  if (isLoading) return <div className="rounded-lg border bg-card p-4 h-[300px] flex items-center justify-center"><Loader2 className="animate-spin" /></div>

  return (
    <div className="rounded-lg border bg-card p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm font-medium">{chart.name}</h3>
        <Button variant="ghost" size="sm" className="h-6 w-6 p-0" onClick={onDelete}>
          <Trash2 className="h-3 w-3" />
        </Button>
      </div>
      <ResponsiveContainer width="100%" height={260}>
        {chart.chart_type === 'pie' || chart.chart_type === 'doughnut' ? (
          <PieChart>
            <Pie
              data={chartData}
              dataKey="value"
              nameKey="group"
              cx="50%"
              cy="50%"
              innerRadius={chart.chart_type === 'doughnut' ? 40 : 0}
              outerRadius={90}
              label={({ name, percent }: { name?: string; percent?: number }) => `${name ?? ''} ${((percent ?? 0) * 100).toFixed(0)}%`}
            >
              {chartData.map((_: unknown, i: number) => (
                <Cell key={i} fill={COLORS[i % COLORS.length]} />
              ))}
            </Pie>
            <Tooltip />
            <Legend />
          </PieChart>
        ) : chart.chart_type === 'line' || chart.chart_type === 'area' ? (
          <LineChart data={chartData}>
            <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
            <XAxis dataKey="group" tick={{ fontSize: 11 }} />
            <YAxis tick={{ fontSize: 11 }} width={50} />
            <Tooltip />
            <Line type="monotone" dataKey="value" stroke="#1f2937" strokeWidth={2} />
          </LineChart>
        ) : (
          <BarChart data={chartData}>
            <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
            <XAxis dataKey="group" tick={{ fontSize: 11 }} />
            <YAxis tick={{ fontSize: 11 }} width={50} />
            <Tooltip />
            <Bar dataKey="value" fill="#1f2937" radius={[4, 4, 0, 0]} />
          </BarChart>
        )}
      </ResponsiveContainer>
    </div>
  )
}

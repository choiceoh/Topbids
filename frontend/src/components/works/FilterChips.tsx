import { ArrowDown, ArrowUp, X } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { FILTER_OPERATORS } from '@/lib/constants'
import type { Field, FilterCondition, FilterGroup } from '@/lib/types'
import { flattenFilterGroup } from '@/lib/types'

import type { SortItem } from './SortPanel'

interface Props {
  conditions?: FilterCondition[]
  filterGroup?: FilterGroup
  sortItems: SortItem[]
  fields: Field[]
  onRemoveFilter: (id: string) => void
  onRemoveSort: (index: number) => void
  onClearAll: () => void
}

const operatorLabel = (op: string) =>
  FILTER_OPERATORS.find((o) => o.value === op)?.label ?? op

function fieldLabel(fields: Field[], slug: string) {
  return fields.find((f) => f.slug === slug)?.label ?? slug
}

export default function FilterChips({
  conditions: legacyConditions,
  filterGroup,
  sortItems,
  fields,
  onRemoveFilter,
  onRemoveSort,
  onClearAll,
}: Props) {
  // Use filterGroup if available, otherwise fall back to legacy conditions
  const conditions = filterGroup
    ? flattenFilterGroup(filterGroup)
    : legacyConditions ?? []

  const groupLogic = filterGroup?.logic

  const total = conditions.length + sortItems.length
  if (total === 0) return null

  return (
    <div className="flex flex-wrap items-center gap-1.5 pt-2">
      {conditions.length > 0 && groupLogic && (
        <span className={`rounded px-1.5 py-0.5 text-[10px] font-semibold ${
          groupLogic === 'and' ? 'bg-blue-100 text-blue-700' : 'bg-amber-100 text-amber-700'
        }`}>
          {groupLogic === 'and' ? 'AND' : 'OR'}
        </span>
      )}
      {conditions.map((c) => (
        <Badge
          key={c.id}
          variant="secondary"
          className="gap-1 pr-1 font-normal"
        >
          <span className="font-medium">{fieldLabel(fields, c.field)}</span>
          <span className="text-muted-foreground">{operatorLabel(c.operator)}</span>
          {c.operator !== 'is_null' && c.value && (
            <span className="max-w-[120px] truncate">{c.value}</span>
          )}
          <button
            type="button"
            className="ml-0.5 rounded-sm hover:text-destructive"
            onClick={() => onRemoveFilter(c.id)}
          >
            <X className="h-3 w-3" />
          </button>
        </Badge>
      ))}

      {sortItems.map((s, i) => (
        <Badge
          key={`sort-${i}`}
          variant="secondary"
          className="gap-1 pr-1 font-normal"
        >
          {s.desc ? (
            <ArrowDown className="h-3 w-3" />
          ) : (
            <ArrowUp className="h-3 w-3" />
          )}
          <span className="font-medium">{fieldLabel(fields, s.field)}</span>
          <button
            type="button"
            className="ml-0.5 rounded-sm hover:text-destructive"
            onClick={() => onRemoveSort(i)}
          >
            <X className="h-3 w-3" />
          </button>
        </Badge>
      ))}

      {total > 1 && (
        <Button
          variant="ghost"
          size="sm"
          className="h-6 px-2 text-xs text-destructive"
          onClick={onClearAll}
        >
          전체 해제
        </Button>
      )}
    </div>
  )
}

import { Loader2, Plus, Trash2 } from 'lucide-react'
import { useId, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useAIAvailable } from '@/contexts/AIAvailabilityContext'
import { useAIBuildFilter } from '@/hooks/useAI'
import {
  FILTER_OPERATORS,
  isLayoutType,
  operatorsForFieldType,
} from '@/lib/constants'
import type { Field, FilterCondition, FilterGroup, FilterLogic } from '@/lib/types'

interface Props {
  fields: Field[]
  /** Flat conditions — legacy interface, used when filterGroup is not provided */
  conditions?: FilterCondition[]
  onChange?: (conditions: FilterCondition[]) => void
  /** Structured filter group with AND/OR logic */
  filterGroup?: FilterGroup
  onFilterGroupChange?: (group: FilterGroup) => void
  slug?: string
}

export default function FilterBuilder({
  fields,
  conditions: legacyConditions,
  onChange: legacyOnChange,
  filterGroup,
  onFilterGroupChange,
  slug,
}: Props) {
  const idBase = useId()
  const idCounter = useRef(0)
  const dataFields = fields.filter((f) => !isLayoutType(f.field_type))
  const aiAvailable = useAIAvailable()
  const buildFilter = useAIBuildFilter(slug)
  const [aiQuery, setAiQuery] = useState('')

  // Determine if we're in group mode or legacy mode
  const isGroupMode = !!filterGroup && !!onFilterGroupChange

  // Legacy flat mode helpers
  const conditions = legacyConditions ?? filterGroup?.conditions ?? []

  function nextId() {
    idCounter.current++
    return `${idBase}-${idCounter.current}`
  }

  function addCondition() {
    const first = dataFields[0]
    if (!first) return
    const ops = operatorsForFieldType(first.field_type)
    const newCond: FilterCondition = {
      id: nextId(),
      field: first.slug,
      operator: ops[0] ?? 'eq',
      value: '',
    }

    if (isGroupMode) {
      onFilterGroupChange!({
        ...filterGroup!,
        conditions: [...filterGroup!.conditions, newCond],
      })
    } else {
      legacyOnChange?.([...conditions, newCond])
    }
  }

  function updateCondition(id: string, patch: Partial<FilterCondition>) {
    const mapper = (c: FilterCondition) => {
      if (c.id !== id) return c
      const updated = { ...c, ...patch }
      if (patch.field && patch.field !== c.field) {
        const f = dataFields.find((df) => df.slug === patch.field)
        if (f) {
          const ops = operatorsForFieldType(f.field_type)
          if (!ops.includes(updated.operator as never)) {
            updated.operator = ops[0] ?? 'eq'
          }
        }
        updated.value = ''
      }
      return updated
    }

    if (isGroupMode) {
      onFilterGroupChange!(updateGroupConditions(filterGroup!, id, mapper))
    } else {
      legacyOnChange?.(conditions.map(mapper))
    }
  }

  function removeCondition(id: string) {
    if (isGroupMode) {
      onFilterGroupChange!(removeFromGroup(filterGroup!, id))
    } else {
      legacyOnChange?.(conditions.filter((c) => c.id !== id))
    }
  }

  function toggleLogic() {
    if (!isGroupMode) return
    onFilterGroupChange!({
      ...filterGroup!,
      logic: filterGroup!.logic === 'and' ? 'or' : 'and',
    })
  }

  function addSubGroup() {
    if (!isGroupMode) return
    onFilterGroupChange!({
      ...filterGroup!,
      groups: [
        ...filterGroup!.groups,
        {
          id: nextId(),
          logic: filterGroup!.logic === 'and' ? 'or' : 'and',
          conditions: [],
          groups: [],
        },
      ],
    })
  }

  function updateSubGroup(index: number, updated: FilterGroup) {
    if (!isGroupMode) return
    const newGroups = [...filterGroup!.groups]
    newGroups[index] = updated
    onFilterGroupChange!({ ...filterGroup!, groups: newGroups })
  }

  function removeSubGroup(index: number) {
    if (!isGroupMode) return
    const newGroups = filterGroup!.groups.filter((_, i) => i !== index)
    onFilterGroupChange!({ ...filterGroup!, groups: newGroups })
  }

  function handleAIFilter() {
    if (!aiQuery.trim() || buildFilter.isPending) return
    buildFilter.mutate(aiQuery.trim(), {
      onSuccess: (res) => {
        const newConditions: FilterCondition[] = res.map((c, i) => ({
          id: `${idBase}-ai-${Date.now()}-${i}`,
          field: c.field,
          operator: c.operator,
          value: c.value ?? '',
        }))
        if (isGroupMode) {
          onFilterGroupChange!({
            ...filterGroup!,
            conditions: newConditions,
            groups: [],
          })
        } else {
          legacyOnChange?.(newConditions)
        }
        setAiQuery('')
      },
    })
  }

  // Render all conditions (root level)
  const rootConditions = isGroupMode ? filterGroup!.conditions : conditions
  const rootLogic: FilterLogic = isGroupMode ? filterGroup!.logic : 'and'

  return (
    <div className="space-y-2">
      {aiAvailable && slug && (
        <>
          <div className="flex gap-1">
            <Input
              value={aiQuery}
              onChange={(e) => setAiQuery(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleAIFilter()}
              placeholder="예: 이번 달 완료된 건 보여줘"
              className="h-8 text-sm"
              disabled={buildFilter.isPending}
            />
            <Button
              type="button"
              size="sm"
              className="h-8 shrink-0"
              disabled={!aiQuery.trim() || buildFilter.isPending}
              onClick={handleAIFilter}
            >
              {buildFilter.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : '적용'}
            </Button>
          </div>
          {(rootConditions.length > 0 || (isGroupMode && filterGroup!.groups.length > 0)) && (
            <div className="border-b pb-1 mb-1">
              <span className="text-[10px] text-muted-foreground">또는 직접 설정</span>
            </div>
          )}
        </>
      )}

      {/* Root conditions */}
      {rootConditions.map((cond, idx) => (
        <div key={cond.id}>
          {idx > 0 && isGroupMode && (
            <div className="flex items-center my-1">
              <button
                type="button"
                className={`rounded px-2 py-0.5 text-[10px] font-semibold transition-colors ${
                  rootLogic === 'and'
                    ? 'bg-blue-100 text-blue-700 hover:bg-blue-200'
                    : 'bg-amber-100 text-amber-700 hover:bg-amber-200'
                }`}
                onClick={toggleLogic}
              >
                {rootLogic === 'and' ? 'AND' : 'OR'}
              </button>
              <div className="flex-1 border-b border-dashed ml-2" />
            </div>
          )}
          <ConditionRow
            cond={cond}
            dataFields={dataFields}
            onUpdate={(patch) => updateCondition(cond.id, patch)}
            onRemove={() => removeCondition(cond.id)}
          />
        </div>
      ))}

      {/* Sub-groups */}
      {isGroupMode && filterGroup!.groups.map((subGroup, gi) => (
        <div key={subGroup.id} className="ml-3 border-l-2 border-muted-foreground/20 pl-3 py-1 space-y-1">
          <div className="flex items-center gap-2">
            <button
              type="button"
              className={`rounded px-2 py-0.5 text-[10px] font-semibold transition-colors ${
                subGroup.logic === 'and'
                  ? 'bg-blue-100 text-blue-700 hover:bg-blue-200'
                  : 'bg-amber-100 text-amber-700 hover:bg-amber-200'
              }`}
              onClick={() => updateSubGroup(gi, { ...subGroup, logic: subGroup.logic === 'and' ? 'or' : 'and' })}
            >
              {subGroup.logic === 'and' ? 'AND' : 'OR'} 그룹
            </button>
            <Button
              variant="ghost"
              size="sm"
              className="h-6 w-6 p-0 text-muted-foreground hover:text-destructive"
              onClick={() => removeSubGroup(gi)}
            >
              <Trash2 className="h-3 w-3" />
            </Button>
          </div>
          <SubGroupBuilder
            group={subGroup}
            dataFields={dataFields}
            onChange={(updated) => updateSubGroup(gi, updated)}
            idBase={idBase}
            idCounter={idCounter}
          />
        </div>
      ))}

      <div className="flex gap-1">
        <Button variant="outline" size="sm" onClick={addCondition} className="gap-1">
          <Plus className="h-3 w-3" />
          조건 추가
        </Button>
        {isGroupMode && (
          <Button variant="outline" size="sm" onClick={addSubGroup} className="gap-1">
            <Plus className="h-3 w-3" />
            그룹 추가
          </Button>
        )}
      </div>
    </div>
  )
}

// --- Sub-components ---

function ConditionRow({
  cond,
  dataFields,
  onUpdate,
  onRemove,
}: {
  cond: FilterCondition
  dataFields: Field[]
  onUpdate: (patch: Partial<FilterCondition>) => void
  onRemove: () => void
}) {
  const field = dataFields.find((f) => f.slug === cond.field)
  const validOps = field ? operatorsForFieldType(field.field_type) : ['eq']
  const opLabels = FILTER_OPERATORS.filter((o) =>
    validOps.includes(o.value as never),
  )

  return (
    <div className="flex items-center gap-2">
      <Select
        value={cond.field}
        onValueChange={(v) => v && onUpdate({ field: v })}
      >
        <SelectTrigger className="w-[140px] h-8 text-sm">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {dataFields.map((f) => (
            <SelectItem key={f.slug} value={f.slug}>
              {f.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select
        value={cond.operator}
        onValueChange={(v) => v && onUpdate({ operator: v })}
      >
        <SelectTrigger className="w-[140px] h-8 text-sm">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {opLabels.map((o) => (
            <SelectItem key={o.value} value={o.value} label={o.label}>
              <div className="flex flex-col items-start whitespace-normal">
                <span>{o.label}</span>
                <span className="text-[11px] text-muted-foreground">{o.description}</span>
              </div>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      {cond.operator !== 'is_null' && (
        <ValueInput
          field={field}
          value={cond.value}
          onChange={(v) => onUpdate({ value: v })}
          isEmpty={!cond.value}
        />
      )}

      <Button
        variant="ghost"
        size="sm"
        className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
        onClick={onRemove}
        aria-label="조건 삭제"
      >
        <Trash2 className="h-4 w-4" />
      </Button>
    </div>
  )
}

function SubGroupBuilder({
  group,
  dataFields,
  onChange,
  idBase,
  idCounter,
}: {
  group: FilterGroup
  dataFields: Field[]
  onChange: (group: FilterGroup) => void
  idBase: string
  idCounter: React.MutableRefObject<number>
}) {
  function addCond() {
    const first = dataFields[0]
    if (!first) return
    const ops = operatorsForFieldType(first.field_type)
    idCounter.current++
    onChange({
      ...group,
      conditions: [
        ...group.conditions,
        {
          id: `${idBase}-sg-${idCounter.current}`,
          field: first.slug,
          operator: ops[0] ?? 'eq',
          value: '',
        },
      ],
    })
  }

  function updateCond(id: string, patch: Partial<FilterCondition>) {
    onChange({
      ...group,
      conditions: group.conditions.map((c) => {
        if (c.id !== id) return c
        const updated = { ...c, ...patch }
        if (patch.field && patch.field !== c.field) {
          const f = dataFields.find((df) => df.slug === patch.field)
          if (f) {
            const ops = operatorsForFieldType(f.field_type)
            if (!ops.includes(updated.operator as never)) {
              updated.operator = ops[0] ?? 'eq'
            }
          }
          updated.value = ''
        }
        return updated
      }),
    })
  }

  function removeCond(id: string) {
    onChange({
      ...group,
      conditions: group.conditions.filter((c) => c.id !== id),
    })
  }

  return (
    <div className="space-y-1">
      {group.conditions.map((cond) => (
        <ConditionRow
          key={cond.id}
          cond={cond}
          dataFields={dataFields}
          onUpdate={(patch) => updateCond(cond.id, patch)}
          onRemove={() => removeCond(cond.id)}
        />
      ))}
      <Button variant="ghost" size="sm" className="h-7 gap-1 text-xs" onClick={addCond}>
        <Plus className="h-3 w-3" />
        조건 추가
      </Button>
    </div>
  )
}

function ValueInput({
  field,
  value,
  onChange,
  isEmpty,
}: {
  field?: Field
  value: string
  onChange: (v: string) => void
  isEmpty?: boolean
}) {
  const ft = field?.field_type

  if ((ft === 'select' || ft === 'multiselect') && field?.options?.choices) {
    const choices = field.options.choices as string[]
    return (
      <Select value={value} onValueChange={(v) => v && onChange(v)}>
        <SelectTrigger className="w-[160px] h-8 text-sm">
          <SelectValue placeholder="값 선택" />
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

  if (ft === 'boolean') {
    return (
      <Select value={value} onValueChange={(v) => v && onChange(v)}>
        <SelectTrigger className="w-[160px] h-8 text-sm">
          <SelectValue placeholder="값 선택" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="true">예</SelectItem>
          <SelectItem value="false">아니오</SelectItem>
        </SelectContent>
      </Select>
    )
  }

  if (ft === 'date') {
    return (
      <Input
        type="date"
        className="w-[160px] h-8 text-sm"
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
    )
  }

  if (ft === 'datetime') {
    return (
      <Input
        type="datetime-local"
        className="w-[200px] h-8 text-sm"
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
    )
  }

  if (ft === 'number' || ft === 'integer') {
    return (
      <Input
        type="number"
        className={`w-[160px] h-8 text-sm ${isEmpty ? 'border-destructive/50' : ''}`}
        placeholder="값 입력"
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
    )
  }

  return (
    <Input
      className={`w-[160px] h-8 text-sm ${isEmpty ? 'border-destructive/50' : ''}`}
      placeholder="값 입력"
      value={value}
      onChange={(e) => onChange(e.target.value)}
    />
  )
}

// --- Helpers ---

function updateGroupConditions(
  group: FilterGroup,
  condId: string,
  mapper: (c: FilterCondition) => FilterCondition,
): FilterGroup {
  return {
    ...group,
    conditions: group.conditions.map(mapper),
    groups: group.groups.map((sg) => updateGroupConditions(sg, condId, mapper)),
  }
}

function removeFromGroup(group: FilterGroup, condId: string): FilterGroup {
  return {
    ...group,
    conditions: group.conditions.filter((c) => c.id !== condId),
    groups: group.groups.map((sg) => removeFromGroup(sg, condId)),
  }
}

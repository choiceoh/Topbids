import { ArrowDown, ArrowUp, Plus, Trash2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { isLayoutType } from '@/lib/constants'
import type { Field } from '@/lib/types'

export interface SortItem {
  field: string
  desc: boolean
}

interface Props {
  fields: Field[]
  sorts: SortItem[]
  onChange: (sorts: SortItem[]) => void
}

export default function SortPanel({ fields, sorts, onChange }: Props) {
  const dataFields = fields.filter((f) => !isLayoutType(f.field_type))
  const usedSlugs = new Set(sorts.map((s) => s.field))

  function addSort() {
    const available = dataFields.find((f) => !usedSlugs.has(f.slug))
    if (!available) return
    onChange([...sorts, { field: available.slug, desc: false }])
  }

  function updateSort(index: number, patch: Partial<SortItem>) {
    onChange(sorts.map((s, i) => (i === index ? { ...s, ...patch } : s)))
  }

  function removeSort(index: number) {
    onChange(sorts.filter((_, i) => i !== index))
  }

  return (
    <div className="space-y-2">
      {sorts.map((sort, i) => {
        return (
          <div key={i} className="flex items-center gap-2">
            {sorts.length > 1 && (
              <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-medium text-muted-foreground">
                {i + 1}
              </span>
            )}
            <Select
              value={sort.field}
              onValueChange={(v) => v && updateSort(i, { field: v })}
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
            <Button
              variant="outline"
              size="sm"
              className="h-8 gap-1"
              onClick={() => updateSort(i, { desc: !sort.desc })}
              aria-label={sort.desc ? '오름차순으로 변경' : '내림차순으로 변경'}
            >
              {sort.desc ? (
                <>
                  <ArrowDown className="h-3 w-3" /> 내림차순
                </>
              ) : (
                <>
                  <ArrowUp className="h-3 w-3" /> 오름차순
                </>
              )}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="h-8 w-8 p-0 text-muted-foreground hover:text-destructive"
              onClick={() => removeSort(i)}
              aria-label="정렬 삭제"
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        )
      })}
      <Button
        variant="outline"
        size="sm"
        onClick={addSort}
        className="gap-1"
        disabled={usedSlugs.size >= dataFields.length}
      >
        <Plus className="h-3 w-3" />
        정렬 추가
      </Button>
    </div>
  )
}

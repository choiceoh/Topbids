import { Check, ChevronsUpDown, X } from 'lucide-react'
import { useMemo, useState } from 'react'

import { Badge } from '@/components/ui/badge'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { useCollection } from '@/hooks/useCollections'
import { useEntries } from '@/hooks/useEntries'
import { cn } from '@/lib/utils'

interface Props {
  targetCollectionId: string
  value: string[]
  onChange: (value: string[]) => void
  placeholder?: string
  disabled?: boolean
}

// RelationMultiCombobox lets the user pick multiple rows from another collection.
// Used for M:N (many-to-many) relation fields.
export default function RelationMultiCombobox({
  targetCollectionId,
  value,
  onChange,
  placeholder = '선택',
  disabled,
}: Props) {
  const [open, setOpen] = useState(false)

  const { data: target } = useCollection(targetCollectionId)
  const shouldFetch = open || (value && value.length > 0)
  const { data: entries } = useEntries(shouldFetch ? target?.slug : undefined, { limit: 100 })

  const labelField = useMemo(() => {
    if (!target?.fields) return null
    const named = target.fields.find((f) => f.slug === 'name' || f.slug === 'title')
    if (named) return named.slug
    const firstText = target.fields.find((f) => f.field_type === 'text')
    return firstText?.slug ?? null
  }, [target])

  const items = useMemo(() => {
    if (!entries?.data) return []
    return entries.data.map((row) => ({
      id: String(row.id),
      label: labelField ? String(row[labelField] ?? row.id) : String(row.id),
    }))
  }, [entries, labelField])

  const selected = value ?? []

  const toggle = (id: string) => {
    if (selected.includes(id)) {
      onChange(selected.filter((v) => v !== id))
    } else {
      onChange([...selected, id])
    }
  }

  const remove = (id: string) => {
    onChange(selected.filter((v) => v !== id))
  }

  const selectedLabels = selected
    .map((id) => items.find((item) => item.id === id))
    .filter(Boolean)

  return (
    <div>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          disabled={disabled}
          className="flex w-full min-h-[38px] items-center justify-between rounded-md border border-input bg-background px-3 py-2 text-sm font-normal hover:bg-accent disabled:opacity-50"
        >
          <span className={selected.length > 0 ? '' : 'text-muted-foreground'}>
            {selected.length > 0 ? `${selected.length}개 선택됨` : placeholder}
          </span>
          <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </PopoverTrigger>
        <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0">
          <Command>
            <CommandInput placeholder="검색..." />
            <CommandList>
              <CommandEmpty>일치하는 항목이 없습니다.</CommandEmpty>
              <CommandGroup>
                {selected.length > 0 && (
                  <CommandItem
                    onSelect={() => {
                      onChange([])
                      setOpen(false)
                    }}
                    className="text-muted-foreground"
                  >
                    전체 해제
                  </CommandItem>
                )}
                {items.map((item) => (
                  <CommandItem
                    key={item.id}
                    value={item.label}
                    onSelect={() => toggle(item.id)}
                  >
                    <Check
                      className={cn(
                        'mr-2 h-4 w-4',
                        selected.includes(item.id) ? 'opacity-100' : 'opacity-0',
                      )}
                    />
                    {item.label}
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
      {selectedLabels.length > 0 && (
        <div className="mt-1.5 flex flex-wrap gap-1">
          {selectedLabels.map((item) => (
            <Badge key={item!.id} variant="secondary" className="gap-1 pr-1">
              {item!.label}
              <button
                type="button"
                className="rounded-full p-0.5 hover:bg-muted"
                onClick={() => remove(item!.id)}
              >
                <X className="h-3 w-3" />
              </button>
            </Badge>
          ))}
        </div>
      )}
    </div>
  )
}

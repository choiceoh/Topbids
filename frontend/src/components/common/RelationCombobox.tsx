import { Check, ChevronsUpDown } from 'lucide-react'
import { useMemo, useState } from 'react'

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
  value: string | undefined
  onChange: (value: string | null) => void
  placeholder?: string
  disabled?: boolean
}

// RelationCombobox lets the user pick one row from another collection.
// It loads the target collection's metadata to find a sensible label field
// (the first text field, or "name", or falls back to id), then fetches up
// to 100 rows to populate the dropdown.
export default function RelationCombobox({
  targetCollectionId,
  value,
  onChange,
  placeholder = '선택',
  disabled,
}: Props) {
  const [open, setOpen] = useState(false)

  const { data: target } = useCollection(targetCollectionId)
  // Only fetch entries when the dropdown is open or a value is selected (to display label).
  const shouldFetch = open || !!value
  const { data: entries } = useEntries(shouldFetch ? target?.slug : undefined, { limit: 100 })

  // Pick a label field: prefer "name" or "title", otherwise the first text field.
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

  const selectedLabel = items.find((item) => item.id === value)?.label ?? placeholder

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        disabled={disabled}
        className="flex w-full items-center justify-between rounded-md border border-input bg-background px-3 py-2 text-sm font-normal hover:bg-accent disabled:opacity-50"
      >
        <span className={value ? '' : 'text-muted-foreground'}>{selectedLabel}</span>
        <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
      </PopoverTrigger>
      <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0">
        <Command>
          <CommandInput placeholder="검색..." />
          <CommandList>
            <CommandEmpty>일치하는 항목이 없습니다.</CommandEmpty>
            <CommandGroup>
              {value && (
                <CommandItem
                  onSelect={() => {
                    onChange(null)
                    setOpen(false)
                  }}
                  className="text-muted-foreground"
                >
                  선택 해제
                </CommandItem>
              )}
              {items.map((item) => (
                <CommandItem
                  key={item.id}
                  value={item.label}
                  onSelect={() => {
                    onChange(item.id)
                    setOpen(false)
                  }}
                >
                  <Check
                    className={cn('mr-2 h-4 w-4', value === item.id ? 'opacity-100' : 'opacity-0')}
                  />
                  {item.label}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

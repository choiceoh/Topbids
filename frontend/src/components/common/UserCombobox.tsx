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
import { useUsers } from '@/hooks/useUsers'
import { cn } from '@/lib/utils'

interface Props {
  value: string | undefined
  onChange: (value: unknown) => void
  placeholder?: string
  disabled?: boolean
}

export default function UserCombobox({
  value,
  onChange,
  placeholder = '사용자 선택',
  disabled,
}: Props) {
  const [open, setOpen] = useState(false)
  const { data: users = [] } = useUsers()

  const items = useMemo(
    () => users.map((u) => ({ id: u.id, label: `${u.name} (${u.email})` })),
    [users],
  )

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
          <CommandInput placeholder="이름 검색..." />
          <CommandList>
            <CommandEmpty>일치하는 사용자가 없습니다.</CommandEmpty>
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

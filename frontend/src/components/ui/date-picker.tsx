import { format, parseISO } from 'date-fns'
import { ko } from 'date-fns/locale'
import { CalendarIcon } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'

interface DatePickerProps {
  value: string | undefined
  onChange: (date: string | undefined) => void
  placeholder?: string
}

export function DatePicker({ value, onChange, placeholder = '날짜 선택' }: DatePickerProps) {
  const date = value ? parseISO(value) : undefined

  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            variant="outline"
            className={cn(
              'w-full justify-start text-left font-normal',
              !date && 'text-muted-foreground',
            )}
          />
        }
      >
        <CalendarIcon className="mr-2 size-4" />
        {date ? format(date, 'yyyy-MM-dd', { locale: ko }) : placeholder}
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align="start">
        <Calendar
          mode="single"
          selected={date}
          onSelect={(d) => {
            if (d) {
              onChange(format(d, 'yyyy-MM-dd'))
            } else {
              onChange(undefined)
            }
          }}
          autoFocus
        />
      </PopoverContent>
    </Popover>
  )
}

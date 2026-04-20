import { Calendar, GanttChart } from 'lucide-react'

import type { Field } from '@/lib/types'

interface Props {
  fields: Field[]
}

interface Hint {
  icon: React.ReactNode
  label: string
  when: (fields: Field[]) => boolean
  message: string
}

const HINTS: Hint[] = [
  {
    icon: <Calendar className="h-3.5 w-3.5" />,
    label: '캘린더',
    when: (fields) => !fields.some((f) => f.field_type === 'date' || f.field_type === 'datetime'),
    message: '날짜 항목을 추가하면 캘린더 보기를 사용할 수 있습니다.',
  },
  {
    icon: <GanttChart className="h-3.5 w-3.5" />,
    label: '간트',
    when: (fields) => fields.filter((f) => f.field_type === 'date' || f.field_type === 'datetime').length < 1,
    message: '날짜 항목을 추가하면 간트 차트를 사용할 수 있습니다.',
  },
]

export default function ViewGuide({ fields }: Props) {
  const active = HINTS.filter((h) => h.when(fields))
  if (active.length === 0) return null

  return (
    <div className="mt-3 space-y-1.5">
      <p className="text-xs font-medium text-muted-foreground">더 많은 보기를 사용하려면</p>
      {active.map((h) => (
        <div key={h.label} className="flex items-start gap-2 rounded-md bg-muted/50 px-3 py-2 text-xs text-muted-foreground">
          {h.icon}
          <span>{h.message}</span>
        </div>
      ))}
    </div>
  )
}

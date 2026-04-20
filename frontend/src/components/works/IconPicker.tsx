import { useState } from 'react'

import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { APP_ICONS, AppIcon, DEFAULT_ICON } from './AppCard'

const ICON_ENTRIES = Object.keys(APP_ICONS)

const ICON_LABELS: Record<string, string> = {
  clipboard: '클립보드',
  document: '문서',
  tool: '도구',
  calendar: '캘린더',
  chart: '차트',
  check: '체크',
  users: '사용자',
  cart: '장바구니',
  mail: '메일',
  building: '건물',
  folder: '폴더',
  briefcase: '서류가방',
  book: '책',
  globe: '글로벌',
  heart: '하트',
  star: '별',
  zap: '번개',
  shield: '방패',
  bell: '알림',
  tag: '태그',
  layers: '레이어',
  package: '패키지',
  truck: '배송',
  card: '카드',
  settings: '설정',
  database: '데이터베이스',
}

export default function IconPicker({
  value,
  onChange,
}: {
  value?: string
  onChange: (icon: string) => void
}) {
  const [open, setOpen] = useState(false)

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        className="inline-flex h-9 w-9 items-center justify-center rounded-md border border-input bg-transparent transition-colors hover:bg-accent"
      >
        <AppIcon name={value || DEFAULT_ICON} className="h-4 w-4" />
      </PopoverTrigger>
      <PopoverContent className="w-64 p-2" align="start">
        <div className="grid grid-cols-6 gap-1">
          {ICON_ENTRIES.map((key) => (
            <button
              key={key}
              type="button"
              title={ICON_LABELS[key] || key}
              onClick={() => {
                onChange(key)
                setOpen(false)
              }}
              className={`flex h-9 w-9 items-center justify-center rounded-md transition-colors hover:bg-accent ${
                (value || DEFAULT_ICON) === key
                  ? 'bg-primary/10 text-primary ring-1 ring-primary/30'
                  : 'text-muted-foreground'
              }`}
            >
              <AppIcon name={key} className="h-4 w-4" />
            </button>
          ))}
        </div>
      </PopoverContent>
    </Popover>
  )
}

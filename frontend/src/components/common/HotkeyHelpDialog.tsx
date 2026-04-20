import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const isMac = typeof navigator !== 'undefined' && /Mac/.test(navigator.userAgent)
const mod = isMac ? '⌘' : 'Ctrl'

const SHORTCUT_GROUPS = [
  {
    title: '전역',
    shortcuts: [
      { keys: `${mod}+K`, label: '빠른 검색 / 이동' },
      { keys: '?', label: '단축키 도움말' },
    ],
  },
  {
    title: '데이터',
    shortcuts: [
      { keys: `${mod}+N`, label: '새 항목 추가' },
      { keys: `${mod}+F`, label: '검색 포커스' },
      { keys: `${mod}+Z`, label: '되돌리기' },
      { keys: `${mod}+Shift+Z`, label: '다시 실행' },
    ],
  },
  {
    title: '테이블',
    shortcuts: [
      { keys: '↑↓←→', label: '셀 이동' },
      { keys: 'Enter', label: '셀 편집' },
      { keys: 'Escape', label: '편집 취소' },
      { keys: `${mod}+C / ${mod}+V`, label: '복사 / 붙여넣기' },
    ],
  },
]

export default function HotkeyHelpDialog({ open, onOpenChange }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>키보드 단축키</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-2">
          {SHORTCUT_GROUPS.map((group) => (
            <div key={group.title}>
              <h4 className="mb-2 text-xs font-medium uppercase text-muted-foreground">
                {group.title}
              </h4>
              <div className="space-y-1.5">
                {group.shortcuts.map((s) => (
                  <div key={s.keys} className="flex items-center justify-between text-sm">
                    <span>{s.label}</span>
                    <kbd className="rounded border bg-muted px-1.5 py-0.5 text-xs font-mono">
                      {s.keys}
                    </kbd>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}

import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router'

import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'
import {
  BarChart3,
  Clipboard,
  GitBranch,
  LayoutGrid,
  Plus,
  Settings,
  User,
  Users,
  Zap,
} from 'lucide-react'
import { useCollections } from '@/hooks/useCollections'

export default function CommandPalette() {
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()
  const { data: collections } = useCollections()

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        setOpen((o) => !o)
      }
    }
    document.addEventListener('keydown', onKeyDown)
    return () => document.removeEventListener('keydown', onKeyDown)
  }, [])

  function go(path: string) {
    navigate(path)
    setOpen(false)
  }

  return (
    <CommandDialog open={open} onOpenChange={setOpen}>
      <CommandInput placeholder="검색 또는 이동... (⌘K)" />
      <CommandList>
        <CommandEmpty>결과가 없습니다.</CommandEmpty>

        {collections && collections.length > 0 && (
          <CommandGroup heading="앱">
            {collections.map((c) => (
              <CommandItem
                key={c.id}
                value={`${c.label} ${c.slug}`}
                onSelect={() => go(`/apps/${c.id}`)}
              >
                <Clipboard className="mr-2 h-4 w-4" />
                {c.label}
              </CommandItem>
            ))}
          </CommandGroup>
        )}

        <CommandSeparator />

        <CommandGroup heading="페이지">
          <CommandItem value="앱 목록" onSelect={() => go('/apps')}>
            <LayoutGrid className="mr-2 h-4 w-4" />
            앱 목록
          </CommandItem>
          <CommandItem value="대시보드" onSelect={() => go('/dashboard')}>
            <BarChart3 className="mr-2 h-4 w-4" />
            대시보드
          </CommandItem>
          <CommandItem value="조직도" onSelect={() => go('/admin/org')}>
            <GitBranch className="mr-2 h-4 w-4" />
            조직도
          </CommandItem>
          <CommandItem value="자동화" onSelect={() => go('/automations')}>
            <Zap className="mr-2 h-4 w-4" />
            자동화
          </CommandItem>
          <CommandItem value="사용자 관리" onSelect={() => go('/admin/users')}>
            <Users className="mr-2 h-4 w-4" />
            사용자 관리
          </CommandItem>
          <CommandItem value="설정" onSelect={() => go('/settings')}>
            <Settings className="mr-2 h-4 w-4" />
            설정
          </CommandItem>
          <CommandItem value="프로필" onSelect={() => go('/profile')}>
            <User className="mr-2 h-4 w-4" />
            프로필
          </CommandItem>
        </CommandGroup>

        <CommandSeparator />

        <CommandGroup heading="작업">
          <CommandItem value="새 앱 만들기" onSelect={() => go('/apps?new=1')}>
            <Plus className="mr-2 h-4 w-4" />
            새 앱 만들기
          </CommandItem>
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  )
}

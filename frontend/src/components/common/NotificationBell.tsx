import { useState } from 'react'
import { useNavigate } from 'react-router'

import { ArrowRightLeft, Bell, MessageSquare } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Badge } from '@/components/ui/badge'
import { useNotifications, useUnreadCount, useMarkRead, useMarkAllRead } from '@/hooks/useNotifications'
import { useSSE } from '@/hooks/useSSE'

export default function NotificationBell() {
  useSSE()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const { data: unread } = useUnreadCount()
  const { data: notifData } = useNotifications()
  const markRead = useMarkRead()
  const markAllRead = useMarkAllRead()

  const count = unread?.count ?? 0

  function handleClick(n: { id: string; is_read: boolean; type: string; ref_collection_id?: string; ref_record_id?: string }) {
    if (!n.is_read) markRead.mutate(n.id)
    // Deep link to entry for state_change and comment notifications.
    if (n.ref_collection_id && n.ref_record_id) {
      setOpen(false)
      navigate(`/apps/${n.ref_collection_id}?entry=${n.ref_record_id}`)
    }
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger className="relative inline-flex items-center justify-center rounded-md px-2 py-1 text-sm hover:bg-stone-100">
        <Bell className="h-4 w-4" />
        {count > 0 && (
          <Badge
            variant="destructive"
            className="absolute -top-1 -right-1 flex h-5 w-5 items-center justify-center p-0 text-[10px]"
          >
            {count > 99 ? '99+' : count}
          </Badge>
        )}
      </PopoverTrigger>
      <PopoverContent className="w-80 p-0" align="end">
        <div className="flex items-center justify-between border-b p-3">
          <span className="text-sm font-semibold">알림</span>
          {count > 0 && (
            <Button
              variant="ghost"
              size="sm"
              className="h-6 text-xs"
              onClick={() => markAllRead.mutate()}
            >
              모두 읽음
            </Button>
          )}
        </div>
        <div className="max-h-80 overflow-y-auto">
          {notifData?.data?.length ? (
            notifData.data.map((n) => (
              <button
                key={n.id}
                className={`w-full border-b p-3 text-left text-sm hover:bg-muted/50 ${
                  !n.is_read ? 'bg-blue-50' : ''
                }`}
                onClick={() => handleClick(n)}
              >
                <div className="flex items-center gap-1.5 font-medium">
                  {n.type === 'state_change' && (
                    <ArrowRightLeft className="h-3.5 w-3.5 text-blue-500 shrink-0" />
                  )}
                  {n.type === 'comment' && (
                    <MessageSquare className="h-3.5 w-3.5 text-gray-500 shrink-0" />
                  )}
                  {n.title}
                </div>
                {n.body && (
                  <div className="mt-0.5 truncate text-xs text-muted-foreground">
                    {n.body}
                  </div>
                )}
                <div className="mt-1 text-xs text-muted-foreground">
                  {new Date(n.created_at).toLocaleString('ko')}
                </div>
              </button>
            ))
          ) : (
            <div className="p-4 text-center text-sm text-muted-foreground">
              알림이 없습니다
            </div>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}

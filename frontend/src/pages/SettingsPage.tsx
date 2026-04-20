import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Globe,
  Key,
  Trash2,
  User as UserIcon,
  Webhook,
} from 'lucide-react'
import { toast } from 'sonner'

import ErrorState from '@/components/common/ErrorState'
import PageHeader from '@/components/common/PageHeader'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { api } from '@/lib/api'
import { queryKeys } from '@/lib/queryKeys'
import type { WebhookEvent } from '@/lib/types'
import { useCurrentUser } from '@/hooks/useAuth'

// ---------------------------------------------------------------------------
// Profile tab
// ---------------------------------------------------------------------------

function ProfileSection() {
  const { data: user, isLoading } = useCurrentUser()
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [phone, setPhone] = useState('')
  const [initialized, setInitialized] = useState(false)

  // Seed local state once user loads.
  if (user && !initialized) {
    setName(user.name ?? '')
    setPhone(user.phone ?? '')
    setInitialized(true)
  }

  const update = useMutation({
    mutationFn: (input: { name?: string, phone?: string }) =>
      api.patch<{ status: string }>('/auth/me', input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.auth.me() })
      toast.success('프로필이 저장되었습니다')
    },
    onError: () => toast.error('프로필 저장에 실패했습니다'),
  })

  const changePw = useMutation({
    mutationFn: (input: { current_password: string, new_password: string }) =>
      api.post<void>('/auth/password', input),
    onSuccess: () => {
      toast.success('비밀번호가 변경되었습니다')
      setCurrentPw('')
      setNewPw('')
      setConfirmPw('')
    },
    onError: () => toast.error('비밀번호 변경에 실패했습니다'),
  })

  const [currentPw, setCurrentPw] = useState('')
  const [newPw, setNewPw] = useState('')
  const [confirmPw, setConfirmPw] = useState('')

  if (isLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-10 w-full" />
        <Skeleton className="h-10 w-full" />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      {/* Basic info */}
      <Card className="p-6">
        <div className="flex items-start gap-3 mb-4">
          <UserIcon className="mt-0.5 h-5 w-5 text-muted-foreground" />
          <div>
            <h3 className="font-medium">기본 정보</h3>
            <p className="text-sm text-muted-foreground">이름, 연락처 등 기본 프로필을 관리합니다</p>
          </div>
        </div>
        <Separator className="mb-4" />
        <div className="grid gap-4 sm:grid-cols-2 max-w-lg">
          <div className="space-y-2">
            <Label htmlFor="email">이메일</Label>
            <Input id="email" value={user?.email ?? ''} disabled />
          </div>
          <div className="space-y-2">
            <Label htmlFor="role">역할</Label>
            <Input id="role" value={user?.role ?? ''} disabled />
          </div>
          <div className="space-y-2">
            <Label htmlFor="name">이름</Label>
            <Input id="name" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="phone">연락처</Label>
            <Input id="phone" value={phone} onChange={(e) => setPhone(e.target.value)} />
          </div>
        </div>
        <div className="mt-4">
          <Button
            size="sm"
            disabled={update.isPending}
            onClick={() => update.mutate({ name, phone })}
          >
            {update.isPending ? '저장 중…' : '저장'}
          </Button>
        </div>
      </Card>

      {/* Password change */}
      <Card className="p-6">
        <div className="flex items-start gap-3 mb-4">
          <Key className="mt-0.5 h-5 w-5 text-muted-foreground" />
          <div>
            <h3 className="font-medium">비밀번호 변경</h3>
            <p className="text-sm text-muted-foreground">보안을 위해 주기적으로 변경하세요</p>
          </div>
        </div>
        <Separator className="mb-4" />
        <div className="grid gap-4 max-w-sm">
          <div className="space-y-2">
            <Label htmlFor="current-pw">현재 비밀번호</Label>
            <Input id="current-pw" type="password" value={currentPw} onChange={(e) => setCurrentPw(e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="new-pw">새 비밀번호</Label>
            <Input id="new-pw" type="password" value={newPw} onChange={(e) => setNewPw(e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="confirm-pw">새 비밀번호 확인</Label>
            <Input id="confirm-pw" type="password" value={confirmPw} onChange={(e) => setConfirmPw(e.target.value)} />
          </div>
        </div>
        <div className="mt-4">
          <Button
            size="sm"
            disabled={changePw.isPending || !currentPw || !newPw || newPw !== confirmPw}
            onClick={() => changePw.mutate({ current_password: currentPw, new_password: newPw })}
          >
            {changePw.isPending ? '변경 중…' : '비밀번호 변경'}
          </Button>
          {newPw && confirmPw && newPw !== confirmPw && (
            <p className="mt-2 text-sm text-destructive">비밀번호가 일치하지 않습니다</p>
          )}
        </div>
      </Card>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Webhook events tab (director only)
// ---------------------------------------------------------------------------

function WebhookSection() {
  const [page, setPage] = useState(1)
  const qc = useQueryClient()

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: queryKeys.webhooks.list({ page }),
    queryFn: () => api.getList<WebhookEvent>(`/webhooks?page=${page}&limit=20`),
  })

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.del(`/webhooks/${id}`),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.webhooks.all })
      toast.success('이벤트가 삭제되었습니다')
    },
  })

  const events = data?.data ?? []
  const totalPages = data?.total_pages ?? 1

  return (
    <div className="space-y-4">
      <Card className="p-6">
        <div className="flex items-start gap-3 mb-4">
          <Globe className="mt-0.5 h-5 w-5 text-muted-foreground" />
          <div>
            <h3 className="font-medium">웹훅 수신 설정</h3>
            <p className="text-sm text-muted-foreground">
              외부 시스템에서 <code className="rounded bg-muted px-1 text-xs">POST /api/hooks/&#123;topic&#125;</code>으로
              이벤트를 수신합니다. HMAC-SHA256 서명 검증은 <code className="rounded bg-muted px-1 text-xs">WEBHOOK_SECRET</code> 환경변수로 활성화됩니다.
            </p>
          </div>
        </div>
      </Card>

      <Card className="p-6">
        <div className="flex items-start gap-3 mb-4">
          <Webhook className="mt-0.5 h-5 w-5 text-muted-foreground" />
          <div>
            <h3 className="font-medium">수신 이벤트 로그</h3>
            <p className="text-sm text-muted-foreground">최근 수신된 웹훅 이벤트 목록입니다</p>
          </div>
        </div>
        <Separator className="mb-4" />

        {isError ? (
          <ErrorState error={error} onRetry={() => refetch()} />
        ) : isLoading ? (
          <div className="space-y-2">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        ) : events.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">수신된 웹훅 이벤트가 없습니다</p>
        ) : (
          <>
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[180px]">수신 시각</TableHead>
                    <TableHead className="w-[120px]">토픽</TableHead>
                    <TableHead className="w-[120px]">소스</TableHead>
                    <TableHead className="w-[80px]">상태</TableHead>
                    <TableHead>페이로드</TableHead>
                    <TableHead className="w-[60px]" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {events.map((evt) => (
                    <TableRow key={evt.id}>
                      <TableCell className="text-xs text-muted-foreground">
                        {new Date(evt.received_at).toLocaleString('ko-KR')}
                      </TableCell>
                      <TableCell>
                        <Badge variant="outline">{evt.topic}</Badge>
                      </TableCell>
                      <TableCell className="text-xs">{evt.source || '-'}</TableCell>
                      <TableCell>
                        <Badge variant={evt.processed ? 'default' : 'secondary'}>
                          {evt.processed ? '처리됨' : '대기'}
                        </Badge>
                      </TableCell>
                      <TableCell className="max-w-[300px] truncate text-xs font-mono text-muted-foreground">
                        {JSON.stringify(evt.payload)}
                      </TableCell>
                      <TableCell>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-7 w-7"
                          onClick={() => deleteMut.mutate(evt.id)}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>

            {/* Pagination */}
            {totalPages > 1 && (
              <div className="flex items-center justify-end gap-2 pt-4">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page <= 1}
                  onClick={() => setPage((p) => p - 1)}
                >
                  이전
                </Button>
                <span className="text-sm text-muted-foreground">
                  {page} / {totalPages}
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={page >= totalPages}
                  onClick={() => setPage((p) => p + 1)}
                >
                  다음
                </Button>
              </div>
            )}
          </>
        )}
      </Card>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------

export default function SettingsPage() {
  const { data: user } = useCurrentUser()
  const isDirector = user?.role === 'director'

  return (
    <div>
      <PageHeader
        title="설정"
        description="프로필 및 워크스페이스 설정을 관리합니다"
      />

      <Tabs defaultValue="profile">
        <TabsList variant="line" className="mb-4">
          <TabsTrigger value="profile">프로필</TabsTrigger>
          {isDirector && <TabsTrigger value="webhooks">웹훅</TabsTrigger>}
        </TabsList>

        <TabsContent value="profile">
          <ProfileSection />
        </TabsContent>

        {isDirector && (
          <TabsContent value="webhooks">
            <WebhookSection />
          </TabsContent>
        )}
      </Tabs>
    </div>
  )
}

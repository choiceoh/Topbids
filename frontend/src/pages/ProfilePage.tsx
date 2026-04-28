import { useEffect, useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { KeyRound, Mail, Shield, User } from 'lucide-react'
import { FormProvider, useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'

import { FormField } from '@/components/common/Form'
import LoadingState from '@/components/common/LoadingState'
import PageHeader from '@/components/common/PageHeader'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { useCurrentUser } from '@/hooks/useAuth'
import { useDepartments } from '@/hooks/useDepartments'
import { useSubsidiaries } from '@/hooks/useSubsidiaries'
import { useUpdateMe, useChangePassword } from '@/hooks/useUsers'
import { formatError } from '@/lib/api'
import { ROLE_LABELS } from '@/lib/constants'

const profileSchema = z.object({
  name: z.string().min(1, '이름을 입력하세요'),
  phone: z.string().optional(),
  position: z.string().optional(),
  title: z.string().optional(),
})

const passwordSchema = z
  .object({
    current_password: z.string().min(1, '현재 비밀번호를 입력하세요'),
    new_password: z.string().min(6, '6자 이상 입력하세요'),
    confirm_password: z.string().min(1, '비밀번호 확인을 입력하세요'),
  })
  .refine((v) => v.new_password === v.confirm_password, {
    message: '비밀번호가 일치하지 않습니다',
    path: ['confirm_password'],
  })

type ProfileForm = z.infer<typeof profileSchema>
type PasswordForm = z.infer<typeof passwordSchema>

const NOTIF_PREFS_KEY = 'topbid_notification_prefs'

function loadNotifPrefs() {
  try {
    const raw = localStorage.getItem(NOTIF_PREFS_KEY)
    if (raw) return JSON.parse(raw) as { desktop: boolean; sound: boolean }
  } catch { /* ignore */ }
  return { desktop: false, sound: true }
}

function saveNotifPrefs(prefs: { desktop: boolean; sound: boolean }) {
  localStorage.setItem(NOTIF_PREFS_KEY, JSON.stringify(prefs))
}

export default function ProfilePage() {
  const { data: user, isLoading } = useCurrentUser()
  const { data: departments } = useDepartments()
  const { data: subsidiaries } = useSubsidiaries()
  const updateMe = useUpdateMe()
  const changePw = useChangePassword()

  const [notifPrefs, setNotifPrefs] = useState(loadNotifPrefs)

  const profileForm = useForm<ProfileForm>({
    resolver: zodResolver(profileSchema),
    values: { name: user?.name ?? '', phone: user?.phone ?? '', position: user?.position ?? '', title: user?.title ?? '' },
  })

  const pwForm = useForm<PasswordForm>({
    resolver: zodResolver(passwordSchema),
    defaultValues: { current_password: '', new_password: '', confirm_password: '' },
  })

  useEffect(() => {
    saveNotifPrefs(notifPrefs)
  }, [notifPrefs])

  const deptName = user?.department_id
    ? departments?.find((d) => d.id === user.department_id)?.name
    : undefined
  const subName = user?.subsidiary_id
    ? subsidiaries?.find((s) => s.id === user.subsidiary_id)?.name
    : undefined

  function onProfileSubmit(values: ProfileForm) {
    updateMe.mutate(values, {
      onSuccess: () => toast.success('프로필이 수정되었습니다'),
      onError: (err) => toast.error(formatError(err)),
    })
  }

  function onPasswordSubmit(values: PasswordForm) {
    changePw.mutate(
      { current_password: values.current_password, new_password: values.new_password },
      {
        onSuccess: () => {
          toast.success('비밀번호가 변경되었습니다')
          pwForm.reset()
        },
        onError: (err) => toast.error(formatError(err)),
      },
    )
  }

  async function handleDesktopToggle(checked: boolean) {
    if (checked && Notification.permission !== 'granted') {
      const perm = await Notification.requestPermission()
      if (perm !== 'granted') {
        toast.error('브라우저 알림 권한이 거부되었습니다')
        return
      }
    }
    setNotifPrefs((prev) => ({ ...prev, desktop: checked }))
  }

  if (isLoading) return <LoadingState />
  if (!user) return null

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <PageHeader title="내 정보" />

      {/* Profile header card */}
      <div className="flex items-center gap-5 rounded-lg border border-stone-200 bg-white p-6">
        <div className="relative">
          <Avatar className="h-20 w-20">
            <AvatarFallback className="bg-stone-900 text-2xl font-semibold text-white">
              {user.name.slice(0, 1)}
            </AvatarFallback>
          </Avatar>
        </div>
        <div className="min-w-0 flex-1">
          <h2 className="text-lg font-semibold text-stone-900">{user.name}</h2>
          <div className="mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-stone-500">
            <span className="flex items-center gap-1">
              <Mail className="h-3.5 w-3.5" />
              {user.email}
            </span>
            <span className="flex items-center gap-1">
              <Shield className="h-3.5 w-3.5" />
              {ROLE_LABELS[user.role] ?? user.role}
            </span>
          </div>
          {(subName || deptName) && (
            <div className="mt-1.5 flex flex-wrap items-center gap-x-3 text-sm text-stone-400">
              {subName && <span>{subName}</span>}
              {subName && deptName && <span>/</span>}
              {deptName && <span>{deptName}</span>}
            </div>
          )}
          {user.joined_at && (
            <p className="mt-1 text-xs text-stone-400">입사일 {user.joined_at}</p>
          )}
        </div>
      </div>

      {/* Profile edit */}
      <section className="rounded-lg border border-stone-200 bg-white p-6">
        <div className="mb-5 flex items-center gap-2">
          <User className="h-4 w-4 text-stone-500" />
          <h2 className="text-sm font-semibold">기본 정보 수정</h2>
        </div>
        <FormProvider {...profileForm}>
          <form onSubmit={profileForm.handleSubmit(onProfileSubmit)} className="space-y-4">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <FormField<ProfileForm> name="name" label="이름" required>
                <Input {...profileForm.register('name')} />
              </FormField>
              <FormField<ProfileForm> name="phone" label="전화번호">
                <Input type="tel" {...profileForm.register('phone')} />
              </FormField>
              <FormField<ProfileForm> name="position" label="직위">
                <Input {...profileForm.register('position')} />
              </FormField>
              <FormField<ProfileForm> name="title" label="직책">
                <Input {...profileForm.register('title')} />
              </FormField>
            </div>
            <div className="flex justify-end">
              <Button type="submit" disabled={updateMe.isPending}>
                {updateMe.isPending ? '저장 중...' : '저장'}
              </Button>
            </div>
          </form>
        </FormProvider>
      </section>

      {/* Notification preferences */}
      <section className="rounded-lg border border-stone-200 bg-white p-6">
        <div className="mb-5 flex items-center gap-2">
          <svg className="h-4 w-4 text-stone-500" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/></svg>
          <h2 className="text-sm font-semibold">알림 설정</h2>
        </div>
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <div>
              <Label htmlFor="desktop-notif" className="text-sm font-medium">데스크톱 알림</Label>
              <p className="text-xs text-stone-400">브라우저 푸시 알림을 받습니다</p>
            </div>
            <Switch
              id="desktop-notif"
              checked={notifPrefs.desktop}
              onCheckedChange={handleDesktopToggle}
            />
          </div>
          <div className="flex items-center justify-between">
            <div>
              <Label htmlFor="sound-notif" className="text-sm font-medium">알림 소리</Label>
              <p className="text-xs text-stone-400">새 알림이 올 때 소리로 알려줍니다</p>
            </div>
            <Switch
              id="sound-notif"
              checked={notifPrefs.sound}
              onCheckedChange={(checked) => setNotifPrefs((prev) => ({ ...prev, sound: checked }))}
            />
          </div>
        </div>
      </section>

      {/* Password change */}
      <section className="rounded-lg border border-stone-200 bg-white p-6">
        <div className="mb-5 flex items-center gap-2">
          <KeyRound className="h-4 w-4 text-stone-500" />
          <h2 className="text-sm font-semibold">비밀번호 변경</h2>
        </div>
        <FormProvider {...pwForm}>
          <form onSubmit={pwForm.handleSubmit(onPasswordSubmit)} className="space-y-4">
            <FormField<PasswordForm> name="current_password" label="현재 비밀번호" required>
              <Input type="password" {...pwForm.register('current_password')} />
            </FormField>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <FormField<PasswordForm> name="new_password" label="새 비밀번호" required>
                <Input type="password" {...pwForm.register('new_password')} />
              </FormField>
              <FormField<PasswordForm> name="confirm_password" label="비밀번호 확인" required>
                <Input type="password" {...pwForm.register('confirm_password')} />
              </FormField>
            </div>
            <div className="flex justify-end">
              <Button type="submit" disabled={changePw.isPending}>
                {changePw.isPending ? '변경 중...' : '비밀번호 변경'}
              </Button>
            </div>
          </form>
        </FormProvider>
      </section>
    </div>
  )
}

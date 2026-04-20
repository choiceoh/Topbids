import { zodResolver } from '@hookform/resolvers/zod'
import { FormProvider, useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'

import { FormField } from '@/components/common/Form'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useCreateUser, useUpdateUser } from '@/hooks/useUsers'
import { formatError } from '@/lib/api'
import { ROLE_LABELS } from '@/lib/constants'
import type { Department, Subsidiary, User } from '@/lib/types'

const NONE = '__none__'

const createSchema = z.object({
  email: z.string().email('이메일 형식이 올바르지 않습니다'),
  name: z.string().min(1, '이름을 입력하세요'),
  password: z.string().min(6, '6자 이상 입력하세요'),
  role: z.enum(['director', 'pm', 'engineer', 'viewer']),
  subsidiary_id: z.string().optional(),
  department_id: z.string().optional(),
  position: z.string().optional(),
  title: z.string().optional(),
  phone: z.string().optional(),
  joined_at: z.string().optional(),
})

const editSchema = z.object({
  email: z.string().email('이메일 형식이 올바르지 않습니다'),
  name: z.string().min(1, '이름을 입력하세요'),
  password: z.string().optional(),
  role: z.enum(['director', 'pm', 'engineer', 'viewer']),
  subsidiary_id: z.string().optional(),
  department_id: z.string().optional(),
  position: z.string().optional(),
  title: z.string().optional(),
  phone: z.string().optional(),
  joined_at: z.string().optional(),
})

type CreateForm = z.infer<typeof createSchema>
type EditForm = z.infer<typeof editSchema>

interface Props {
  user?: User
  departments: Department[]
  subsidiaries: Subsidiary[]
  onClose: () => void
  onToggleActive?: () => void
}

export default function UserFormDialog({ user, departments, subsidiaries, onClose, onToggleActive }: Props) {
  const isEdit = !!user
  const createUser = useCreateUser()
  const updateUser = useUpdateUser()

  const form = useForm<CreateForm | EditForm>({
    resolver: zodResolver(isEdit ? editSchema : createSchema),
    defaultValues: {
      email: user?.email ?? '',
      name: user?.name ?? '',
      password: '',
      role: user?.role ?? 'viewer',
      subsidiary_id: user?.subsidiary_id ?? NONE,
      department_id: user?.department_id ?? NONE,
      position: user?.position ?? '',
      title: user?.title ?? '',
      phone: user?.phone ?? '',
      joined_at: user?.joined_at ?? '',
    },
  })

  function onSubmit(values: CreateForm | EditForm) {
    const deptId = values.department_id === NONE ? '' : values.department_id
    const subId = values.subsidiary_id === NONE ? '' : values.subsidiary_id

    if (isEdit) {
      updateUser.mutate(
        {
          id: user!.id,
          email: values.email,
          name: values.name,
          role: values.role,
          subsidiary_id: subId || null,
          department_id: deptId || null,
          position: values.position ?? '',
          title: values.title ?? '',
          phone: values.phone ?? '',
          joined_at: values.joined_at || null,
          ...(values.password ? { password: values.password } : {}),
        },
        {
          onSuccess: () => {
            toast.success('사용자 정보가 수정되었습니다')
            onClose()
          },
          onError: (err) => toast.error(formatError(err)),
        },
      )
    } else {
      const v = values as CreateForm
      createUser.mutate(
        {
          email: v.email,
          name: v.name,
          password: v.password,
          role: v.role,
          subsidiary_id: subId || null,
          department_id: deptId || null,
          position: v.position ?? '',
          title: v.title ?? '',
          phone: v.phone ?? '',
          joined_at: v.joined_at || null,
        },
        {
          onSuccess: () => {
            toast.success('사용자가 생성되었습니다')
            onClose()
          },
          onError: (err) => toast.error(formatError(err)),
        },
      )
    }
  }

  const isPending = createUser.isPending || updateUser.isPending

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{isEdit ? '사용자 편집' : '사용자 추가'}</DialogTitle>
        </DialogHeader>
        <FormProvider {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            <div className="grid grid-cols-2 gap-4">
              <FormField<CreateForm | EditForm> name="name" label="이름" required>
                <Input {...form.register('name')} />
              </FormField>
              <FormField<CreateForm | EditForm> name="email" label="이메일" required>
                <Input type="email" {...form.register('email')} />
              </FormField>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <FormField<CreateForm | EditForm> name="password" label="비밀번호" required={!isEdit}>
                <Input
                  type="password"
                  placeholder={isEdit ? '변경 시에만 입력' : ''}
                  {...form.register('password')}
                />
              </FormField>
              <FormField<CreateForm | EditForm> name="role" label="역할" required>
                <Select
                  value={form.watch('role')}
                  onValueChange={(v) => form.setValue('role', v as CreateForm['role'])}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {Object.entries(ROLE_LABELS).map(([value, label]) => (
                      <SelectItem key={value} value={value}>
                        {label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </FormField>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <FormField<CreateForm | EditForm> name="subsidiary_id" label="계열사">
                <Select
                  value={form.watch('subsidiary_id') || NONE}
                  onValueChange={(v) => {
                    form.setValue('subsidiary_id', v ?? NONE)
                    form.setValue('department_id', NONE)
                  }}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NONE}>미지정</SelectItem>
                    {subsidiaries.map((s) => (
                      <SelectItem key={s.id} value={s.id}>
                        {s.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </FormField>
              <FormField<CreateForm | EditForm> name="department_id" label="부서">
                <Select
                  value={form.watch('department_id') || NONE}
                  onValueChange={(v) => form.setValue('department_id', v ?? NONE)}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={NONE}>미지정</SelectItem>
                    {departments
                      .filter((d) => {
                        const selectedSub = form.watch('subsidiary_id')
                        if (!selectedSub || selectedSub === NONE) return true
                        return d.subsidiary_id === selectedSub
                      })
                      .map((d) => (
                        <SelectItem key={d.id} value={d.id}>
                          {d.name}
                        </SelectItem>
                      ))}
                  </SelectContent>
                </Select>
              </FormField>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <FormField<CreateForm | EditForm> name="position" label="직위">
                <Input placeholder="예: 대리, 과장" {...form.register('position')} />
              </FormField>
              <FormField<CreateForm | EditForm> name="title" label="직책">
                <Input placeholder="예: 팀장, 본부장" {...form.register('title')} />
              </FormField>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <FormField<CreateForm | EditForm> name="phone" label="전화번호">
                <Input type="tel" {...form.register('phone')} />
              </FormField>
              <FormField<CreateForm | EditForm> name="joined_at" label="입사일">
                <Input type="date" {...form.register('joined_at')} />
              </FormField>
            </div>

            <DialogFooter className="gap-2">
              {isEdit && onToggleActive && (
                <Button
                  type="button"
                  variant="outline"
                  onClick={onToggleActive}
                  className="mr-auto"
                >
                  {user?.is_active ? '비활성화' : '활성화'}
                </Button>
              )}
              <Button type="button" variant="ghost" onClick={onClose}>
                취소
              </Button>
              <Button type="submit" disabled={isPending}>
                {isPending ? '저장 중...' : '저장'}
              </Button>
            </DialogFooter>
          </form>
        </FormProvider>
      </DialogContent>
    </Dialog>
  )
}

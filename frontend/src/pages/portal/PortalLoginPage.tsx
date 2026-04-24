import { zodResolver } from '@hookform/resolvers/zod'
import { FormProvider, useForm } from 'react-hook-form'
import { toast } from 'sonner'
import { z } from 'zod'

import { FormField } from '@/components/common/Form'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useLogin } from '@/hooks/useAuth'
import { formatError } from '@/lib/api'

const schema = z.object({
  email: z.string().email('이메일 형식이 올바르지 않습니다'),
  password: z.string().min(1, '비밀번호를 입력하세요'),
})

type LoginForm = z.infer<typeof schema>

/**
 * Supplier-facing login page. Identical auth mechanics to the internal
 * LoginPage — the useLogin hook's role-based redirect sends suppliers to
 * `/portal` and internal users to `/`, so even if an internal user lands
 * here they'll end up in the right place after submitting.
 *
 * Copy and navigation target are the only surface-level differences; we
 * keep it as a distinct component so portal branding can evolve without
 * touching the internal login path.
 */
export default function PortalLoginPage() {
  const form = useForm<LoginForm>({
    resolver: zodResolver(schema),
    defaultValues: { email: '', password: '' },
  })
  const login = useLogin()

  function onSubmit(values: LoginForm) {
    login.mutate(values, {
      onError: (err) => toast.error(formatError(err)),
    })
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-4">
      <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_top,oklch(0.94_0.01_260/0.3),transparent_70%)]" />
      <FormProvider {...form}>
        <form
          onSubmit={form.handleSubmit(onSubmit)}
          className="relative w-full max-w-sm space-y-6 rounded-2xl border border-border/60 bg-white p-10 shadow-premium-lg animate-scale-in"
        >
          <div className="space-y-1.5">
            <div className="flex items-center gap-2.5">
              <img src="/logo.png" alt="Topbids" className="h-8 w-8" />
              <h1 className="text-xl font-bold tracking-tight text-foreground">Topbids</h1>
            </div>
            <p className="text-sm text-muted-foreground">공급사 포털 · 입찰 참여 전용</p>
          </div>

          <FormField<LoginForm> name="email" label="이메일" required>
            <Input type="email" autoComplete="email" {...form.register('email')} />
          </FormField>

          <FormField<LoginForm> name="password" label="비밀번호" required>
            <Input
              type="password"
              autoComplete="current-password"
              {...form.register('password')}
            />
          </FormField>

          <Button type="submit" disabled={login.isPending} className="w-full h-9">
            {login.isPending ? '로그인 중...' : '로그인'}
          </Button>

          <p className="text-center text-xs text-muted-foreground">
            계정 관련 문의는 구매 담당자에게 연락해 주세요.
          </p>
        </form>
      </FormProvider>
    </div>
  )
}

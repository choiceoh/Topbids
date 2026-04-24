import { useEffect } from 'react'
import { LogOut } from 'lucide-react'
import { Link, NavLink, Outlet, useNavigate } from 'react-router'

import LoadingState from '@/components/common/LoadingState'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { useCurrentUser, useLogout } from '@/hooks/useAuth'

/**
 * Supplier-facing portal chrome. Intentionally minimal — no app builder, no
 * admin controls, no AI. Only the three supplier-relevant surfaces: RFQ list,
 * bid submission, past bids.
 *
 * Access is gated to `role='supplier'`. Non-suppliers are redirected to the
 * internal SPA so a bookmarked `/portal` URL doesn't get past the wrong
 * landing page.
 */
export default function PortalLayout() {
  const { data: user, isLoading, isError } = useCurrentUser()
  const logout = useLogout()
  const navigate = useNavigate()

  useEffect(() => {
    if (isError) navigate('/portal/login', { replace: true })
  }, [isError, navigate])

  // Internal users (director/pm/engineer/viewer) don't belong on the portal.
  // Redirect to the regular SPA rather than showing an empty shell.
  useEffect(() => {
    if (user && user.role !== 'supplier') navigate('/', { replace: true })
  }, [user, navigate])

  if (isLoading) return <LoadingState />
  if (!user || user.role !== 'supplier') return null // redirect in flight

  const navCls = ({ isActive }: { isActive: boolean }) =>
    `rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
      isActive ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground'
    }`

  return (
    <div className="min-h-screen bg-background">
      <header className="sticky top-0 z-30 flex items-center justify-between border-b border-border/60 bg-white/80 px-6 py-2.5 backdrop-blur-md">
        <div className="flex items-center gap-6">
          <Link
            to="/portal"
            className="flex items-center gap-2 text-lg font-bold tracking-tight text-foreground"
          >
            <img src="/logo.png" alt="Topbids" className="h-6 w-6 grayscale" />
            <span>공급사 포털</span>
          </Link>
          <nav className="flex items-center gap-0.5">
            <NavLink to="/portal" end className={navCls}>
              공고
            </NavLink>
            <NavLink to="/portal/history" className={navCls}>
              내 입찰 내역
            </NavLink>
          </nav>
        </div>
        <div className="flex items-center gap-3 text-sm text-muted-foreground">
          <div className="flex items-center gap-2">
            <Avatar className="h-7 w-7">
              <AvatarFallback className="bg-foreground text-[11px] font-medium text-white">
                {user.name.slice(0, 1)}
              </AvatarFallback>
            </Avatar>
            <span className="text-foreground">{user.name}</span>
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => logout.mutate()}
            className="h-8 gap-1.5 text-muted-foreground"
          >
            <LogOut className="h-4 w-4" />
            로그아웃
          </Button>
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-6 py-8">
        <Outlet />
      </main>
    </div>
  )
}

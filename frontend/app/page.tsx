import { DesktopTitlebar } from '@/components/DesktopTitlebar'
import { LoginForm } from '@/components/login-form'
export default function LoginPage() {
  return (
    <div className="flex min-h-screen flex-col bg-[linear-gradient(135deg,#f8fafc_0%,#eef4ff_38%,#f8f5ff_100%)] dark:bg-gradient-to-br dark:from-slate-950 dark:via-slate-900 dark:to-slate-800">
      <DesktopTitlebar />
      <div className="relative flex min-h-0 flex-1 overflow-y-auto overflow-x-hidden">
        <div className="absolute inset-0 pointer-events-none bg-[radial-gradient(circle_at_14%_18%,rgba(59,130,246,0.14),transparent_32%),radial-gradient(circle_at_84%_12%,rgba(99,102,241,0.12),transparent_30%),radial-gradient(circle_at_72%_86%,rgba(251,191,36,0.10),transparent_36%)] dark:bg-[radial-gradient(circle_at_15%_20%,rgba(59,130,246,0.18),transparent_45%),radial-gradient(circle_at_80%_10%,rgba(244,114,182,0.14),transparent_40%),radial-gradient(circle_at_70%_85%,rgba(234,179,8,0.12),transparent_45%)]" />
        <div className="absolute inset-0 pointer-events-none bg-[linear-gradient(180deg,rgba(255,255,255,0.55),rgba(255,255,255,0.08))] dark:hidden" />
        <div className="auth-page-content relative z-10 flex min-h-full w-full justify-center px-4 py-4">
          <LoginForm />
        </div>
      </div>
    </div>
  )
}

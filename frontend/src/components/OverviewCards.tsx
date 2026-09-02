import type { ReactNode } from 'react'
import type { StatusPayload } from '../api'

type Engine = StatusPayload['engine']

const engineName: Record<Engine, string> = {
  napcat: 'NapCat',
  luckylillia: 'LuckyLillia',
  snowluma: 'SnowLuma'
}

export type OverviewHealth = {
  tone: 'success' | 'warning' | 'danger' | 'neutral'
  label: string
  detail: string
}

export function getOverviewHealth(status: StatusPayload | null): OverviewHealth {
  if (!status) return { tone: 'neutral', label: '正在读取状态', detail: '正在从运行时读取当前状态。' }
  if (status.error) return { tone: 'danger', label: '需要处理', detail: status.error }
  if (!status.installed) return { tone: 'neutral', label: '尚未安装', detail: '安装核心后即可继续完成 QQ 登录与连接配置。' }
  if (!status.running) return { tone: 'warning', label: '服务已停止', detail: '启动服务后可继续登录 QQ 或连接 OneBot。' }
  if (status.loginPending) return { tone: 'warning', label: '等待 QQ 登录', detail: '请使用手机 QQ 扫码，完成后状态会自动更新。' }
  if (status.oneBotReady) return { tone: 'success', label: '运行正常', detail: 'QQ 与 OneBot 已就绪，可同步到机器人。' }
  if (status.qqLoggedIn) return { tone: 'warning', label: '等待 OneBot', detail: 'QQ 已登录，请完成或检查 OneBot 连接配置。' }
  return { tone: 'warning', label: '正在连接', detail: '核心正在准备 QQ 登录和 OneBot 服务。' }
}

export function JourneyCard({
  engine,
  title,
  description,
  action
}: {
  engine: Engine
  title: string
  description: string
  action: ReactNode
}) {
  return (
    <section className="grid gap-4 rounded-panel border border-[var(--theme-border-strong)] bg-[var(--theme-surface-panel)] p-4 shadow-[var(--theme-shadow-panel)]">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="grid gap-1.5">
          <span className="text-xs font-semibold uppercase tracking-[0.08em] text-[var(--theme-text-muted)]">{engineName[engine]} 控制台</span>
          <h1 className="m-0 text-lg font-semibold text-[var(--theme-text-strong)]">{title}</h1>
          <p className="m-0 max-w-2xl text-xs leading-5 text-[var(--theme-text-muted)]">{description}</p>
        </div>
        <div className="shrink-0">{action}</div>
      </div>
    </section>
  )
}

export function HealthSummary({
  status,
  webAction
}: {
  status: StatusPayload
  webAction?: ReactNode
}) {
  const services = [
    { label: '服务', value: !status?.installed ? '未安装' : status.running ? '运行中' : '已停止' },
    { label: 'QQ', value: status?.loginPending ? '等待扫码' : status?.qqLoggedIn ? '已登录' : '未连接' },
    { label: 'OneBot', value: status?.oneBotReady ? '已连接' : '未就绪' }
  ]
  return (
    <section aria-label="核心健康状态" className="grid gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3">
      {webAction && (
        <div className="flex justify-end">{webAction}</div>
      )}
      <div className="grid grid-cols-3 divide-x divide-[var(--theme-border-default)] rounded-md border border-[var(--theme-border-default)] bg-[var(--theme-surface-input)]">
        {services.map(service => (
          <div key={service.label} className="grid gap-0.5 px-3 py-2 text-xs">
            <span className="text-[var(--theme-text-muted)]">{service.label}</span>
            <strong className="font-semibold text-[var(--theme-text-secondary)]">{service.value}</strong>
          </div>
        ))}
      </div>
    </section>
  )
}

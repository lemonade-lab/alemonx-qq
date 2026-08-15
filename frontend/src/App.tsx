import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { chooseSystemPath, fetchHostRobotContext, fetchLocalServices, fetchOperationLog, fetchPluginLog, fetchPrivilegedAudit, fetchRobotProjects, fetchStatus, luckyQRCodeURL, napcatQRCodeURL, privilegePreflight, runActionAndPoll, syncRobotOneBot, type ActionResult, type LocalService, type PrivilegedAuditItem, type RobotProject, type StatusPayload, type Task, type TaskStep } from './api'
import { splitStatusLines, type StatusLine } from './status'
import { loadSession, saveSession, type QQEngine, type QQView } from './session'

type View = QQView
type Engine = QQEngine
type ResultOrigin = 'manage' | 'config-read' | 'config-http' | 'config-ws' | 'config-lucky' | 'config-auth' | 'config-sync' | null

const statusColor: Record<StatusLine['kind'], string> = {
  ok: 'text-[var(--theme-success-text)]',
  fail: 'text-[var(--theme-danger-text)]',
  warn: 'text-[var(--theme-warning-text)]',
  plain: 'text-[var(--theme-text-primary)]'
}

const statusSymbol: Record<StatusLine['kind'], string> = {
  ok: '✓',
  fail: '!',
  warn: '?',
  plain: ''
}

function StatusLineRow({ line }: { line: StatusLine }) {
  return (
    <div
      className={
        'grid grid-cols-[1.125rem_1fr] items-baseline gap-1.5 py-1 text-xs leading-5 ' +
        statusColor[line.kind]
      }
    >
      <span className="text-center font-bold" aria-hidden>
        {statusSymbol[line.kind]}
      </span>
		<span
		  className={
			'min-w-0 [overflow-wrap:anywhere] ' + (line.kind === 'plain' ? 'whitespace-pre-wrap font-mono' : '')
		  }
		>
		  {line.text || ' '}
		</span>
    </div>
  )
}

function ResultPanel({
  state,
  result,
  steps = [],
  liveDetail = '',
  onViewLog,
  compact = false
}: {
  state: 'idle' | 'running' | 'done' | 'failed'
  result?: ActionResult
  steps?: TaskStep[]
  liveDetail?: string
  onViewLog?: () => void
  compact?: boolean
}) {
  const lines = useMemo(() => splitStatusLines(result?.output ?? ''), [result])
	const current = steps.at(-1)
  if (state === 'idle') return null
  return (
    <section role="status" aria-live="polite" className={'grid min-w-0 gap-2 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] text-xs ' + (compact ? 'p-2.5' : 'p-3')}>
      <header className="flex items-center justify-between gap-3">
        <strong className="flex items-center gap-1.5 text-sm font-semibold text-[var(--theme-text-strong)]">
          {state === 'running' && (
            <span className="size-3.5 animate-spin rounded-full border-2 border-[var(--theme-border-strong)] border-t-[var(--theme-accent)]" />
          )}
          {state === 'running'
            ? '正在执行…'
            : state === 'failed'
              ? '操作未完成'
              : '操作完成'}
        </strong>
      </header>
      {state === 'failed' && result?.error && (
        <div className="grid gap-2 rounded-md bg-[var(--theme-danger-soft)] px-2 py-2 text-[var(--theme-danger-text)]">
          <strong>操作失败</strong>
          <pre className="m-0 max-h-48 min-w-0 max-w-full overflow-auto whitespace-pre-wrap [overflow-wrap:anywhere] font-mono text-xs font-normal leading-5">
            {result.error}
          </pre>
          {onViewLog && (
            <div>
              <button className="secondary-button" onClick={onViewLog}>查看完整日志</button>
            </div>
          )}
        </div>
      )}
      {current && (
        <div className="grid gap-1.5 rounded-md border border-[var(--theme-border-default)] bg-[var(--theme-surface-input)] px-2.5 py-2">
          <div className="flex items-center justify-between gap-3 text-xs">
            <span className="min-w-0 truncate text-[var(--theme-text-secondary)]">{current.message}</span>
            <span className="shrink-0 font-mono text-[var(--theme-text-muted)]">{current.progress}%</span>
          </div>
          <div className="h-1.5 overflow-hidden rounded-full bg-[var(--theme-surface-hover)]">
            <div className="h-full rounded-full bg-[var(--theme-accent)] transition-[width] duration-300" style={{ width: `${current.progress}%` }} />
          </div>
        </div>
      )}
			{state === 'running' && liveDetail && (
				<div className="grid gap-1.5 rounded-md border border-[var(--theme-border-default)] bg-[var(--theme-surface-input)] p-2.5">
					<strong className="text-xs text-[var(--theme-text-secondary)]">当前执行详情</strong>
					<pre className={'m-0 min-w-0 max-w-full overflow-auto whitespace-pre-wrap [overflow-wrap:anywhere] font-mono text-xs leading-5 text-[var(--theme-text-secondary)] ' + (compact ? 'max-h-32' : 'max-h-56')}>{liveDetail}</pre>
				</div>
			)}
			{steps.length > 1 && (
				<details className="text-xs text-[var(--theme-text-muted)]">
					<summary className="cursor-pointer">已完成步骤（{steps.length}）</summary>
					<div className="mt-1 grid min-w-0 max-w-full gap-1 [overflow-wrap:anywhere] font-mono">{steps.map((step, index) => <div key={`${step.at}-${index}`}>{step.progress}% {step.message}</div>)}</div>
				</details>
			)}
      {state === 'running' && !result?.output ? (
        <div className="grid gap-2 py-1">
          <div className="h-3 w-2/3 animate-pulse rounded bg-[var(--theme-surface-hover)]" />
          <div className="h-3 w-1/2 animate-pulse rounded bg-[var(--theme-surface-hover)]" />
        </div>
      ) : (
        <div className="grid">
          {lines.map((line, index) => (
            <StatusLineRow key={index} line={line} />
          ))}
        </div>
      )}
    </section>
  )
}

const inputClass =
  'min-h-9 rounded-control border border-[var(--theme-border-default)] bg-[var(--theme-surface-input)] px-2.5 text-sm font-normal text-[var(--theme-text-primary)] outline-none focus:border-[var(--theme-accent)]'

function Field({
  label,
  name,
  type = 'text',
  defaultValue,
  children
}: {
  label: string
  name: string
  type?: string
  defaultValue?: string
  children?: ReactNode
}) {
  return (
    <label className="grid min-w-[180px] flex-1 gap-1 text-xs font-semibold text-[var(--theme-text-secondary)]">
      {label}
      {children ?? (
        <input
          className={inputClass}
          type={type}
          name={name}
          defaultValue={defaultValue}
        />
      )}
    </label>
  )
}

function ActionField({ label = '', children }: { label?: string; children: ReactNode }) {
	return (
		<label className="grid min-w-[180px] flex-1 gap-1 text-xs font-semibold text-[var(--theme-text-secondary)] [&>button]:w-full">
			<span>{label}</span>
			{children}
		</label>
	)
}

function ConfirmModal({
  title,
  description,
  onConfirm,
  onCancel,
  tone = 'primary'
}: {
  title: string
  description: string
  onConfirm: () => void
  onCancel: () => void
  tone?: 'primary' | 'danger'
}) {
  const confirmRef = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    confirmRef.current?.focus()
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onCancel()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('keydown', onKeyDown)
      previous?.focus()
    }
  }, [onCancel])
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-[var(--theme-surface-overlay)]" role="dialog" aria-modal="true" aria-label={title}>
      <div className="w-[min(460px,calc(100vw-32px))] rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-4 shadow-[var(--theme-shadow-pop)]">
        <h3 className="m-0 mb-1 text-[15px] font-semibold text-[var(--theme-text-strong)]">
          {title}
        </h3>
        <p className="m-0 mb-3 text-[13px] text-[var(--theme-text-muted)]">
          {description}
        </p>
        <div className="flex justify-end gap-2">
          <button className="secondary-button" onClick={onCancel}>
            取消
          </button>
          <button ref={confirmRef} className={tone === 'danger' ? 'danger-button' : 'primary-button'} onClick={onConfirm}>
            确认执行
          </button>
        </div>
      </div>
    </div>
  )
}

function LogModal({
  text,
  onClose,
  autoRefresh,
  onToggleAutoRefresh,
  onRefresh,
  title
}: {
  text: string
  onClose: () => void
  autoRefresh: boolean
  onToggleAutoRefresh: () => void
  onRefresh: () => void
  title: string
}) {
  const closeRef = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    closeRef.current?.focus()
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('keydown', onKeyDown)
      previous?.focus()
    }
  }, [onClose])
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-[var(--theme-surface-overlay)] p-4" role="dialog" aria-modal="true" aria-label="实时安装日志">
      <div className="grid min-w-0 max-h-[min(680px,calc(100vh-32px))] w-[min(760px,100%)] gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-4 shadow-[var(--theme-shadow-pop)]">
        <div className="flex items-center justify-between gap-3">
          <strong className="text-sm text-[var(--theme-text-strong)]">{title}</strong>
          <div className="flex gap-2">
            <button className="secondary-button" onClick={onToggleAutoRefresh}>{autoRefresh ? '自动刷新：开' : '自动刷新：关'}</button>
            <button className="secondary-button" onClick={onRefresh}>刷新</button>
            <button ref={closeRef} className="secondary-button" onClick={onClose}>关闭</button>
          </div>
        </div>
        <pre className="m-0 max-h-[560px] min-w-0 max-w-full overflow-auto whitespace-pre-wrap [overflow-wrap:anywhere] rounded-md bg-[var(--theme-surface-input)] p-3 font-mono text-xs leading-5 text-[var(--theme-text-secondary)]">{text}</pre>
      </div>
    </div>
  )
}

function SudoModal({
  title,
  description,
  busy,
  onConfirm,
  onCancel
}: {
  title: string
  description: string
  busy: boolean
  onConfirm: (password: string) => void
  onCancel: () => void
}) {
  const passwordRef = useRef<HTMLInputElement>(null)
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    passwordRef.current?.focus()
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onCancel()
    }
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('keydown', onKeyDown)
      previous?.focus()
    }
  }, [onCancel])
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-[var(--theme-surface-overlay)]" role="dialog" aria-modal="true" aria-label={title}>
      <form className="grid w-[min(460px,calc(100vw-32px))] gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-4 shadow-[var(--theme-shadow-pop)]" onSubmit={event => {
        event.preventDefault()
        onConfirm(passwordRef.current?.value || '')
      }}>
        <h3 className="m-0 mb-1 text-[15px] font-semibold text-[var(--theme-text-strong)]">{title}</h3>
        <p className="m-0 text-[13px] leading-5 text-[var(--theme-text-muted)]">{description}</p>
        <label className="grid gap-1 text-xs font-semibold text-[var(--theme-text-secondary)]">
         系统 sudo 密码
          <input ref={passwordRef} className={inputClass} type="password" name="sudoPassword" autoComplete="current-password" placeholder="root 或已配置免密 sudo 时可留空" />
        </label>
        <div className="flex justify-end gap-2">
          <button type="button" className="secondary-button" onClick={onCancel} disabled={busy}>取消</button>
          <button type="submit" className="primary-button" disabled={busy}>{busy ? '正在执行…' : '执行'}</button>
        </div>
      </form>
    </div>
  )
}

function ActionButton({
  label,
  variant = 'primary',
  running,
	disabled = false,
  onClick,
  title
}: {
  label: string
  variant?: 'primary' | 'secondary' | 'danger'
  running: boolean
	disabled?: boolean
  onClick: () => void
  title?: string
}) {
  const className =
    variant === 'primary'
      ? 'primary-button'
      : variant === 'danger'
        ? 'danger-button'
        : 'secondary-button'
  return (
    <button className={className} disabled={running || disabled} onClick={onClick} title={title}>
      {label}
    </button>
  )
}

function actionTitle(action: string | null) {
	if (!action) return ''
	if (action.includes('reinstall')) return '正在重装并保留现有配置'
	if (action === 'luckylillia-install') return '正在下载、验证并准备 LuckyLillia'
	if (action === 'luckylillia-auth-token-set' || action === 'luckylillia-auth-token-set-start') return '正在保存 Auth Token 并启动 LuckyLillia'
	if (action === 'snowluma-install') return '正在下载并准备 SnowLuma 原生内核'
	if (action === 'snowluma-update') return '正在更新 SnowLuma 原生内核'
	if (action === 'install') return '正在下载、安装并启动 NapCat'
	if (action === 'start' || action.endsWith('-start')) return '正在启动 QQ 服务并等待二维码'
	if (action === 'stop' || action.endsWith('-stop')) return '正在停止核心服务'
	if (action.includes('uninstall')) return '正在卸载核心'
	if (action.includes('forget')) return '正在取消关联'
	if (action.includes('adopt')) return '正在关联现有安装'
	if (action.includes('log-clear')) return '正在清理日志'
	if (action === 'log' || action.endsWith('-log')) return '正在读取日志'
	if (action.includes('update')) return '正在检查或更新核心版本'
	if (action === 'onebot-config' || action === 'luckylillia-onebot-config') return '正在读取 OneBot 配置'
	if (action.includes('onebot')) return '正在保存 OneBot 连接配置'
	return '正在执行操作'
}

function engineLabel(engine: Engine) {
	return engine === 'napcat' ? 'NapCat' : engine === 'luckylillia' ? 'LuckyLillia' : 'SnowLuma'
}

function engineAction(engine: Engine, action: string) {
	return engine === 'napcat' ? action : `${engine}-${action}`
}

export default function App() {
	const [initialSession] = useState(loadSession)
  const [view, setView] = useState<View>(initialSession.view)
	const [engine, setEngine] = useState<Engine>(initialSession.engine)
  const [result, setResult] = useState<ActionResult | undefined>()
	const [operationSteps, setOperationSteps] = useState<TaskStep[]>([])
	const [activeAction, setActiveAction] = useState<string | null>(null)
	const actionInFlight = useRef(false)
  const [state, setState] = useState<'idle' | 'running' | 'done' | 'failed'>(
    'idle'
  )
	const [pendingConfirm, setPendingConfirm] = useState<{
    title: string
    description: string
    tone?: 'primary' | 'danger'
    action: () => Promise<void>
	} | null>(null)
	const [sudoRequest, setSudoRequest] = useState<{ action: string; title: string; description: string; intentId: string } | null>(null)
	const [logText, setLogText] = useState<string | null>(null)
	const [logAutoRefresh, setLogAutoRefresh] = useState(true)
	const [qrLoadFailed, setQrLoadFailed] = useState(false)
	const [operationDetail, setOperationDetail] = useState('')
	const [resultOrigin, setResultOrigin] = useState<ResultOrigin>(null)
	const [activeEngine, setActiveEngine] = useState<Engine>(engine)
	const [statusByEngine, setStatusByEngine] = useState<Partial<Record<Engine, StatusPayload>>>({})
	const [statusLoading, setStatusLoading] = useState<Partial<Record<Engine, boolean>>>({})
	const liveStatus = statusByEngine[engine] ?? null
	const [projects, setProjects] = useState<RobotProject[]>([])
	const [services, setServices] = useState<LocalService[]>([])
	const [auditItems, setAuditItems] = useState<PrivilegedAuditItem[]>([])
	const [auditLoading, setAuditLoading] = useState(false)
	const [robotRoot, setRobotRoot] = useState(initialSession.robotRoot)
	const webServiceID = engine === 'napcat' ? 'napcat-webui' : engine === 'luckylillia' ? 'luckylillia-webui' : 'snowluma-webui'
	const webService = services.find(service => service.id === webServiceID)
	const webUrl = webService?.reachable && webService.embed ? webService.proxyUrl : ''
	const qrImageUrl = liveStatus?.qrCodeAvailable
		? engine === 'napcat'
			? napcatQRCodeURL(liveStatus.qrCodeUpdatedAt)
			: luckyQRCodeURL(liveStatus.qrCodeUpdatedAt)
		: ''
	const qrAgeSeconds = useMemo(() => {
		if (!liveStatus?.qrCodeUpdatedAt) return null
		const updated = Date.parse(liveStatus.qrCodeUpdatedAt)
		if (Number.isNaN(updated)) return null
		return Math.max(0, (Date.now() - updated) / 1000)
	}, [liveStatus?.qrCodeUpdatedAt])
	const qrStale = liveStatus?.loginPending === true && qrAgeSeconds != null && qrAgeSeconds > 150
	useEffect(() => { setQrLoadFailed(false) }, [qrImageUrl])
	const luckyAction = (action: string) => `luckylillia-${action}`
	const snowLumaAction = (action: string) => `snowluma-${action}`
	const applyOperationTask = (task: Task) => {
		const output = task.output || (task.progress ? `正在执行（${task.progress}%）` : '正在执行…')
		setResult({ output })
		if (task.steps) setOperationSteps(task.steps)
	}
	const luckyManaged = liveStatus?.managed === true
	const luckyInstalled = liveStatus?.installed === true
	const napcatManagedActions = engine === 'napcat' && liveStatus?.managed === true
	const [napcatQQ, setNapcatQQ] = useState(initialSession.napcatQQ)
	useEffect(() => {
		saveSession({ version: 1, engine, view, robotRoot, napcatQQ })
	}, [engine, view, robotRoot, napcatQQ])
	const selectedNapcatAccount = liveStatus?.accounts?.find(account => account.qq === (napcatQQ || liveStatus.selectedAccount))
	const selectedOneBotReady = engine === 'napcat' ? Boolean(selectedNapcatAccount?.oneBotReady) : Boolean(liveStatus?.oneBotReady)
	const selectedOneBotURL = engine === 'napcat'
		? selectedNapcatAccount?.oneBotUrl
		: liveStatus?.oneBotUrl
	const coreNeedsInstall = liveStatus?.installed === false
	const nativeLauncherNapcat = engine === 'napcat' && (liveStatus?.platform === 'darwin-external' || liveStatus?.platform === 'windows-external')
	// While a lifecycle task runs, logs and operation details must follow the
	// engine that owns the task, even if the user switches to the other core.
	const logEngine = state === 'running' ? activeEngine : engine

	const run = async (action: string, params: Record<string, string> = {}, confirm = false, origin: ResultOrigin = 'manage', actionEngine: Engine = engine) => {
		// State updates are asynchronous, so `state === running` alone cannot
		// stop two rapid clicks (or two confirmation clicks) from starting two
		// installations. Keep a synchronous guard in the browser as well.
		if (actionInFlight.current) return
		actionInFlight.current = true
		setActiveEngine(actionEngine)
	setActiveAction(action)
    setState('running')
		setResult(undefined)
		setOperationSteps([])
		setOperationDetail('正在创建本次操作记录…')
		setResultOrigin(origin)
    try {
		const outcome = await runActionAndPoll(action, params, confirm, task => {
		applyOperationTask(task)
	  })
      setResult(outcome)
      setState(outcome.error ? 'failed' : 'done')
    } catch (reason) {
      setResult({ output: '', error: reason instanceof Error ? reason.message : String(reason) })
      setState('failed')
	} finally {
		setActiveAction(null)
		actionInFlight.current = false
	  }
	}

	// Manifest-declared system operation (password-authorized). The host owns
	// the sudo prompt, intent validation, timeout and audit; the plugin UI only
	// forwards the one-time intent and the password for this single request.
	const runPrivileged = async (action: string, sudoPassword: string, authorizationId: string) => {
		if (actionInFlight.current) return
		actionInFlight.current = true
		setActiveEngine(engine)
		setActiveAction(action)
		setState('running')
		setResult(undefined)
		setOperationSteps([])
		setOperationDetail('正在创建本次操作记录…')
		setResultOrigin('manage')
		try {
			const outcome = await runActionAndPoll(action, {}, true, task => applyOperationTask(task), { sudoPassword, authorizationId })
			setResult(outcome)
			setState(outcome.error ? 'failed' : 'done')
		} catch (reason) {
			setResult({ output: '', error: reason instanceof Error ? reason.message : String(reason) })
			setState('failed')
		} finally {
			setActiveAction(null)
			actionInFlight.current = false
			setSudoRequest(null)
			void refreshAudit()
		}
	}

	const refreshAudit = useCallback(async () => {
		setAuditLoading(true)
		try {
			setAuditItems(await fetchPrivilegedAudit('alemonx-qq'))
		} catch {
			// Audit is auxiliary; a failure must not disturb the main UI.
		} finally {
			setAuditLoading(false)
		}
	}, [])

	const requestLinuxEnvironment = async () => {
		try {
			const preflight = await privilegePreflight('alemonx-qq', 'napcat-install-dependencies')
			if (!preflight.available) {
				setResultOrigin('manage')
				setResult({ output: '', error: preflight.reason || '该系统操作当前不可用。' })
				setState('failed')
				return
			}
			if (preflight.authorization !== 'password' || !preflight.intentId) {
				setResultOrigin('manage')
				setResult({ output: '', error: `当前环境需要「${preflight.authorization}」授权方式，请使用 ALemonX 的系统操作入口执行。` })
				setState('failed')
				return
			}
			setSudoRequest({
				action: 'napcat-install-dependencies',
				title: preflight.title,
				description: preflight.description || '将安装 QQ 登录所需的系统组件；密码只用于本次系统授权。',
				intentId: preflight.intentId
			})
		} catch (reason) {
			setResultOrigin('manage')
			setResult({ output: '', error: reason instanceof Error ? reason.message : String(reason) })
			setState('failed')
		}
	}

	// Live operation detail: while a lifecycle task runs, poll the runner's
	// per-operation trace. This must live at the component level, never inside
	// an event handler, so it is registered on every render while running.
	useEffect(() => {
		if (state !== 'running') return
		let stopped = false
		const refresh = async () => {
			try {
				const detail = await fetchOperationLog(logEngine)
				if (!stopped && detail) setOperationDetail(detail)
			} catch { /* task progress remains the primary display */ }
		}
		void refresh()
		const timer = window.setInterval(() => void refresh(), 1000)
		return () => { stopped = true; window.clearInterval(timer) }
	}, [logEngine, state])

	const openLiveLog = async () => {
		setLogText('正在读取日志…')
		try {
			setLogText(await fetchPluginLog(logEngine))
		} catch (reason) {
			setLogText(reason instanceof Error ? reason.message : String(reason))
		}
	}

	// Keep the log modal fresh while it stays open and auto-refresh is on.
	useEffect(() => {
		if (logText === null || !logAutoRefresh) return
		const timer = window.setInterval(() => {
			void fetchPluginLog(logEngine).then(text => setLogText(text)).catch(() => undefined)
		}, 2500)
		return () => window.clearInterval(timer)
	}, [logText, logAutoRefresh, logEngine])

	const requestNapcatInstall = async () => {
		await run('install', {}, true)
	}

	const refreshStatus = useCallback(async (targetEngine: Engine = engine) => {
		setStatusLoading(current => ({ ...current, [targetEngine]: true }))
		try {
			const [status, nextServices] = await Promise.all([fetchStatus(targetEngine), fetchLocalServices()])
			setStatusByEngine(current => ({ ...current, [targetEngine]: status }))
			setServices(nextServices)
			if (targetEngine === 'napcat') setNapcatQQ(current => current || status.selectedAccount || '')
		} catch {
			// Keep the last usable state while a local service is changing.
		} finally {
			setStatusLoading(current => ({ ...current, [targetEngine]: false }))
		}
	}, [engine])

	// Login QR codes are short-lived. Poll the read-only endpoint faster only
	// while login is pending; hidden pages do not need background refreshes.
  useEffect(() => {
    let stopped = false
		let timer: ReturnType<typeof setTimeout> | undefined
    const tick = async () => {
			if (document.hidden) return
			await refreshStatus()
		if (!stopped) timer = setTimeout(tick, liveStatus?.loginPending ? 2000 : 5000)
    }
    void tick()
		const onVisibility = () => {
			if (!document.hidden) void tick()
		}
		document.addEventListener('visibilitychange', onVisibility)
    return () => {
      stopped = true
		if (timer) clearTimeout(timer)
		 document.removeEventListener('visibilitychange', onVisibility)
    }
	}, [liveStatus?.loginPending, refreshStatus])

	useEffect(() => {
		void Promise.all([fetchRobotProjects(), fetchHostRobotContext().catch(() => null)])
			.then(([items, current]) => {
				setProjects(items)
				setRobotRoot(previous => previous || (current && items.some(item => item.root === current.root) ? current.root : ''))
			})
			.catch(() => setProjects([]))
	}, [])

	useEffect(() => { void refreshAudit() }, [refreshAudit])

  const confirm = (title: string, description: string, action: () => Promise<void>, tone: 'primary' | 'danger' = 'primary') => {
    setPendingConfirm({ title, description, action, tone })
  }

	const guide = useMemo(() => {
		if (!liveStatus) return {
			title: engineLabel(engine),
			description: statusLoading[engine] ? '正在读取当前状态…' : '暂时无法读取状态，点击刷新重试。',
			label: statusLoading[engine] ? '读取中' : '刷新状态',
			action: () => { if (!statusLoading[engine]) void refreshStatus() }
		}
		if (engine === 'napcat' && liveStatus.platform === 'darwin-external' && !liveStatus.installed) return liveStatus.installerReady ? {
			title: '安装 NapCat',
			description: '安装器已下载，点击打开。',
			label: '打开安装器',
			action: () => confirm('打开 NapCat 安装器', '将打开已下载的官方安装器。', () => run('napcat-macos-installer-open', {}, true)),
		} : {
			title: '安装 NapCat',
			description: '点击后自动下载官方安装器。',
			label: '安装 NapCat',
			action: () => confirm('安装 NapCat', '工作台将下载官方安装器。', () => run('napcat-macos-installer-download', {}, true)),
		}
		if (engine === 'napcat' && liveStatus.platform === 'windows-external') return liveStatus.launcherPath ? {
			title: 'NapCat 启动器',
			description: liveStatus.launcherPath,
			label: '打开 NapCat 启动器',
			action: () => confirm('打开 NapCat 启动器', '将打开官方 NapCat 启动器。', () => run('napcat-windows-launcher-open', {}, true)),
		} : {
			title: '安装 NapCat',
			description: '点击后自动下载官方图形安装器。',
			label: '安装 NapCat',
			action: () => confirm('安装 NapCat', '工作台将下载官方安装器。', () => run('napcat-windows-installer-download', {}, true)),
		}
		if (liveStatus.journey?.phase === 'repair') return {
			title: liveStatus.journey.title,
			description: liveStatus.journey.detail,
			label: '重新安装',
			action: () => engine === 'napcat'
				? void requestNapcatInstall()
				: confirm('重装 LuckyLillia', '会下载并验证官方组件，并保留已保存的私有配置。', () => run(luckyAction('reinstall'), {}, true))
		}
		if (liveStatus.journey?.phase === 'needs-auth-token') return {
			title: liveStatus.journey.title,
			description: liveStatus.journey.detail,
			label: '填写 Token',
			action: () => setView('config')
		}
		if (liveStatus.journey?.phase === 'starting' || liveStatus.journey?.phase === 'connecting') return {
			title: liveStatus.journey.title,
			description: liveStatus.journey.detail,
			label: '查看实时日志',
			action: () => void openLiveLog()
		}
		if (engine === 'napcat' && liveStatus.diagnosticHint?.includes('图形运行环境未就绪')) return {
			title: 'Linux 图形运行环境未就绪',
			description: liveStatus.diagnosticHint,
			label: '准备 QQ 登录运行环境',
			action: () => void requestLinuxEnvironment()
		}
		if (engine === 'napcat' && !liveStatus.installed) return { title: '安装 NapCat', description: '自动安装并启动。', label: '安装 NapCat', action: () => void requestNapcatInstall() }
		if (engine === 'napcat' && liveStatus.installed && liveStatus.running && !liveStatus.oneBotReady && !liveStatus.loginPending) return { title: '正在准备登录二维码', description: '核心已启动，请稍候。二维码出现后用手机 QQ 扫描即可。', label: '等待二维码', action: () => void refreshStatus() }
		if (nativeLauncherNapcat) return {
			title: 'NapCat 启动器',
			description: liveStatus.launcherPath || '使用官方启动器管理 NapCat。',
			label: '打开 NapCat 启动器',
			action: () => confirm('打开 NapCat 启动器', '启动器负责安装、启动和管理 NapCat。', () => run('napcat-macos-launcher-open', {}, true)),
		}
		if (engine === 'napcat' && liveStatus.installed && !liveStatus.managed) return { title: 'NapCat 已关联', description: webUrl ? '可以继续登录 QQ。' : '正在检查登录状态。', label: webUrl ? '打开登录页' : '刷新', action: () => webUrl ? setView('webui') : void refreshStatus() }
		if (engine === 'snowluma' && liveStatus.supported === false) return { title: '此系统暂无上游原生 Hook', description: liveStatus.diagnosticHint || 'macOS 尚无官方 Darwin Hook。', label: '查看状态', action: () => void refreshStatus() }
		if (engine === 'snowluma' && !liveStatus.installed) return { title: '安装 SnowLuma', description: '下载官方完整发行包，不使用 Docker。', label: '安装 SnowLuma', action: () => confirm('安装 SnowLuma', '将下载、验证并安装官方原生完整包。', () => run(snowLumaAction('install'), {}, true)) }
		if (engine === 'snowluma' && !liveStatus.running) return { title: '启动 SnowLuma', description: '先启动同一系统用户、同一权限级别的 QQ；Linux 还需可用的 X11/Xvfb 与 ptrace 条件。', label: '启动 SnowLuma', action: () => confirm('启动 SnowLuma', '将先验证 QQ 与运行环境，再启动工作台受管的 SnowLuma 原生进程。', () => run(snowLumaAction('start'), {}, true)) }
		if (engine === 'snowluma' && liveStatus.loginPending) return { title: '请在 QQ 窗口扫码', description: 'SnowLuma 已启动，正在等待 QQ 登录并建立 OneBot 服务。', label: '查看实时日志', action: () => void openLiveLog() }
		if (engine === 'luckylillia' && liveStatus.supported === false) return { title: '此系统暂不支持', description: '请改用 NapCat，或手动安装后关联。', label: '关联目录', action: () => document.getElementById('luckylillia-association')?.scrollIntoView({ behavior: 'smooth', block: 'center' }) }
		if (!liveStatus.installed) return { title: '安装 QQ 核心', description: engine === 'napcat' ? '自动安装并启动。' : '安装后需填写 Auth Token。', label: engine === 'napcat' ? '安装 NapCat' : '安装 LuckyLillia', action: () => engine === 'napcat' ? void requestNapcatInstall() : confirm('安装 QQ 核心', '下载并安装 LuckyLillia。', () => run(luckyAction('install'), {}, true)) }
		if (engine === 'luckylillia' && !liveStatus.managed) return { title: 'LuckyLillia 已关联', description: webUrl ? '可以继续登录 QQ。' : '正在检查登录状态。', label: webUrl ? '打开登录页' : '刷新', action: () => webUrl ? setView('webui') : void refreshStatus() }
		if (engine === 'luckylillia' && !liveStatus.authTokenReady) return { title: '需要 Auth Token', description: '在「网络配置」中填写 Token。', label: '填写 Token', action: () => setView('config') }
		if (!liveStatus.running) return { title: '启动 QQ', description: '启动后即可扫码登录。', label: engine === 'napcat' ? '启动 NapCat' : '启动 LuckyLillia', action: () => confirm('启动 QQ', '启动后请使用手机 QQ 扫码。', () => run(engine === 'napcat' ? 'start' : luckyAction('start'), {}, true)) }
		if (liveStatus.loginPending) return { title: '请用手机 QQ 扫码', description: liveStatus.qrCodeAvailable ? '扫码后自动继续。' : '正在生成二维码…', label: liveStatus.qrCodeAvailable ? '查看实时日志' : (webUrl ? '打开登录页' : '等待登录'), action: liveStatus.qrCodeAvailable ? () => void openLiveLog() : (webUrl ? () => setView('webui') : () => void refreshStatus()) }
		if (!liveStatus.oneBotReady) return { title: '正在连接', description: '请稍候。', label: '刷新', action: () => void refreshStatus() }
		return { title: 'QQ 已就绪', description: '可同步到机器人。', label: '同步到机器人', action: () => setView('config') }
	}, [engine, liveStatus, refreshStatus, statusLoading, webUrl])

	// A single result panel per origin: lifecycle tasks use the global panel at
	// the bottom, while form saves show a compact panel inside their own card.
	const localResult = (origin: Exclude<ResultOrigin, null>) => resultOrigin === origin
		? <ResultPanel compact state={state} result={result} steps={operationSteps} liveDetail={operationDetail} onViewLog={() => void openLiveLog()} />
		: null

	// The one-click sync reads the core's current WebSocket token directly from
	// its config; the intermediate token response is consumed in memory and
	// never rendered or persisted.
	const readOneBotToken = async (): Promise<string> => {
		const action = engine === 'napcat' ? 'napcat-onebot-token-read' : engine === 'luckylillia' ? 'luckylillia-onebot-token-read' : 'snowluma-onebot-token-read'
		const params: Record<string, string> = engine === 'napcat' && napcatQQ ? { qq: napcatQQ } : {}
		const outcome = await runActionAndPoll(action, params, false)
		if (outcome.error) throw new Error(outcome.error)
		try {
			const payload = JSON.parse(outcome.output || '{}') as { token?: string }
			return payload.token || ''
		} catch {
			throw new Error('无法读取核心 OneBot Token，请先保存连接配置。')
		}
	}

  return (
    <div className="qq-plugin-shell">
      <aside className="qq-plugin-sidebar" aria-label="QQ 插件导航">
        <nav className="qq-plugin-nav" aria-label="QQ 内核">
          {(['napcat', 'luckylillia', 'snowluma'] as Engine[]).map(item => (
            <button
              key={item}
              type="button"
              className="qq-plugin-nav-button"
              aria-current={engine === item ? 'page' : undefined}
              onClick={() => {
                setEngine(item)
                setView('manage')
                setResult(undefined)
                setOperationSteps([])
                setState('idle')
                setResultOrigin(null)
                setOperationDetail('')
                void refreshStatus(item)
              }}
            >
              <span className="qq-plugin-nav-icon" data-icon={item} aria-hidden />
              {engineLabel(item)}
            </button>
          ))}
        </nav>

        {!coreNeedsInstall && !nativeLauncherNapcat && (
          <nav className="qq-plugin-nav qq-plugin-view-nav" aria-label="QQ 功能">
            {(['manage', 'config', 'background', 'webui'] as View[]).map(tab => (
              <button
                key={tab}
                type="button"
                className="qq-plugin-nav-button"
                aria-current={view === tab ? 'page' : undefined}
                onClick={() => setView(tab)}
              >
                <span className="qq-plugin-nav-icon" data-icon={tab} aria-hidden />
                {tab === 'manage'
                  ? '管理'
                  : tab === 'config'
                    ? '网络配置'
                    : tab === 'background'
                      ? '后台运行'
                      : '管理面板'}
              </button>
            ))}
          </nav>
        )}
      </aside>

      <main className="qq-plugin-content">
        <div className="qq-plugin-content-inner">

      {view === 'manage' && (
        <div className="grid gap-3">

		  {guide && (
			<section className="grid gap-3 rounded-panel border border-[var(--theme-border-strong)] bg-[var(--theme-surface-panel)] p-4">
				<div className="flex flex-wrap items-start justify-between gap-3">
					<div className="grid gap-1">
						<strong className="text-sm font-semibold text-[var(--theme-text-strong)]">{guide.title}</strong>
						<p className="m-0 text-xs leading-5 text-[var(--theme-text-muted)]">{guide.description}</p>
					</div>
					<ActionButton label={state === 'running' ? actionTitle(activeAction) : guide.label} running={state === 'running'} disabled={guide.label === '等待二维码' || guide.label === '读取中'} onClick={guide.action} />
				</div>
			</section>
		  )}


		  {engine === 'luckylillia' && liveStatus?.supported === false && (
			<section id="luckylillia-association" className="grid gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3">
				<h2 className="m-0 text-sm font-semibold text-[var(--theme-text-strong)]">关联已有 LuckyLillia 安装</h2>
				<p className="m-0 text-xs leading-5 text-[var(--theme-text-muted)]">当前系统不支持自动安装，可手动解压官方 CLI 后关联。</p>
				<div>
					<ActionButton label="选择目录并关联" variant="secondary" running={state === 'running'} onClick={async () => {
						try {
							const dir = await chooseSystemPath('luckylillia-directory')
							if (!dir) return
							confirm('关联 LuckyLillia', `将关联 ${dir}。工作台不会修改该目录中的文件。`, () => run(luckyAction('adopt'), { installDir: dir }, true))
						} catch (reason) {
							setResult({ output: '', error: reason instanceof Error ? reason.message : String(reason) })
							setState('failed')
						}
					}} />
				</div>
			</section>
		  )}


		  {!coreNeedsInstall && !nativeLauncherNapcat && <details className="rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3">
			<summary className="cursor-pointer text-xs font-semibold text-[var(--theme-text-secondary)]">更多操作</summary>
			<div className="mt-3 flex flex-wrap gap-2">
			{engine === 'napcat' ? <>
				<ActionButton label="启动" variant="secondary" running={state === 'running'} disabled={!napcatManagedActions || !liveStatus?.installed} onClick={() => confirm('启动 NapCat', '启动工作台受管的后台进程，用手机 QQ 扫码登录。', () => run('start', {}, true))} />
				<ActionButton label="停止" variant="secondary" running={state === 'running'} disabled={!napcatManagedActions || !liveStatus?.running} onClick={() => confirm('停止 NapCat', '停止工作台受管的 NapCat 进程组。', () => run('stop', {}, true))} />
				{liveStatus?.installed && !liveStatus?.managed ? <ActionButton label="取消关联" variant="danger" running={state === 'running'} onClick={() => confirm('取消关联 NapCat', '不会删除或修改外部目录。', () => run('napcat-forget', {}, true), 'danger')} /> : <ActionButton label="卸载" variant="danger" running={state === 'running'} disabled={!napcatManagedActions} onClick={() => confirm('卸载 NapCat', '会停止并删除工作台受管目录。', () => run('uninstall', {}, true), 'danger')} />}
				<ActionButton label="看日志" variant="secondary" running={state === 'running'} onClick={() => void run('log')} />
				<ActionButton label="清理日志" variant="secondary" running={state === 'running'} onClick={() => confirm('清理 NapCat 日志', '将清空核心日志与操作日志，不影响安装与配置。', () => run('napcat-log-clear', {}, true), 'danger')} />
			</> : engine === 'luckylillia' ? <>
				<ActionButton label="启动" variant="secondary" running={state === 'running'} disabled={!luckyManaged || !luckyInstalled} onClick={() => confirm('启动 LuckyLillia', '将启动官方 CLI 并等待登录。', () => run(luckyAction('start'), {}, true))} />
				<ActionButton label="停止" variant="secondary" running={state === 'running'} disabled={!luckyManaged || !liveStatus?.running} onClick={() => confirm('停止 LuckyLillia', '停止由工作台管理的 LuckyLillia 进程。', () => run(luckyAction('stop'), {}, true))} />
				{luckyInstalled && (luckyManaged ? <ActionButton label="卸载" variant="danger" running={state === 'running'} onClick={() => confirm('卸载 LuckyLillia', '会停止并删除工作台安装的 LuckyLillia。', () => run(luckyAction('uninstall'), {}, true), 'danger')} /> : <ActionButton label="取消关联" variant="danger" running={state === 'running'} onClick={() => confirm('取消关联 LuckyLillia', '不会删除外部目录或修改其中的文件。', () => run(luckyAction('forget'), {}, true), 'danger')} />)}
				<ActionButton label="看日志" variant="secondary" running={state === 'running'} onClick={() => void run(luckyAction('log'))} />
				<ActionButton label="清理日志" variant="secondary" running={state === 'running'} onClick={() => confirm('清理 LuckyLillia 日志', '将清空核心日志与操作日志，不影响安装与配置。', () => run(luckyAction('log-clear'), {}, true), 'danger')} />
			</> : <>
				<ActionButton label="启动" variant="secondary" running={state === 'running'} disabled={!liveStatus?.managed || !liveStatus?.installed} onClick={() => confirm('启动 SnowLuma', '启动工作台受管的 SnowLuma 原生进程。', () => run(snowLumaAction('start'), {}, true))} />
				<ActionButton label="停止" variant="secondary" running={state === 'running'} disabled={!liveStatus?.managed || !liveStatus?.running} onClick={() => confirm('停止 SnowLuma', '停止工作台受管的 SnowLuma 原生进程。', () => run(snowLumaAction('stop'), {}, true))} />
				<ActionButton label="卸载" variant="danger" running={state === 'running'} disabled={!liveStatus?.managed} onClick={() => confirm('卸载 SnowLuma', '将删除工作台安装的 SnowLuma 原生完整包。', () => run(snowLumaAction('uninstall'), {}, true), 'danger')} />
				<ActionButton label="看日志" variant="secondary" running={state === 'running'} onClick={() => void run(snowLumaAction('log'))} />
				<ActionButton label="清理操作日志" variant="secondary" running={state === 'running'} onClick={() => confirm('清理 SnowLuma 操作日志', '不会删除 SnowLuma 安装、QQ 登录或配置。', () => run(snowLumaAction('log-clear'), {}, true), 'danger')} />
			</>}
			</div>
		  </details>}

		  <details className="rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3">
			<summary className="cursor-pointer text-xs font-semibold text-[var(--theme-text-secondary)]">系统操作记录（{auditItems.length}）</summary>
			<div className="mt-2 grid gap-1.5">
				{auditItems.length === 0 ? (
					<p className="m-0 text-xs text-[var(--theme-text-muted)]">{auditLoading ? '正在读取…' : '暂无系统操作记录。'}</p>
				) : (
					auditItems.slice(0, 8).map(item => (
						<div key={item.id} className="grid grid-cols-[auto_1fr_auto] items-baseline gap-2 text-xs">
							<span className="font-semibold text-[var(--theme-text-strong)]">{item.action}</span>
							<span className="min-w-0 truncate text-[var(--theme-text-muted)]">{item.output || item.operation}</span>
							<span className="shrink-0 text-[var(--theme-text-faint)]">{new Date(item.createdAt).toLocaleString()}</span>
						</div>
					))
				)}
				<div>
					<ActionButton label="刷新" variant="secondary" running={state === 'running' || auditLoading} onClick={() => void refreshAudit()} />
				</div>
			</div>
		  </details>


		  {engine !== 'snowluma' && !nativeLauncherNapcat && liveStatus?.loginPending && (
			<section className="grid justify-items-center gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-4 text-center">
				<div>
					<h2 className="m-0 text-sm font-semibold text-[var(--theme-text-strong)]">{engine === 'napcat' ? '请用手机 QQ 扫码登录' : 'LuckyLillia 登录二维码'}</h2>
					<p className="m-0 mt-1 text-xs text-[var(--theme-text-muted)]">扫码后自动继续。</p>
				</div>
				{qrImageUrl ? (
					qrLoadFailed ? (
						<div className="grid gap-2 text-[var(--theme-warning-text)]">
							<p className="m-0 text-xs">二维码读取失败，请重试或查看日志。</p>
							<div className="flex justify-center gap-2">
								<ActionButton label="重试" variant="secondary" running={state === 'running'} onClick={() => void refreshStatus()} />
								<ActionButton label="查看日志" variant="secondary" running={state === 'running'} onClick={() => void openLiveLog()} />
							</div>
						</div>
					) : (
						<img className="size-56 rounded-lg bg-white p-2" src={qrImageUrl} onLoad={() => setQrLoadFailed(false)} onError={() => setQrLoadFailed(true)} alt={`${engine === 'napcat' ? 'NapCat' : 'LuckyLillia'} QQ 登录二维码`} />
					)
				) : (
					<p className="m-0 text-xs text-[var(--theme-text-muted)]">正在获取二维码…</p>
				)}
				{qrStale && (
					<div className="grid gap-2 rounded-md bg-[var(--theme-warning-soft)] px-3 py-2 text-xs text-[var(--theme-warning-text)]">
						<p className="m-0">二维码可能已过期，正在等待内核刷新。</p>
						<div className="flex justify-center gap-2">
							<ActionButton label="刷新状态" variant="secondary" running={state === 'running'} onClick={() => void refreshStatus()} />
							<ActionButton label="查看日志" variant="secondary" running={state === 'running'} onClick={() => void openLiveLog()} />
						</div>
					</div>
				)}
			</section>
		  )}




		  {liveStatus && !coreNeedsInstall && !nativeLauncherNapcat && (
            <section className="grid gap-2 rounded-panel border p-3 text-xs"
              style={{ borderColor: 'var(--theme-border-default)', background: 'var(--theme-surface-panel)' }}
            >
              <div className="flex items-center gap-2">
                <span
                  className={
                    'inline-flex size-2.5 rounded-full ' +
						(liveStatus.installed && liveStatus.running && (liveStatus.webUiReady || liveStatus.portReachable)
                      ? 'bg-[var(--theme-success)]'
                      : 'bg-[var(--theme-danger)]')
                  }
                />
                <strong className="text-sm font-semibold text-[var(--theme-text-strong)]">
					{liveStatus.installed && liveStatus.running && (liveStatus.webUiReady || liveStatus.portReachable)
                    ? '运行正常'
                    : '需要关注'}
                </strong>
                <span className="text-[var(--theme-text-muted)]">
					{liveStatus.installed ? (liveStatus.running ? (liveStatus.loginPending ? '等待扫码登录' : liveStatus.oneBotReady ? '已连接' : '正在连接') : '已停止') : '尚未安装'}
                </span>
              </div>
              {liveStatus.error && (
                <p className="m-0 rounded-md bg-[var(--theme-danger-soft)] px-2 py-1.5 font-semibold text-[var(--theme-danger-text)]">
                  {liveStatus.error}
                </p>
              )}
				{liveStatus.diagnosticHint && <p className="m-0 rounded-md bg-[var(--theme-warning-soft)] px-2 py-1.5 text-[var(--theme-warning-text)]">{liveStatus.diagnosticHint}</p>}
			  {engine === 'napcat' && napcatManagedActions && liveStatus.installed && !liveStatus.running && (
                <div className="flex gap-2">
                  <ActionButton label="一键重启" running={state === 'running'} onClick={() => void run('restart', {}, true)} />
                </div>
              )}
			  {engine === 'luckylillia' && luckyManaged && luckyInstalled && !liveStatus.running && (
                <div className="flex gap-2">
                  <ActionButton label="一键重启" running={state === 'running'} onClick={() => void run(luckyAction('restart'), {}, true)} />
                </div>
              )}
			  {engine === 'snowluma' && liveStatus.managed && liveStatus.installed && !liveStatus.running && (
				<div className="flex gap-2"><ActionButton label="一键重启" running={state === 'running'} onClick={() => void run(snowLumaAction('restart'), {}, true)} /></div>
			  )}
			</section>
		  )}


		  {!coreNeedsInstall && !nativeLauncherNapcat && webUrl && (
            <section className="grid gap-2 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3 text-xs">
              <strong className="text-sm font-semibold text-[var(--theme-text-strong)]">
                管理面板可用
              </strong>
              <p className="m-0 text-[var(--theme-text-muted)]">
                可在「管理面板」页签或新窗口打开。
              </p>
            </section>
          )}
        </div>
      )}

      {view === 'config' && (
        <div className="grid gap-3">
		  <section className="flex flex-wrap items-center justify-between gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3 text-xs">
			<div className="grid gap-1">
				<strong className="text-sm text-[var(--theme-text-strong)]">OneBot 连接健康</strong>
				<p className="m-0 text-[var(--theme-text-muted)]">{liveStatus?.oneBotReady ? '核心已就绪，可同步到机器人。' : '核心未就绪，请先完成 QQ 登录。'}</p>
			</div>
			{engine === 'snowluma' ? <p className="m-0 text-xs text-[var(--theme-text-muted)]">SnowLuma 的账号级 OneBot 配置由其 WebUI 管理；默认 WebSocket 为 3001。</p> : <ActionButton label="读取当前配置" variant="secondary" running={state === 'running'} onClick={() => void run(engine === 'napcat' ? 'onebot-config' : luckyAction('onebot-config'), engine === 'napcat' && napcatQQ ? { qq: napcatQQ } : {}, false, 'config-read')} />}
		  </section>
		  {localResult('config-read')}

		  {engine === 'luckylillia' && liveStatus?.managed && !liveStatus.authTokenReady && (
			<section className="grid gap-3 rounded-panel border border-[var(--theme-warning-text)] bg-[var(--theme-warning-soft)] p-3">
				<h2 className="m-0 text-sm font-semibold text-[var(--theme-text-strong)]">登录凭据</h2>
				<form className="grid gap-3" onSubmit={(event) => {
					event.preventDefault()
					const data = new FormData(event.currentTarget)
					confirm('保存并启动 LuckyLillia', '保存 Token 并启动。', () => run(luckyAction('auth-token-set-start'), { authToken: String(data.get('authToken') || '') }, true, 'config-auth'))
				}}>
					<p className="m-0 text-xs leading-5 text-[var(--theme-text-muted)]">从 <a className="text-[var(--theme-accent)] underline" href="https://auth.luckylillia.com" target="_blank" rel="noreferrer">auth.luckylillia.com</a> 获取 Token。</p>
					<Field label="Auth Token" name="authToken" type="password" />
					<div><button className="primary-button min-h-9" type="submit" disabled={state === 'running'}>保存并启动</button></div>
				</form>
				{localResult('config-auth')}
			</section>
		  )}

          <section className="grid gap-3">
			<h2 className="m-0 text-sm font-semibold text-[var(--theme-text-strong)]">连接服务</h2>
          {engine === 'napcat' ? <><form
            className="grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3"
            onSubmit={(event) => {
              event.preventDefault()
              const data = new FormData(event.currentTarget)
			  const params = {
				qq: napcatQQ,
                port: String(data.get('httpPort') || '3000'),
                enable: String(data.get('httpEnable') || 'true'),
                token: String(data.get('httpToken') || '')
              }
              confirm('保存 HTTP 服务', '更新 HTTP 端口与 Token，重启 NapCat 后生效。', () => run('onebot-http-set', params, true, 'config-http'))
            }}
          >
            <h3 className="col-span-full m-0 text-sm font-semibold text-[var(--theme-text-strong)]">
              HTTP 服务
            </h3>
			{(liveStatus?.accounts?.length || 0) > 1 && <Field label="QQ 账号" name="httpQQ"><select className={inputClass} value={napcatQQ} onChange={event => setNapcatQQ(event.target.value)}><option value="">请选择账号</option>{liveStatus?.accounts?.map(account => <option key={account.qq} value={account.qq}>{account.qq}</option>)}</select></Field>}
            <Field label="启用" name="httpEnable" defaultValue="true">
              <select className={inputClass} name="httpEnable" defaultValue="true">
                <option value="true">是</option>
                <option value="false">否</option>
              </select>
            </Field>
            <Field label="端口" name="httpPort" type="number" defaultValue="3000" />
            <Field label="Token" name="httpToken" />
			<ActionField><button className="primary-button min-h-9" type="submit" disabled={!napcatManagedActions || ((liveStatus?.accounts?.length || 0) > 1 && !napcatQQ)}>保存 HTTP</button></ActionField>
			{localResult('config-http')}
          </form>

          <form
            className="grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3"
            onSubmit={(event) => {
              event.preventDefault()
              const data = new FormData(event.currentTarget)
			  const params = {
				qq: napcatQQ,
                port: String(data.get('wsPort') || '3001'),
                enable: String(data.get('wsEnable') || 'true'),
                token: String(data.get('wsToken') || '')
              }
              confirm('保存 WebSocket 服务', '更新 WS 端口与 Token，重启 NapCat 后生效。', () => run('onebot-ws-set', params, true, 'config-ws'))
            }}
          >
            <h3 className="col-span-full m-0 text-sm font-semibold text-[var(--theme-text-strong)]">
              WebSocket 服务
            </h3>
			{(liveStatus?.accounts?.length || 0) > 1 && <Field label="QQ 账号" name="wsQQ"><select className={inputClass} value={napcatQQ} onChange={event => setNapcatQQ(event.target.value)}><option value="">请选择账号</option>{liveStatus?.accounts?.map(account => <option key={account.qq} value={account.qq}>{account.qq}</option>)}</select></Field>}
            <Field label="启用" name="wsEnable" defaultValue="true">
              <select className={inputClass} name="wsEnable" defaultValue="true">
                <option value="true">是</option>
                <option value="false">否</option>
              </select>
            </Field>
            <Field label="端口" name="wsPort" type="number" defaultValue="3001" />
            <Field label="Token" name="wsToken" />
			<ActionField><button className="primary-button min-h-9" type="submit" disabled={!napcatManagedActions || ((liveStatus?.accounts?.length || 0) > 1 && !napcatQQ)}>保存 WebSocket</button></ActionField>
			{localResult('config-ws')}
		  </form></> : engine === 'luckylillia' ? <form
			className="grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3"
			onSubmit={(event) => {
				event.preventDefault()
				const data = new FormData(event.currentTarget)
				const params = { port: String(data.get('port') || '7199'), enable: String(data.get('enable') || 'true'), token: String(data.get('token') || '') }
				confirm('保存 LuckyLillia OneBot 服务', '更新 WebSocket 端口与 Token，重启 LuckyLillia 后生效。', () => run(luckyAction('onebot-set'), params, true, 'config-lucky'))
			}}>
			<h3 className="col-span-full m-0 text-sm font-semibold text-[var(--theme-text-strong)]">LuckyLillia OneBot WebSocket</h3>
			<Field label="启用" name="enable"><select className={inputClass} name="enable" defaultValue="true"><option value="true">是</option><option value="false">否</option></select></Field>
			<Field label="端口" name="port" type="number" defaultValue="7199" />
			<Field label="Token" name="token" />
			<ActionField><button className="primary-button min-h-9" type="submit" disabled={!liveStatus?.managed}>保存连接</button></ActionField>
			{localResult('config-lucky')}
		  </form> : <section className="grid gap-2 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3 text-xs"><strong className="text-sm text-[var(--theme-text-strong)]">SnowLuma OneBot</strong><p className="m-0 leading-5 text-[var(--theme-text-muted)]">请在 SnowLuma WebUI 中完成登录并管理 `config/onebot.json`。默认 HTTP 为 3000，WebSocket 为 3001；只有在已生成并启用 WebSocket 配置后，工作台才会读取 Token 并允许同步。</p><div><ActionButton label="打开 SnowLuma WebUI" variant="secondary" running={state === 'running'} disabled={!webUrl} onClick={() => setView('webui')} /></div></section>}
		  </section>

		  <section className="grid gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3">
			<div className="flex items-center justify-between gap-3">
				<h2 className="m-0 text-sm font-semibold text-[var(--theme-text-strong)]">集成到 AlemonJS 机器人</h2>
				<ActionButton label="刷新" variant="secondary" running={state === 'running'} onClick={() => void fetchRobotProjects(true).then(setProjects).catch(() => setProjects([]))} />
			</div>
			<div className="grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3">
				<label className="grid gap-1 text-xs font-semibold text-[var(--theme-text-secondary)]">目标机器人
					<select className={inputClass} value={robotRoot} onChange={event => setRobotRoot(event.target.value)}><option value="">请选择机器人</option>{projects.map(project => <option key={project.root} value={project.root}>{project.name}</option>)}</select>
				</label>
				<ActionField><ActionButton label="同步连接" running={state === 'running'} onClick={() => confirm('同步 OneBot 配置', '自动读取 Token 并写入目标机器人。', async () => {
					setResultOrigin('config-sync')
					if (!robotRoot) { setResult({ output: '', error: '请选择目标机器人。' }); setState('failed'); return }
					setState('running'); setResult(undefined); setOperationSteps([])
					try {
						const token = await readOneBotToken()
						const url = selectedOneBotURL || (engine === 'napcat' || engine === 'snowluma' ? 'ws://127.0.0.1:3001' : 'ws://127.0.0.1:7199')
						await syncRobotOneBot(robotRoot, url, token)
						setResult({ output: '✓ OneBot 配置已同步到目标机器人（Token 已自动读取）。请按需重启机器人使连接生效。' }); setState('done')
					} catch (reason) { setResult({ output: '', error: reason instanceof Error ? reason.message : String(reason) }); setState('failed') }
				})} /></ActionField>
			</div>
			{!selectedOneBotReady && <p className="m-0 text-xs text-[var(--theme-warning-text)]">核心未就绪，可先同步，就绪后自动连接。</p>}
			{projects.length === 0 && <p className="m-0 text-xs text-[var(--theme-text-muted)]">未发现机器人项目。</p>}
			{localResult('config-sync')}
		  </section>
        </div>
      )}

      {view === 'background' && (
        <div className="grid gap-3">
          <p className="m-0 text-xs leading-5 text-[var(--theme-text-muted)]">后台进程独立于工作台运行；ALemonX 更新或重启不会中断它们。主机重启后需手动再次启动。</p>
          <section className="grid gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3">
            <div className="flex items-center justify-between gap-3">
              <strong className="text-sm font-semibold text-[var(--theme-text-strong)]">{engineLabel(engine)}</strong>
              <span className={'text-xs ' + (liveStatus?.running ? 'text-[var(--theme-success-text)]' : 'text-[var(--theme-text-muted)]')}>
                {liveStatus?.installed ? (liveStatus.running ? `后台运行中${liveStatus.pid ? `（PID ${liveStatus.pid}）` : ''}` : '已停止') : '未安装'}
              </span>
            </div>
            {liveStatus?.error && <p className="m-0 rounded-md bg-[var(--theme-danger-soft)] px-2 py-1.5 text-xs text-[var(--theme-danger-text)]">{liveStatus.error}</p>}
            <div className="flex flex-wrap gap-2">
              <ActionButton label="后台启动" running={state === 'running'} disabled={!liveStatus?.installed || liveStatus?.running === true} onClick={() => confirm(`后台启动 ${engineLabel(engine)}`, '进程将脱离工作台独立运行；ALemonX 更新或重启不会中断。', () => run(engineAction(engine, 'start'), {}, true, 'manage', engine))} />
              <ActionButton label="停止" variant="secondary" running={state === 'running'} disabled={liveStatus?.running !== true} onClick={() => confirm(`停止 ${engineLabel(engine)}`, '停止后台进程。', () => run(engineAction(engine, 'stop'), {}, true, 'manage', engine))} />
              <ActionButton label="看日志" variant="secondary" running={state === 'running'} onClick={() => void run(engineAction(engine, 'log'), {}, false, 'manage', engine)} />
            </div>
          </section>
        </div>
      )}

      {view === 'webui' && (
        <div className="relative min-h-0 overflow-hidden rounded-panel border border-[var(--theme-border-default)]">
          {webUrl ? (
            <iframe
              className="h-[min(720px,calc(100vh-160px))] min-h-[480px] w-full border-0"
              src={webUrl}
              title={`${engineLabel(engine)} 管理面板`}
            />
          ) : (
            <div className="grid gap-2 p-6 text-center">
              <strong className="text-sm text-[var(--theme-text-strong)]">
                管理面板未连接
              </strong>
              <p className="m-0 text-xs leading-5 text-[var(--theme-text-muted)]">
                需先「安装」并「启动」，且其管理面板就绪后，这里才能内嵌显示。
              </p>
            </div>
          )}
        </div>
      )}


      {(view === 'manage' || view === 'background') && resultOrigin === 'manage' && <ResultPanel state={state} result={result ?? (state === 'running' ? { output: actionTitle(activeAction) } : undefined)} steps={operationSteps} liveDetail={operationDetail} onViewLog={() => void openLiveLog()} />}

      {pendingConfirm && (
        <ConfirmModal
          title={pendingConfirm.title}
          description={pendingConfirm.description}
          tone={pendingConfirm.tone}
          onConfirm={() => {
            void pendingConfirm.action()
            setPendingConfirm(null)
          }}
          onCancel={() => setPendingConfirm(null)}
        />
      )}
      {sudoRequest && (
        <SudoModal
          title={sudoRequest.title}
          description={sudoRequest.description}
          busy={state === 'running'}
          onConfirm={(password) => void runPrivileged(sudoRequest.action, password, sudoRequest.intentId)}
          onCancel={() => setSudoRequest(null)}
        />
      )}
      {logText !== null && <LogModal text={logText} onClose={() => setLogText(null)} autoRefresh={logAutoRefresh} onToggleAutoRefresh={() => setLogAutoRefresh(current => !current)} onRefresh={() => void openLiveLog()} title={`${engineLabel(logEngine)} 日志`} />}
        </div>
      </main>
    </div>
  )
}

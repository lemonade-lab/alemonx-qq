import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { fetchHostRobotContext, fetchLocalServices, fetchOperationLog, fetchPluginLog, fetchRobotProjects, fetchStatus, napcatQRCodeURL, runActionAndPoll, syncRobotOneBot, type ActionResult, type LocalService, type RobotProject, type StatusPayload, type Task, type TaskStep } from './api'
import { splitStatusLines, type StatusLine } from './status'
import { loadSession, saveSession, type QQEngine, type QQView } from './session'

type View = QQView
type Engine = QQEngine

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
        'grid grid-cols-[1.125rem_1fr] items-baseline gap-1.5 py-1 text-[11px] leading-5 ' +
        statusColor[line.kind]
      }
    >
      <span className="text-center font-bold" aria-hidden>
        {statusSymbol[line.kind]}
      </span>
      <span
        className={
          line.kind === 'plain' ? 'whitespace-pre-wrap font-mono' : undefined
        }
      >
        {line.text || ' '}
      </span>
    </div>
  )
}

function ResultPanel({
  state,
  result,
  steps = [],
  liveDetail = '',
  onViewLog
}: {
  state: 'idle' | 'running' | 'done' | 'failed'
  result?: ActionResult
  steps?: TaskStep[]
  liveDetail?: string
  onViewLog?: () => void
}) {
  const lines = useMemo(() => splitStatusLines(result?.output ?? ''), [result])
	const current = steps.at(-1)
  if (state === 'idle') return null
  return (
    <section className="grid gap-2 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3 text-xs">
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
          <pre className="m-0 max-h-48 overflow-auto whitespace-pre-wrap break-words font-mono text-[11px] font-normal leading-5">
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
					<pre className="m-0 max-h-56 overflow-auto whitespace-pre-wrap break-words font-mono text-[11px] leading-5 text-[var(--theme-text-secondary)]">{liveDetail}</pre>
				</div>
			)}
			{steps.length > 1 && (
				<details className="text-[11px] text-[var(--theme-text-muted)]">
					<summary className="cursor-pointer">已完成步骤（{steps.length}）</summary>
					<div className="mt-1 grid gap-1 font-mono">{steps.map((step, index) => <div key={`${step.at}-${index}`}>{step.progress}% {step.message}</div>)}</div>
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
  children,
  hint
}: {
  label: string
  name: string
  type?: string
  defaultValue?: string
  children?: ReactNode
  hint?: string
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
      {hint && (
        <span className="font-normal leading-4 text-[var(--theme-text-muted)]">
          {hint}
        </span>
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
  onCancel
}: {
  title: string
  description: string
  onConfirm: () => void
  onCancel: () => void
}) {
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-[var(--theme-surface-overlay)]">
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
          <button className="danger-button" onClick={onConfirm}>
            确认执行
          </button>
        </div>
      </div>
    </div>
  )
}

function LogModal({ text, onClose }: { text: string; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-[var(--theme-surface-overlay)] p-4">
      <div className="grid max-h-[min(680px,calc(100vh-32px))] w-[min(760px,100%)] gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-4 shadow-[var(--theme-shadow-pop)]">
        <div className="flex items-center justify-between gap-3">
          <strong className="text-sm text-[var(--theme-text-strong)]">实时安装日志</strong>
          <button className="secondary-button" onClick={onClose}>关闭</button>
        </div>
        <pre className="m-0 max-h-[560px] overflow-auto whitespace-pre-wrap break-words rounded-md bg-[var(--theme-surface-input)] p-3 font-mono text-[11px] leading-5 text-[var(--theme-text-secondary)]">{text}</pre>
      </div>
    </div>
  )
}

function ActionButton({
  label,
  variant = 'primary',
  running,
	disabled = false,
  onClick
}: {
  label: string
  variant?: 'primary' | 'secondary' | 'danger'
  running: boolean
	disabled?: boolean
  onClick: () => void
}) {
  const className =
    variant === 'primary'
      ? 'primary-button'
      : variant === 'danger'
        ? 'danger-button'
        : 'secondary-button'
  return (
    <button className={className} disabled={running || disabled} onClick={onClick}>
      {label}
    </button>
  )
}

function actionTitle(action: string | null) {
	if (!action) return ''
	if (action.includes('reinstall')) return '正在重装并保留现有配置'
	if (action === 'luckylillia-install') return '正在下载、验证并准备 LuckyLillia'
	if (action === 'luckylillia-auth-token-set' || action === 'luckylillia-auth-token-set-start') return '正在保存 Auth Token 并启动 LuckyLillia'
	if (action === 'install') return '正在下载、安装并启动 NapCat'
	if (action === 'start' || action.endsWith('-start')) return '正在启动 QQ 服务并等待二维码'
	if (action === 'stop' || action.endsWith('-stop')) return '正在停止核心服务'
	if (action.includes('update')) return '正在检查或更新核心版本'
	if (action.includes('onebot')) return '正在保存 OneBot 连接配置'
	return '正在执行操作'
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
    action: () => Promise<void>
	} | null>(null)
	const [logText, setLogText] = useState<string | null>(null)
	const [operationDetail, setOperationDetail] = useState('')
	const [statusByEngine, setStatusByEngine] = useState<Partial<Record<Engine, StatusPayload>>>({})
	const [statusLoading, setStatusLoading] = useState<Partial<Record<Engine, boolean>>>({})
	const liveStatus = statusByEngine[engine] ?? null
	const [projects, setProjects] = useState<RobotProject[]>([])
	const [services, setServices] = useState<LocalService[]>([])
	const [robotRoot, setRobotRoot] = useState(initialSession.robotRoot)
	const [syncToken, setSyncToken] = useState('')
	const webServiceID = engine === 'napcat' ? 'napcat-webui' : 'luckylillia-webui'
	const webService = services.find(service => service.id === webServiceID)
	const webUrl = webService?.reachable && webService.embed ? webService.proxyUrl : ''
	const qrImageUrl = liveStatus?.qrCodeAvailable
		? napcatQRCodeURL(liveStatus.qrCodeUpdatedAt)
		: ''
	const luckyAction = (action: string) => `luckylillia-${action}`
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
	// Linux's first-run path has exactly one user outcome: scan the QQ code.
	// Keep configuration, directories and maintenance controls out of sight
	// until the core either becomes ready or genuinely needs recovery.
	const napcatLoginJourney = engine === 'napcat' && !nativeLauncherNapcat && liveStatus?.installed === true && liveStatus.running && !liveStatus.oneBotReady

	const run = async (action: string, params: Record<string, string> = {}, confirm = false) => {
		// State updates are asynchronous, so `state === running` alone cannot
		// stop two rapid clicks (or two confirmation clicks) from starting two
		// installations. Keep a synchronous guard in the browser as well.
		if (actionInFlight.current) return
		actionInFlight.current = true
	setActiveAction(action)
    setState('running')
		setResult(undefined)
		setOperationSteps([])
		setOperationDetail('正在创建本次操作记录…')
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

	const openLiveLog = async () => {
		setLogText('正在读取日志…')
		try {
			setLogText(await fetchPluginLog(engine))
		} catch (reason) {
			setLogText(reason instanceof Error ? reason.message : String(reason))
		}

	useEffect(() => {
	if (state !== 'running') return
		let stopped = false
		const refresh = async () => {
			try {
				const detail = await fetchOperationLog(engine)
				if (!stopped && detail) setOperationDetail(detail)
			} catch { /* task progress remains the primary display */ }
		}
		void refresh()
		const timer = window.setInterval(() => void refresh(), 1000)
		return () => { stopped = true; window.clearInterval(timer) }
	}, [engine, state])
	}

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

  const confirm = (title: string, description: string, action: () => Promise<void>) => {
    setPendingConfirm({ title, description, action })
  }

	const guide = useMemo(() => {
		if (!liveStatus) return {
			title: engine === 'napcat' ? 'NapCat' : 'LuckyLillia',
			description: statusLoading[engine] ? '正在读取当前状态…' : '暂时无法读取状态，点击刷新重试。',
			label: statusLoading[engine] ? '读取中' : '刷新状态',
			action: () => { if (!statusLoading[engine]) void refreshStatus() }
		}
		if (engine === 'napcat' && liveStatus.platform === 'darwin-external' && !liveStatus.installed) return liveStatus.installerReady ? {
			title: '安装 NapCat',
			description: liveStatus.installerPath ? `${liveStatus.installerPath}` : '安装器已下载。',
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
		if (engine === 'napcat' && !liveStatus.installed) return { title: '安装 NapCat', description: '点击后自动安装并启动，随后显示登录二维码。', label: '安装 NapCat', action: () => void requestNapcatInstall() }
		if (engine === 'napcat' && liveStatus.installed && liveStatus.running && !liveStatus.oneBotReady) {
			if (liveStatus.loginPending) return null
			return { title: '正在准备登录二维码', description: 'NapCat 已启动，请稍候。二维码出现后用手机 QQ 扫描即可。', label: '等待二维码', action: () => undefined }
		}
		if (nativeLauncherNapcat) return {
			title: 'NapCat 启动器',
			description: liveStatus.launcherPath || '使用官方启动器管理 NapCat。',
			label: '打开 NapCat 启动器',
			action: () => confirm('打开 NapCat 启动器', '启动器负责安装、启动和管理 NapCat。', () => run('napcat-macos-launcher-open', {}, true)),
		}
		if (engine === 'napcat' && liveStatus.installed && !liveStatus.managed) return { title: 'NapCat 已关联', description: webUrl ? '可以继续登录 QQ。' : '正在检查登录状态。', label: webUrl ? '打开登录页' : '刷新', action: () => webUrl ? setView('webui') : void refreshStatus() }
		if (engine === 'luckylillia' && liveStatus.supported === false) return { title: '此系统暂不支持', description: '请改用 NapCat，或手动安装后关联。', label: '关联目录', action: () => document.getElementById('luckylillia-association')?.scrollIntoView({ behavior: 'smooth', block: 'center' }) }
		if (!liveStatus.installed) return { title: '安装 QQ 核心', description: engine === 'napcat' ? '安装完成后会自动启动并进入登录。' : '安装后需先填写官方 Auth Token，再启动登录服务。', label: engine === 'napcat' ? '安装 NapCat' : '安装 LuckyLillia', action: () => engine === 'napcat' ? void requestNapcatInstall() : confirm('安装 QQ 核心', '将下载并验证官方组件。填写 Auth Token 后才会启动。', () => run(luckyAction('install'), {}, true)) }
		if (engine === 'luckylillia' && !liveStatus.managed) return { title: 'LuckyLillia 已关联', description: webUrl ? '可以继续登录 QQ。' : '正在检查登录状态。', label: webUrl ? '打开登录页' : '刷新', action: () => webUrl ? setView('webui') : void refreshStatus() }
		if (engine === 'luckylillia' && !liveStatus.authTokenReady) return { title: '需要 Auth Token', description: '从 auth.luckylillia.com 获取 Token 后，在网络配置中保存。', label: '填写 Token', action: () => setView('config') }
		if (!liveStatus.running) return { title: '启动 QQ', description: '启动后即可扫码登录。', label: engine === 'napcat' ? '启动 NapCat' : '启动 LuckyLillia', action: () => confirm('启动 QQ', '启动后请使用手机 QQ 扫码。', () => run(engine === 'napcat' ? 'start' : luckyAction('start'), {}, true)) }
		if (liveStatus.loginPending) return { title: '请用手机 QQ 扫码', description: '扫码后会自动继续。', label: webUrl ? '打开登录页' : '等待登录', action: () => webUrl ? setView('webui') : undefined }
		if (!liveStatus.oneBotReady) return { title: '正在连接', description: '请稍候。', label: '刷新', action: () => void refreshStatus() }
		return { title: 'QQ 已就绪', description: '现在可同步到机器人。', label: '同步到机器人', action: () => setView('config') }
	}, [engine, liveStatus, refreshStatus, statusLoading, webUrl])

  return (
    <div className="mx-auto grid max-w-[860px] gap-4 p-4">
      <header className="flex items-baseline justify-between gap-3 border-b border-[var(--theme-border-default)] pb-3">
        <div>
          <h1 className="m-0 text-base font-semibold tracking-tight text-[var(--theme-text-strong)]">
            QQ 内核管理
          </h1>
        </div>
		<div className="flex gap-1">
			{(['napcat', 'luckylillia'] as Engine[]).map(item => (
				<button key={item} className={engine === item ? 'primary-button' : 'secondary-button'} onClick={() => { setEngine(item); setView('manage'); setResult(undefined); setOperationSteps([]); setState('idle'); void refreshStatus(item) }}>
					{item === 'napcat' ? 'NapCat' : 'LuckyLillia'}
				</button>
			))}
		</div>
      </header>

	  {!coreNeedsInstall && !nativeLauncherNapcat && !napcatLoginJourney && <nav className="flex gap-1 border-b border-[var(--theme-border-default)] pb-2">
        {(['manage', 'config', 'webui'] as View[]).map((tab) => (
          <button
            key={tab}
            className={
              'min-h-8 rounded-control border px-3 text-xs font-semibold transition ' +
              (view === tab
                ? 'border-[var(--theme-border-strong)] bg-[var(--theme-surface-panel)] text-[var(--theme-text-strong)]'
                : 'border-transparent text-[var(--theme-text-muted)] hover:bg-[var(--theme-surface-hover)] hover:text-[var(--theme-text-strong)]')
            }
            onClick={() => setView(tab)}
          >
            {tab === 'manage' ? '管理' : tab === 'config' ? '网络配置' : '管理面板'}
          </button>
        ))}
	  </nav>}

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

		  {engine === 'napcat' && !nativeLauncherNapcat && liveStatus?.loginPending && (
			<section className="grid justify-items-center gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-4 text-center">
				<div>
					<h2 className="m-0 text-sm font-semibold text-[var(--theme-text-strong)]">请用手机 QQ 扫码登录</h2>
					<p className="m-0 mt-1 text-xs text-[var(--theme-text-muted)]">扫码完成后会自动继续。</p>
				</div>
				{qrImageUrl ? (
					<img className="size-56 rounded-lg bg-white p-2" src={qrImageUrl} alt="NapCat QQ 登录二维码" />
				) : (
					<p className="m-0 text-xs text-[var(--theme-text-muted)]">正在获取二维码…</p>
				)}
			</section>
		  )}

		  {liveStatus && !coreNeedsInstall && !nativeLauncherNapcat && !napcatLoginJourney && (
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
			</section>
		  )}


		  {!coreNeedsInstall && !nativeLauncherNapcat && !napcatLoginJourney && <details className="rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3">
			<summary className="cursor-pointer text-xs font-semibold text-[var(--theme-text-secondary)]">更多操作</summary>
			<div className="mt-3 flex flex-wrap gap-2">
			{engine === 'napcat' ? <>
				<ActionButton label="启动" variant="secondary" running={state === 'running'} disabled={!napcatManagedActions || !liveStatus?.installed} onClick={() => confirm('启动 NapCat', '启动工作台受管的后台进程，用手机 QQ 扫码登录。', () => run('start', {}, true))} />
				<ActionButton label="停止" variant="secondary" running={state === 'running'} disabled={!napcatManagedActions || !liveStatus?.running} onClick={() => confirm('停止 NapCat', '停止工作台受管的 NapCat 进程组。', () => run('stop', {}, true))} />
				{liveStatus?.installed && !liveStatus?.managed ? <ActionButton label="取消关联" variant="danger" running={state === 'running'} onClick={() => confirm('取消关联 NapCat', '不会删除或修改外部目录。', () => run('napcat-forget', {}, true))} /> : <ActionButton label="卸载" variant="danger" running={state === 'running'} disabled={!napcatManagedActions} onClick={() => confirm('卸载 NapCat', '会停止并删除工作台受管目录。', () => run('uninstall', {}, true))} />}
				<ActionButton label="看日志" variant="secondary" running={state === 'running'} onClick={() => void run('log')} />
			</> : <>
				<ActionButton label="启动" variant="secondary" running={state === 'running'} disabled={!luckyManaged || !luckyInstalled} onClick={() => confirm('启动 LuckyLillia', '将启动官方 CLI 并等待登录。', () => run(luckyAction('start'), {}, true))} />
				<ActionButton label="停止" variant="secondary" running={state === 'running'} disabled={!luckyManaged || !liveStatus?.running} onClick={() => confirm('停止 LuckyLillia', '停止由工作台管理的 LuckyLillia 进程。', () => run(luckyAction('stop'), {}, true))} />
				{luckyInstalled && (luckyManaged ? <ActionButton label="卸载" variant="danger" running={state === 'running'} onClick={() => confirm('卸载 LuckyLillia', '会停止并删除工作台安装的 LuckyLillia。', () => run(luckyAction('uninstall'), {}, true))} /> : <ActionButton label="取消关联" variant="danger" running={state === 'running'} onClick={() => confirm('取消关联 LuckyLillia', '不会删除外部目录或修改其中的文件。', () => run(luckyAction('forget'), {}, true))} />)}
				<ActionButton label="看日志" variant="secondary" running={state === 'running'} onClick={() => void run(luckyAction('log'))} />
			</>}
			</div>
		  </details>}


			  <ResultPanel state={state} result={result ?? (state === 'running' ? { output: actionTitle(activeAction) } : undefined)} steps={operationSteps} liveDetail={operationDetail} onViewLog={() => void openLiveLog()} />

		  {!coreNeedsInstall && !nativeLauncherNapcat && !napcatLoginJourney && webUrl && (
            <section className="grid gap-2 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3 text-xs">
              <strong className="text-sm font-semibold text-[var(--theme-text-strong)]">
                管理面板可用
              </strong>
              <p className="m-0 text-[var(--theme-text-muted)]">
                切到「管理面板」页签，或{' '}
                <a className="text-[var(--theme-accent)] underline" href={webUrl} target="_blank" rel="noreferrer">
                  在浏览器打开
                </a>{' '}
                用手机 QQ 扫码登录。
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
				<p className="m-0 text-[var(--theme-text-muted)]">{liveStatus?.oneBotReady ? `核心已在 ${liveStatus.oneBotUrl || '本机 WebSocket 地址'} 就绪。选择机器人并输入 Token 后即可同步。` : '核心 OneBot 尚未就绪。请先完成 QQ 登录，再同步到机器人。'}</p>
			</div>
			<ActionButton label="读取当前配置" variant="secondary" running={state === 'running'} onClick={() => void run(engine === 'napcat' ? 'onebot-config' : luckyAction('onebot-config'), engine === 'napcat' && napcatQQ ? { qq: napcatQQ } : {})} />
		  </section>
		  <ResultPanel state={state} result={result} steps={operationSteps} onViewLog={() => void openLiveLog()} />

		  {engine === 'luckylillia' && liveStatus?.managed && !liveStatus.authTokenReady && (
			<form className="grid gap-3 rounded-panel border border-[var(--theme-warning-text)] bg-[var(--theme-warning-soft)] p-3" onSubmit={(event) => {
				event.preventDefault()
				const data = new FormData(event.currentTarget)
				confirm('保存并启动 LuckyLillia', 'Token 只会保存到本机私有文件，不会在状态或日志中显示。保存后将立即启动并等待管理页面。', () => run(luckyAction('auth-token-set-start'), { authToken: String(data.get('authToken') || '') }, true))
			}}>
				<div className="grid gap-1">
					<strong className="text-sm text-[var(--theme-text-strong)]">LuckyLillia Auth Token</strong>
					<p className="m-0 text-xs leading-5 text-[var(--theme-text-muted)]">先到 <a className="text-[var(--theme-accent)] underline" href="https://auth.luckylillia.com" target="_blank" rel="noreferrer">auth.luckylillia.com</a> 获取 Token。保存后才可启动 WebUI。</p>
				</div>
				<Field label="Auth Token" name="authToken" type="password" hint="仅保存到本机 auth_token.txt；不会回显。" />
				<div><button className="primary-button" type="submit" disabled={state === 'running'}>保存并启动</button></div>
			</form>
		  )}

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
              confirm('保存 HTTP 服务', '更新 HTTP 端口与 Token，重启 NapCat 后生效。', () => run('onebot-http-set', params, true))
            }}
          >
            <h2 className="col-span-full m-0 text-sm font-semibold text-[var(--theme-text-strong)]">
              HTTP 服务
            </h2>
			{(liveStatus?.accounts?.length || 0) > 1 && <Field label="QQ 账号" name="httpQQ"><select className={inputClass} value={napcatQQ} onChange={event => setNapcatQQ(event.target.value)}><option value="">请选择账号</option>{liveStatus?.accounts?.map(account => <option key={account.qq} value={account.qq}>{account.qq}</option>)}</select></Field>}
            <Field label="启用" name="httpEnable" defaultValue="true">
              <select className={inputClass} name="httpEnable" defaultValue="true">
                <option value="true">是</option>
                <option value="false">否</option>
              </select>
            </Field>
            <Field label="端口" name="httpPort" type="number" defaultValue="3000" hint="默认 3000。" />
            <Field label="Token" name="httpToken" hint="留空不改动；填 **** 也视为不改动。" />
			<ActionField><button className="primary-button min-h-9" type="submit" disabled={!napcatManagedActions || ((liveStatus?.accounts?.length || 0) > 1 && !napcatQQ)}>保存 HTTP</button></ActionField>
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
              confirm('保存 WebSocket 服务', '更新 WS 端口与 Token，重启 NapCat 后生效。', () => run('onebot-ws-set', params, true))
            }}
          >
            <h2 className="col-span-full m-0 text-sm font-semibold text-[var(--theme-text-strong)]">
              WebSocket 服务
            </h2>
			{(liveStatus?.accounts?.length || 0) > 1 && <Field label="QQ 账号" name="wsQQ"><select className={inputClass} value={napcatQQ} onChange={event => setNapcatQQ(event.target.value)}><option value="">请选择账号</option>{liveStatus?.accounts?.map(account => <option key={account.qq} value={account.qq}>{account.qq}</option>)}</select></Field>}
            <Field label="启用" name="wsEnable" defaultValue="true">
              <select className={inputClass} name="wsEnable" defaultValue="true">
                <option value="true">是</option>
                <option value="false">否</option>
              </select>
            </Field>
            <Field label="端口" name="wsPort" type="number" defaultValue="3001" hint="默认 3001。" />
            <Field label="Token" name="wsToken" hint="留空不改动；填 **** 也视为不改动。" />
			<ActionField><button className="primary-button min-h-9" type="submit" disabled={!napcatManagedActions || ((liveStatus?.accounts?.length || 0) > 1 && !napcatQQ)}>保存 WebSocket</button></ActionField>
		  </form></> : <form
			className="grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3"
			onSubmit={(event) => {
				event.preventDefault()
				const data = new FormData(event.currentTarget)
				const params = { port: String(data.get('port') || '7199'), enable: String(data.get('enable') || 'true'), token: String(data.get('token') || '') }
				confirm('保存 LuckyLillia OneBot 服务', '更新 WebSocket 端口与 Token，重启 LuckyLillia 后生效。', () => run(luckyAction('onebot-set'), params, true))
			}}>
			<h2 className="col-span-full m-0 text-sm font-semibold text-[var(--theme-text-strong)]">LuckyLillia OneBot WebSocket</h2>
			<Field label="启用" name="enable"><select className={inputClass} name="enable" defaultValue="true"><option value="true">是</option><option value="false">否</option></select></Field>
			<Field label="端口" name="port" type="number" defaultValue="7199" hint="默认 7199。" />
			<Field label="Token" name="token" hint="留空不改动；Token 不会在状态中显示。" />
			<ActionField><button className="primary-button min-h-9" type="submit" disabled={!liveStatus?.managed}>保存连接</button></ActionField>
		  </form>}

		  <section className="grid gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3">
			<h2 className="m-0 text-sm font-semibold text-[var(--theme-text-strong)]">同步到 AlemonJS 机器人</h2>
			<p className="m-0 text-xs leading-5 text-[var(--theme-text-muted)]">选择已受工作台管理的机器人，写入 OneBot URL 与 Token；不会自动重启机器人。缺少 @alemonjs/onebot 时只会提示安装。</p>
			<div className="grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3">
				<label className="grid gap-1 text-xs font-semibold text-[var(--theme-text-secondary)]">目标机器人
					<select className={inputClass} value={robotRoot} onChange={event => setRobotRoot(event.target.value)}><option value="">请选择机器人</option>{projects.map(project => <option key={project.root} value={project.root}>{project.name}</option>)}</select>
				</label>
				<label className="grid gap-1 text-xs font-semibold text-[var(--theme-text-secondary)]">OneBot Token
					<input className={inputClass} type="password" value={syncToken} onChange={event => setSyncToken(event.target.value)} placeholder="输入当前内核的 Token" />
				</label>
				<ActionField><ActionButton label="同步连接" running={state === 'running'} disabled={!syncToken.trim() || !selectedOneBotReady} onClick={() => confirm('同步 OneBot 配置', '将写入目标机器人的 OneBot URL、Token 并切换登录连接；不会重启机器人。', async () => {
					if (!robotRoot) { setResult({ output: '', error: '请选择目标机器人。' }); setState('failed'); return }
					if (!syncToken.trim()) { setResult({ output: '', error: '必须显式输入非空 OneBot Token。' }); setState('failed'); return }
					const url = selectedOneBotURL || (engine === 'napcat' ? 'ws://127.0.0.1:3001' : 'ws://127.0.0.1:7199')
					try {
						await syncRobotOneBot(robotRoot, url, syncToken)
						setResult({ output: '✓ OneBot 配置已同步到目标机器人。请按需重启机器人使连接生效。' }); setState('done')
					} catch (reason) { setResult({ output: '', error: reason instanceof Error ? reason.message : String(reason) }); setState('failed') }
				})} /></ActionField>
			</div>
		  </section>
        </div>
      )}

      {view === 'webui' && (
        <div className="relative min-h-0 overflow-hidden rounded-panel border border-[var(--theme-border-default)]">
          {webUrl ? (
            <iframe
              className="h-[640px] w-full border-0"
              src={webUrl}
              title="NapCat 管理面板"
            />
          ) : (
            <div className="grid gap-2 p-6 text-center">
              <strong className="text-sm text-[var(--theme-text-strong)]">
                管理面板未连接
              </strong>
              <p className="m-0 text-xs leading-5 text-[var(--theme-text-muted)]">
                NapCat 需先「安装」并「启动」，且其管理面板（6099 端口）就绪后，这里才能内嵌显示。
              </p>
            </div>
          )}
        </div>
      )}

      {pendingConfirm && (
        <ConfirmModal
          title={pendingConfirm.title}
          description={pendingConfirm.description}
          onConfirm={() => {
            void pendingConfirm.action()
            setPendingConfirm(null)
          }}
          onCancel={() => setPendingConfirm(null)}
        />
      )}
      {logText !== null && <LogModal text={logText} onClose={() => setLogText(null)} />}
    </div>
  )
}

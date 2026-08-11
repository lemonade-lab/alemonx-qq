import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { fetchLocalServices, fetchRobotProjects, fetchStatus, napcatQRCodeURL, preflightPrivilege, runActionAndPoll, runNapcatDependenciesAndPoll, syncRobotOneBot, type ActionResult, type LocalService, type PrivilegePreflight, type RobotProject, type StatusPayload } from './api'
import { splitStatusLines, type StatusLine } from './status'

type View = 'manage' | 'config' | 'webui'
type Engine = 'napcat' | 'luckylillia'

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
  result
}: {
  state: 'idle' | 'running' | 'done' | 'failed'
  result?: ActionResult
}) {
  const lines = useMemo(() => splitStatusLines(result?.output ?? ''), [result])
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
        <div className="whitespace-pre-wrap rounded-md bg-[var(--theme-danger-soft)] px-2 py-1.5 font-semibold leading-5 text-[var(--theme-danger-text)]">
          {result.error}
        </div>
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

function ActionField({ label = '操作', children }: { label?: string; children: ReactNode }) {
	return (
		<label className="grid min-w-[180px] flex-1 gap-1 text-xs font-semibold text-[var(--theme-text-secondary)] [&_button]:w-full">
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

function SudoPasswordModal({
  onSubmit,
  onCancel,
  systemAccount,
	preflight,
	serverError
}: {
  onSubmit: (password: string) => void
  onCancel: () => void
  systemAccount?: string
	preflight?: PrivilegePreflight
	serverError?: string
}) {
  const [password, setPassword] = useState('')
  const [confirmation, setConfirmation] = useState('')
  const [error, setError] = useState('')
  const submit = () => {
    if (!password) {
      setError('请输入当前系统账户的 sudo 密码。')
      return
    }
    if (password !== confirmation) {
      setError('两次输入的密码不一致。')
      return
    }
    const value = password
    setPassword('')
    setConfirmation('')
    onSubmit(value)
  }
  const unavailable = !preflight?.available || !preflight.intentId || preflight.authorization !== 'password'
  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-[var(--theme-surface-overlay)]">
      <div className="grid w-[min(460px,calc(100vw-32px))] gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-4 shadow-[var(--theme-shadow-pop)]">
        <div className="grid gap-1">
          <h3 className="m-0 text-[15px] font-semibold text-[var(--theme-text-strong)]">{preflight?.title || '系统权限请求'}</h3>
          <p className="m-0 text-[13px] leading-5 text-[var(--theme-text-muted)]">{preflight?.description || '正在检查本次系统权限请求。'}</p>
          {systemAccount && <p className="m-0 text-xs text-[var(--theme-text-secondary)]">系统账户：<code>{systemAccount}</code></p>}
        </div>
        {unavailable ? <p className="m-0 rounded-md bg-[var(--theme-warning-soft)] px-2 py-1.5 text-xs text-[var(--theme-warning-text)]">{preflight?.reason || '当前无法发起系统授权。请在本机桌面工作台中重试。'}</p> : <>
          <label className="grid gap-1 text-xs font-semibold text-[var(--theme-text-secondary)]">sudo 密码
            <input autoFocus className={inputClass} type="password" autoComplete="current-password" value={password} onChange={event => { setPassword(event.target.value); setError('') }} />
          </label>
          <label className="grid gap-1 text-xs font-semibold text-[var(--theme-text-secondary)]">确认 sudo 密码
            <input className={inputClass} type="password" autoComplete="current-password" value={confirmation} onChange={event => { setConfirmation(event.target.value); setError('') }} onKeyDown={event => { if (event.key === 'Enter') submit() }} />
          </label>
        </>}
        {(error || serverError) && <p className="m-0 rounded-md bg-[var(--theme-danger-soft)] px-2 py-1.5 text-xs text-[var(--theme-danger-text)]">{error || serverError}</p>}
        <div className="flex justify-end gap-2">
          <button className="secondary-button" onClick={() => { setPassword(''); setConfirmation(''); onCancel() }}>取消</button>
          {!unavailable && <button className="danger-button" onClick={submit}>确认授权</button>}
        </div>
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
	if (action === 'install' || action.endsWith('-install')) return '正在下载并安装核心'
	if (action === 'start' || action.endsWith('-start')) return '正在启动 QQ 服务并等待二维码'
	if (action === 'stop' || action.endsWith('-stop')) return '正在停止核心服务'
	if (action.includes('update')) return '正在检查或更新核心版本'
	if (action.includes('onebot')) return '正在保存 OneBot 连接配置'
	return '正在执行操作'
}

function SetupSteps({ status, engine }: { status: StatusPayload; engine: Engine }) {
	const steps = engine === 'napcat'
		? [
			['安装核心', status.installed],
			['启动服务', status.running],
			['QQ 登录', status.running && !status.loginPending],
			['OneBot 就绪', Boolean(status.oneBotReady)]
		]
		: [
			['安装核心', status.installed],
			['启动服务', status.running],
			['登录完成', status.running && !status.loginPending],
			['OneBot 就绪', Boolean(status.oneBotReady)]
		]
	return (
		<div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
			{steps.map(([label, complete]) => (
				<div key={String(label)} className="flex items-center gap-1.5 rounded-md bg-[var(--theme-surface-hover)] px-2 py-1.5 text-xs">
					<span className={'grid size-4 place-items-center rounded-full text-[10px] font-bold ' + (complete ? 'bg-[var(--theme-success)] text-white' : 'bg-[var(--theme-border-strong)] text-[var(--theme-text-muted)]')}>{complete ? '✓' : '·'}</span>
					<span className={complete ? 'text-[var(--theme-text-strong)]' : 'text-[var(--theme-text-muted)]'}>{label}</span>
				</div>
			))}
		</div>
	)
}

export default function App() {
  const [view, setView] = useState<View>('manage')
	const [engine, setEngine] = useState<Engine>('napcat')
  const [result, setResult] = useState<ActionResult | undefined>()
	const [activeAction, setActiveAction] = useState<string | null>(null)
  const [state, setState] = useState<'idle' | 'running' | 'done' | 'failed'>(
    'idle'
  )
  const [pendingConfirm, setPendingConfirm] = useState<{
    title: string
    description: string
    action: () => Promise<void>
  } | null>(null)
	const [sudoPromptOpen, setSudoPromptOpen] = useState(false)
	const [napcatPreflight, setNapcatPreflight] = useState<PrivilegePreflight>()
	const [napcatPrivilegeError, setNapcatPrivilegeError] = useState('')
	// Set only after the user explicitly confirms the NapCat installation. It
	// never comes from an installer error, so an upstream script cannot turn a
	// password prompt into an arbitrary privileged retry.
	const resumeNapcatInstallRef = useRef(false)
  const [liveStatus, setLiveStatus] = useState<StatusPayload | null>(null)
	const [projects, setProjects] = useState<RobotProject[]>([])
	const [services, setServices] = useState<LocalService[]>([])
	const [robotRoot, setRobotRoot] = useState('')
	const [syncToken, setSyncToken] = useState('')
	const webServiceID = engine === 'napcat' ? 'napcat-webui' : 'luckylillia-webui'
	const webService = services.find(service => service.id === webServiceID)
	const webUrl = webService?.reachable && webService.embed ? webService.proxyUrl : ''
	const qrImageUrl = liveStatus?.qrCodeAvailable
		? napcatQRCodeURL(liveStatus.qrCodeUpdatedAt)
		: ''
	const luckyAction = (action: string) => `luckylillia-${action}`
	const luckyManaged = liveStatus?.managed === true
	const luckyInstalled = liveStatus?.installed === true
	const napcatManagedActions = engine === 'napcat' && liveStatus?.managedActions === true
	const [napcatQQ, setNapcatQQ] = useState('')
	const selectedNapcatAccount = liveStatus?.accounts?.find(account => account.qq === (napcatQQ || liveStatus.selectedAccount))
	const selectedOneBotReady = engine === 'napcat' ? Boolean(selectedNapcatAccount?.oneBotReady) : Boolean(liveStatus?.oneBotReady)
	const selectedOneBotURL = engine === 'napcat'
		? selectedNapcatAccount?.oneBotUrl
		: liveStatus?.oneBotUrl

	const run = async (action: string, params: Record<string, string> = {}, confirm = false) => {
	setActiveAction(action)
    setState('running')
    setResult(undefined)
    try {
		const outcome = await runActionAndPoll(action, params, confirm, task => {
		setResult({ output: task.output || (task.progress ? `正在执行（${task.progress}%）` : '正在执行…') })
	  })
      setResult(outcome)
      setState(outcome.error ? 'failed' : 'done')
    } catch (reason) {
      setResult({ output: '', error: reason instanceof Error ? reason.message : String(reason) })
      setState('failed')
	} finally {
		setActiveAction(null)
	  }
	}

	const installNapcatDependencies = (password: string) => {
		if (!napcatPreflight?.intentId) return
		setSudoPromptOpen(false)
		setNapcatPrivilegeError('')
		setActiveAction('napcat-install-dependencies')
		setState('running')
		setResult(undefined)
		void runNapcatDependenciesAndPoll(password, napcatPreflight.intentId, task => setResult({ output: task.output || (task.progress ? `正在执行（${task.progress}%）` : '正在执行…') }))
			.then(async outcome => {
				const resumeInstall = resumeNapcatInstallRef.current
				resumeNapcatInstallRef.current = false
				if (outcome.error) {
					setResult(outcome)
					setState('failed')
					if (outcome.error.includes('权限请求已') || outcome.error.includes('请先在工作台确认')) {
						await requestNapcatDependencyAuthorization(resumeInstall)
						setNapcatPrivilegeError('权限请求已刷新，请重新输入密码后继续。')
						return
					}
					setNapcatPrivilegeError(outcome.error)
					setSudoPromptOpen(true)
					return
				}
				if (resumeInstall) {
					setResult({ output: `${outcome.output}\n依赖已补齐，正在继续安装 NapCat…` })
					await run('install', {}, true)
					return
				}
				setResult(outcome)
				setState('done')
			})
			.catch(reason => { const message = reason instanceof Error ? reason.message : String(reason); resumeNapcatInstallRef.current = false; setResult({ output: '', error: message }); setNapcatPrivilegeError(message); setSudoPromptOpen(true); setState('failed') })
			.finally(() => setActiveAction(null))
	}

	const requestNapcatInstall = async () => {
		if (!liveStatus?.verified) {
			setResult({ output: '', error: liveStatus?.verificationReason || '当前平台暂不支持自动安装 NapCat。' })
			setState('failed')
			return
		}
		if (liveStatus.linuxDependencies && !liveStatus.linuxDependencies.ready) {
			await requestNapcatDependencyAuthorization(true)
			return
		}
		await run('install', {}, true)
	}

	const requestNapcatDependencyAuthorization = async (resumeInstall: boolean) => {
		resumeNapcatInstallRef.current = resumeInstall
		setNapcatPrivilegeError('')
		try {
			const preflight = await preflightPrivilege('napcat-install-dependencies')
			setNapcatPreflight(preflight)
			setSudoPromptOpen(true)
		} catch (reason) {
			resumeNapcatInstallRef.current = false
			setResult({ output: '', error: reason instanceof Error ? reason.message : String(reason) })
			setState('failed')
		}
	}

	const cancelSudoPrompt = () => {
		resumeNapcatInstallRef.current = false
		setNapcatPreflight(undefined)
		setNapcatPrivilegeError('')
		setSudoPromptOpen(false)
	}

	// Login QR codes are short-lived. Poll the read-only endpoint faster only
	// while login is pending; hidden pages do not need background refreshes.
  useEffect(() => {
    let stopped = false
		let timer: ReturnType<typeof setTimeout> | undefined
    const tick = async () => {
			if (document.hidden) return
		try {
			const [status, nextServices] = await Promise.all([fetchStatus(engine), fetchLocalServices()])
			if (!stopped) {
				setLiveStatus(status)
				setServices(nextServices)
				setNapcatQQ(current => current || status.selectedAccount || '')
			}
      } catch {
		// Probe failed; keep last known status.
      }
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
	}, [engine, liveStatus?.loginPending])

	useEffect(() => {
		void fetchRobotProjects().then(setProjects).catch(() => setProjects([]))
	}, [])

  const confirm = (title: string, description: string, action: () => Promise<void>) => {
    setPendingConfirm({ title, description, action })
  }

	const guide = useMemo(() => {
		if (!liveStatus) return null
		if (engine === 'napcat' && liveStatus.platform === 'darwin-external' && !liveStatus.installed) return {
			title: '关联现有 NapCat',
			description: 'macOS 的 NapCat 通过 QQ 注入运行。工作台不会修改 QQ、停止 QQ 或删除注入文件；请关联现有目录后查看状态、二维码与 WebUI。',
			label: '关联目录',
			action: () => document.getElementById('napcat-association')?.scrollIntoView({ behavior: 'smooth', block: 'center' }),
		}
		if (engine === 'napcat' && !liveStatus.installed && !liveStatus.verified) return { title: '当前平台暂不支持自动安装', description: liveStatus.verificationReason || '请使用当前系统的官方安装方式，或关联已有 NapCat。', label: '查看状态', action: () => void run('napcat-status') }
		if (engine === 'napcat' && liveStatus.installed && !liveStatus.managed) return { title: '已关联外部 NapCat', description: '工作台只显示状态、登录二维码和内嵌 WebUI，不会启动、停止、更新、写配置或删除外部目录。', label: webUrl ? '打开管理面板' : '查看状态', action: () => webUrl ? setView('webui') : void run('napcat-status') }
		if (engine === 'napcat' && liveStatus.installed && !liveStatus.managedActions) return { title: '需要修复受管安装', description: '安装目录或运行文件与记录不一致。为保护已有 QQ 环境，请重装，或改为关联外部实例。', label: '查看状态', action: () => void run('napcat-status') }
		if (engine === 'luckylillia' && liveStatus.supported === false) return { title: '当前平台不支持 LuckyLillia CLI', description: 'MVP 仅支持 Windows x64、macOS Apple Silicon、Linux x64 与 Linux ARM64。', label: '查看状态', action: () => void run(luckyAction('status')) }
		if (engine === 'luckylillia' && !liveStatus.verified) return { title: '当前平台暂不支持自动安装', description: `${liveStatus.platform || '当前平台'} 没有可用的 LuckyLillia 官方 CLI 包。可使用官方方式安装后再关联目录。`, label: '查看状态', action: () => void run(luckyAction('status')) }
		if (!liveStatus.installed) return { title: '第一步：安装核心', description: '下载官方组件并准备本机运行环境。', label: engine === 'napcat' ? '安装 NapCat' : '安装 LuckyLillia', action: () => confirm('安装 QQ 核心', engine === 'napcat' && liveStatus.linuxDependencies && !liveStatus.linuxDependencies.ready ? '将先通过一次 sudo 授权补齐固定 Linux 依赖，成功后自动继续安装 NapCat。' : '将下载并安装官方组件；安装过程可能需要几分钟。', () => engine === 'napcat' ? requestNapcatInstall() : run(luckyAction('install'), {}, true)) }
		if (engine === 'luckylillia' && !liveStatus.managed) return { title: '已关联外部 LuckyLillia', description: '工作台仅显示状态和内嵌 WebUI；不会替你启动、更新、写配置或删除外部目录。', label: webUrl ? '打开管理面板' : '查看状态', action: () => webUrl ? setView('webui') : void run(luckyAction('status')) }
		if (!liveStatus.running) return { title: '第二步：启动服务', description: '启动后将自动等待 QQ 登录二维码。', label: engine === 'napcat' ? '启动 NapCat' : '启动 LuckyLillia', action: () => confirm('启动 QQ 核心', '启动后台服务并等待登录。', () => run(engine === 'napcat' ? 'start' : luckyAction('start'), {}, true)) }
		if (liveStatus.loginPending) return { title: '第三步：使用手机 QQ 扫码', description: '二维码会自动刷新；完成登录后状态会自动进入下一步。', label: webUrl ? '打开管理面板' : '等待二维码', action: () => webUrl ? setView('webui') : undefined }
		if (!liveStatus.oneBotReady) return { title: '正在等待 OneBot 服务', description: 'QQ 登录后的服务初始化可能需要片刻，请保持此页面打开。', label: '刷新状态', action: () => void run(engine === 'napcat' ? 'napcat-status' : luckyAction('status')) }
		return { title: 'QQ已就绪', description: '现在可以同步 OneBot 连接到 AlemonJS 机器人，或打开管理面板。', label: '打开管理面板', action: () => setView('webui') }
	}, [engine, liveStatus, webUrl])

  return (
    <div className="mx-auto grid max-w-[860px] gap-4 p-4">
      <header className="flex items-baseline justify-between gap-3 border-b border-[var(--theme-border-default)] pb-3">
        <div>
          <h1 className="m-0 text-base font-semibold tracking-tight text-[var(--theme-text-strong)]">
            QQ 内核管理
          </h1>
          <p className="m-0 mt-0.5 text-xs text-[var(--theme-text-muted)]">
            管理 NapCat 与 LuckyLillia，连接到 AlemonJS OneBot
          </p>
        </div>
		<div className="flex gap-1">
			{(['napcat', 'luckylillia'] as Engine[]).map(item => (
				<button key={item} className={engine === item ? 'primary-button' : 'secondary-button'} onClick={() => { resumeNapcatInstallRef.current = false; setSudoPromptOpen(false); setEngine(item); setView('manage'); setResult(undefined); setState('idle') }}>
					{item === 'napcat' ? 'NapCat' : 'LuckyLillia'}
				</button>
			))}
		</div>
      </header>

      <nav className="flex gap-1 border-b border-[var(--theme-border-default)] pb-2">
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
      </nav>

      {view === 'manage' && (
        <div className="grid gap-3">
		  {guide && (
			<section className="grid gap-3 rounded-panel border border-[var(--theme-border-strong)] bg-[var(--theme-surface-panel)] p-4">
				<div className="flex flex-wrap items-start justify-between gap-3">
					<div className="grid gap-1">
						<strong className="text-sm font-semibold text-[var(--theme-text-strong)]">{guide.title}</strong>
						<p className="m-0 text-xs leading-5 text-[var(--theme-text-muted)]">{guide.description}</p>
					</div>
					<ActionButton label={state === 'running' ? actionTitle(activeAction) : guide.label} running={state === 'running'} disabled={guide.label === '等待二维码'} onClick={guide.action} />
				</div>
				{liveStatus && <SetupSteps status={liveStatus} engine={engine} />}
			</section>
		  )}
		  {liveStatus && (
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
					{liveStatus.installed ? `已安装 · 版本 ${liveStatus.version || '?'}` : '未安装'}
                  {liveStatus.running ? ' · 进程运行中' : ' · 进程未运行'}
					{liveStatus.webUiReady || liveStatus.portReachable ? ' · WebUI 可访问' : ' · WebUI 不可达'}
					{liveStatus.loginPending ? ' · 等待登录' : liveStatus.oneBotReady ? ' · OneBot 就绪' : ''}
                </span>
              </div>
              {liveStatus.error && (
                <p className="m-0 rounded-md bg-[var(--theme-danger-soft)] px-2 py-1.5 font-semibold text-[var(--theme-danger-text)]">
                  {liveStatus.error}
                </p>
              )}
				{liveStatus.diagnosticHint && <p className="m-0 rounded-md bg-[var(--theme-warning-soft)] px-2 py-1.5 text-[var(--theme-warning-text)]">{liveStatus.diagnosticHint}</p>}
				{engine === 'luckylillia' && liveStatus.assetName && <p className="m-0 text-xs text-[var(--theme-text-muted)]">官方 CLI 包：<code>{liveStatus.assetName}</code>{liveStatus.entrypoint ? ` · 启动入口 ${liveStatus.entrypoint}` : ''}</p>}
			  {engine === 'napcat' && napcatManagedActions && liveStatus.installed && !liveStatus.running && (
                <div className="flex gap-2">
                  <ActionButton label="一键重启" running={state === 'running'} onClick={() => void run('restart', {}, true)} />
                </div>
              )}
				{engine === 'napcat' && napcatManagedActions && <div className="flex items-center gap-2 border-t border-[var(--theme-border-default)] pt-2">
                <span className="text-[var(--theme-text-muted)]">
                  守护模式（自动拉起）：
                </span>
                {liveStatus.watchdog ? (
                  <ActionButton label="关闭守护" variant="secondary" running={state === 'running'} onClick={() => confirm('关闭守护模式', 'NapCat 异常退出后不再自动拉起。', () => run('watchdog-off', {}, true))} />
                ) : (
                  <ActionButton label="开启守护" variant="secondary" running={state === 'running'} onClick={() => confirm('开启守护模式', 'NapCat 异常退出后约 15 秒会自动拉起。', () => run('watchdog-on', {}, true))} />
                )}
				</div>}
			</section>
		  )}

		  {engine === 'napcat' && liveStatus?.linuxDependencies && !liveStatus.linuxDependencies.ready && (
			<section className="grid gap-3 rounded-panel border border-[var(--theme-warning)] bg-[var(--theme-warning-soft)] p-3">
				<div className="grid gap-1">
					<strong className="text-sm font-semibold text-[var(--theme-text-strong)]">补齐 Linux 运行依赖</strong>
					<p className="m-0 text-xs leading-5 text-[var(--theme-text-muted)]">{liveStatus.linuxDependencies.hint || 'NapCat 需要系统运行依赖。'}</p>
					{liveStatus.linuxDependencies.missing?.length ? <p className="m-0 text-xs text-[var(--theme-text-secondary)]">缺少：<code>{liveStatus.linuxDependencies.missing.join('、')}</code></p> : null}
				</div>
				<ActionButton label="安装系统依赖" running={state === 'running'} disabled={!liveStatus.linuxDependencies.supported} onClick={() => confirm('安装 NapCat Linux 系统依赖', '工作台会先说明本次系统权限请求；在本机可用时再输入一次 sudo 密码。', () => requestNapcatDependencyAuthorization(false))} />
			</section>
		  )}

		  {engine === 'luckylillia' && liveStatus?.supported !== false && !liveStatus?.installed && (
			<section className="grid gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3">
				<div className="grid gap-1">
					<strong className="text-sm font-semibold text-[var(--theme-text-strong)]">关联已解压的官方 CLI</strong>
					<p className="m-0 text-xs leading-5 text-[var(--theme-text-muted)]">可选：关联已解压的同平台官方 CLI 包。系统只接受当前平台的标准启动入口。</p>
				</div>
				<form className="grid grid-cols-[minmax(0,1fr)_180px] gap-3 max-sm:grid-cols-1" onSubmit={(event) => {
					event.preventDefault()
					const installDir = String(new FormData(event.currentTarget).get('installDir') || '')
					confirm('关联 LuckyLillia CLI', '工作台将校验当前平台的官方 CLI 启动入口并保存目录。', () => run(luckyAction('adopt'), { installDir }, true))
				}}>
					<Field label="CLI 解压目录" name="installDir" hint="请输入绝对路径，例如 /opt/LLBot 或 D:\\LLBot。" />
					<ActionField><button className="primary-button min-h-9" type="submit">关联目录</button></ActionField>
				</form>
			</section>
		  )}

		  {engine === 'napcat' && !liveStatus?.managed && (
			<section id="napcat-association" className="grid gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3">
				<div className="grid gap-1">
					<strong className="text-sm font-semibold text-[var(--theme-text-strong)]">关联现有 NapCat</strong>
					<p className="m-0 text-xs leading-5 text-[var(--theme-text-muted)]">仅保存目录与指纹，用于状态、二维码和内嵌 WebUI。不会修改此目录；macOS 留空可使用 QQ 注入目录。</p>
				</div>
				<form className="grid grid-cols-[minmax(0,1fr)_180px] gap-3 max-sm:grid-cols-1" onSubmit={(event) => {
					event.preventDefault()
					const installDir = String(new FormData(event.currentTarget).get('installDir') || '')
					confirm('关联外部 NapCat', '工作台只读取该目录的状态和登录信息，不会获取删除或运行权限。', () => run('napcat-adopt', { installDir }, true))
				}}>
					<Field label="NapCat 安装目录" name="installDir" hint="请输入绝对路径；macOS 可留空以自动定位 QQ 注入目录。" />
					<ActionField><button className="primary-button min-h-9" type="submit">关联目录</button></ActionField>
				</form>
			</section>
		  )}

		  <details className="rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3">
			<summary className="cursor-pointer text-xs font-semibold text-[var(--theme-text-secondary)]">其他操作与诊断（更新、日志、重启、卸载）</summary>
			<div className="mt-3 flex flex-wrap gap-2">
			{engine === 'napcat' ? <>
				<ActionButton label="查看状态" variant="secondary" running={state === 'running'} onClick={() => void run('napcat-status')} />
				<ActionButton label="安装" running={state === 'running'} disabled={!liveStatus?.verified || liveStatus?.installed} onClick={() => confirm('安装 NapCat', liveStatus?.linuxDependencies && !liveStatus.linuxDependencies.ready ? '将先通过一次 sudo 授权补齐固定 Linux 依赖，成功后自动继续安装 NapCat。' : '会下载并校验官方组件，然后完成安装。', requestNapcatInstall)} />
				<ActionButton label="启动" variant="secondary" running={state === 'running'} disabled={!napcatManagedActions || !liveStatus?.installed} onClick={() => confirm('启动 NapCat', '启动工作台受管的后台进程，用手机 QQ 扫码登录。', () => run('start', {}, true))} />
				<ActionButton label="停止" variant="secondary" running={state === 'running'} disabled={!napcatManagedActions || !liveStatus?.running} onClick={() => confirm('停止 NapCat', '停止工作台受管的 NapCat 进程组。', () => run('stop', {}, true))} />
				<ActionButton label="重启" variant="secondary" running={state === 'running'} disabled={!napcatManagedActions || !liveStatus?.installed} onClick={() => confirm('重启 NapCat', '停止后重新启动工作台受管的 NapCat。', () => run('restart', {}, true))} />
				{liveStatus?.installed && !liveStatus?.managed ? <ActionButton label="取消关联" variant="danger" running={state === 'running'} onClick={() => confirm('取消关联 NapCat', '不会删除或修改外部目录。', () => run('napcat-forget', {}, true))} /> : <ActionButton label="卸载" variant="danger" running={state === 'running'} disabled={!napcatManagedActions} onClick={() => confirm('卸载 NapCat', '会停止并删除工作台受管目录。', () => run('uninstall', {}, true))} />}
				<ActionButton label="看日志" variant="secondary" running={state === 'running'} onClick={() => void run('log')} />
				<ActionButton label="检查更新" variant="secondary" running={state === 'running'} onClick={() => void run('update-check')} />
				<ActionButton label="更新" variant="secondary" running={state === 'running'} disabled={!napcatManagedActions} onClick={() => confirm('更新 NapCat', '会停止旧进程，原子替换；失败时恢复旧目录与原运行状态。', () => run('update', {}, true))} />
			</> : <>
				<ActionButton label="查看状态" variant="secondary" running={state === 'running'} onClick={() => void run(luckyAction('status'))} />
				<ActionButton label="安装" running={state === 'running'} disabled={!liveStatus?.verified || luckyInstalled} onClick={() => confirm('安装 LuckyLillia', `将从官方 Release 下载并验证 ${liveStatus?.assetName || '当前平台安装包'}。`, () => run(luckyAction('install'), {}, true))} />
				<ActionButton label="重装" variant="secondary" running={state === 'running'} disabled={!liveStatus?.verified || !luckyManaged} onClick={() => confirm('重装 LuckyLillia', '会停止旧进程，原子替换并在失败时回滚。', () => run(luckyAction('reinstall'), {}, true))} />
				<ActionButton label="启动" variant="secondary" running={state === 'running'} disabled={!liveStatus?.verified || !luckyManaged || !luckyInstalled} onClick={() => confirm('启动 LuckyLillia', '将启动官方 CLI 并等待 WebUI（3080）就绪。', () => run(luckyAction('start'), {}, true))} />
				<ActionButton label="停止" variant="secondary" running={state === 'running'} disabled={!luckyManaged || !liveStatus?.running} onClick={() => confirm('停止 LuckyLillia', '停止由工作台管理的 LuckyLillia 进程。', () => run(luckyAction('stop'), {}, true))} />
				<ActionButton label="重启" variant="secondary" running={state === 'running'} disabled={!liveStatus?.verified || !luckyManaged || !luckyInstalled} onClick={() => confirm('重启 LuckyLillia', '会使用 LuckyLillia 专属停止与启动流程。', () => run(luckyAction('restart'), {}, true))} />
				{luckyInstalled && (luckyManaged ? <ActionButton label="卸载" variant="danger" running={state === 'running'} onClick={() => confirm('卸载 LuckyLillia', '会停止并删除工作台安装的 LuckyLillia。', () => run(luckyAction('uninstall'), {}, true))} /> : <ActionButton label="取消关联" variant="danger" running={state === 'running'} onClick={() => confirm('取消关联 LuckyLillia', '不会删除外部目录或修改其中的文件。', () => run(luckyAction('forget'), {}, true))} />)}
				<ActionButton label="看日志" variant="secondary" running={state === 'running'} onClick={() => void run(luckyAction('log'))} />
				<ActionButton label="检查更新" variant="secondary" running={state === 'running'} disabled={!liveStatus?.verified || !luckyManaged} onClick={() => void run(luckyAction('update-check'))} />
				<ActionButton label="更新" variant="secondary" running={state === 'running'} disabled={!liveStatus?.verified || !luckyManaged} onClick={() => confirm('更新 LuckyLillia', '会停止旧进程，下载新版并在失败时恢复旧版本与进程。', () => run(luckyAction('update'), {}, true))} />
			</>}
			</div>
		  </details>


		  <ResultPanel state={state} result={state === 'running' ? { output: actionTitle(activeAction) } : result} />

		  {engine === 'napcat' && liveStatus?.loginPending && (
			<section className="grid justify-items-center gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-4 text-center">
				<div>
					<h2 className="m-0 text-sm font-semibold text-[var(--theme-text-strong)]">QQ 扫码登录</h2>
					<p className="m-0 mt-1 text-xs text-[var(--theme-text-muted)]">请使用手机 QQ 扫描二维码。二维码刷新后会自动更新。</p>
				</div>
				{qrImageUrl ? (
					<img className="size-56 rounded-lg bg-white p-2" src={qrImageUrl} alt="NapCat QQ 登录二维码" />
				) : (
					<p className="m-0 text-xs text-[var(--theme-text-muted)]">正在等待 NapCat 生成二维码…</p>
				)}
			</section>
		  )}

          {webUrl && (
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
          <ResultPanel state={state} result={result} />

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
			<ActionField><button className="primary-button min-h-9" type="submit" disabled={!liveStatus?.verified || !liveStatus?.managed}>保存连接</button></ActionField>
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
		{sudoPromptOpen && <SudoPasswordModal onSubmit={installNapcatDependencies} onCancel={cancelSudoPrompt} systemAccount={liveStatus?.linuxDependencies?.systemAccount} preflight={napcatPreflight} serverError={napcatPrivilegeError} />}
    </div>
  )
}

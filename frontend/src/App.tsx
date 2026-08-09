import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { fetchLocalServices, fetchRobotProjects, fetchStatus, napcatQRCodeURL, runActionAndPoll, syncRobotOneBot, type ActionResult, type LocalService, type RobotProject, type StatusPayload } from './api'
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
        <div className="rounded-md bg-[var(--theme-danger-soft)] px-2 py-1.5 font-semibold text-[var(--theme-danger-text)]">
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

  const run = async (action: string, params: Record<string, string> = {}, confirm = false) => {
	setActiveAction(action)
    setState('running')
    setResult(undefined)
    try {
      const outcome = await runActionAndPoll(action, params, confirm)
      setResult(outcome)
      setState(outcome.error ? 'failed' : 'done')
    } catch (reason) {
      setResult({ output: '', error: reason instanceof Error ? reason.message : String(reason) })
      setState('failed')
	} finally {
		setActiveAction(null)
    }
  }

	// NapCat login QR codes are short-lived. Poll faster while this core is
	// selected so a regenerated QR is reflected without opening WebUI.
  useEffect(() => {
    let stopped = false
    const tick = async () => {
		try {
			const status = await fetchStatus(engine)
			const nextServices = await fetchLocalServices()
			if (!stopped) { setLiveStatus(status); setServices(nextServices) }
      } catch {
		// Probe failed; keep last known status.
      }
    }
    void tick()
		const timer = setInterval(tick, engine === 'napcat' ? 2000 : 5000)
    return () => {
      stopped = true
      clearInterval(timer)
    }
	}, [engine])

	useEffect(() => {
		void fetchRobotProjects().then(setProjects).catch(() => setProjects([]))
	}, [])

  const confirm = (title: string, description: string, action: () => Promise<void>) => {
    setPendingConfirm({ title, description, action })
  }

	const guide = useMemo(() => {
		if (!liveStatus) return null
		if (engine === 'luckylillia' && !liveStatus.verified) return { title: '实验能力等待验证', description: 'LuckyLillia 尚未通过真实 Linux ARM64 验证，当前不开放安装或启动。', label: '查看状态', action: () => void run(luckyAction('status')) }
		if (!liveStatus.installed) return { title: '第一步：安装核心', description: '下载官方组件并准备本机运行环境。', label: engine === 'napcat' ? '安装 NapCat' : '安装 LuckyLillia', action: () => confirm('安装 QQ 核心', '将下载并安装官方组件；安装过程可能需要几分钟。', () => run(engine === 'napcat' ? 'install' : luckyAction('install'), {}, true)) }
		if (!liveStatus.running) return { title: '第二步：启动服务', description: '启动后将自动等待 QQ 登录二维码。', label: engine === 'napcat' ? '启动 NapCat' : '启动 LuckyLillia', action: () => confirm('启动 QQ 核心', '启动后台服务并等待登录。', () => run(engine === 'napcat' ? 'start' : luckyAction('start'), {}, true)) }
		if (liveStatus.loginPending) return { title: '第三步：使用手机 QQ 扫码', description: '二维码会自动刷新；完成登录后状态会自动进入下一步。', label: webUrl ? '打开管理面板' : '等待二维码', action: () => webUrl ? setView('webui') : undefined }
		if (!liveStatus.oneBotReady) return { title: '正在等待 OneBot 服务', description: 'QQ 登录后的服务初始化可能需要片刻，请保持此页面打开。', label: '刷新状态', action: () => void run(engine === 'napcat' ? 'napcat-status' : luckyAction('status')) }
		return { title: 'QQ 机器人已就绪', description: '现在可以同步 OneBot 连接到 AlemonJS 机器人，或打开管理面板。', label: '打开管理面板', action: () => setView('webui') }
	}, [engine, liveStatus, webUrl])

  return (
    <div className="mx-auto grid max-w-[860px] gap-4 p-4">
      <header className="flex items-baseline justify-between gap-3 border-b border-[var(--theme-border-default)] pb-3">
        <div>
          <h1 className="m-0 text-base font-semibold tracking-tight text-[var(--theme-text-strong)]">
            QQ 机器人内核管理
          </h1>
          <p className="m-0 mt-0.5 text-xs text-[var(--theme-text-muted)]">
            管理 NapCat 与 LuckyLillia，连接到 AlemonJS OneBot
          </p>
        </div>
		<div className="flex gap-1">
			{(['napcat', 'luckylillia'] as Engine[]).map(item => (
				<button key={item} className={engine === item ? 'primary-button' : 'secondary-button'} onClick={() => { setEngine(item); setView('manage'); setResult(undefined); setState('idle') }}>
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
				{engine === 'luckylillia' && liveStatus.verified === false && <p className="m-0 rounded-md bg-[var(--theme-warning-soft)] px-2 py-1.5 text-[var(--theme-warning-text)]">LuckyLillia 实验能力未验证；正式版暂不提供安装、更新、启动或配置写入。</p>}
				{liveStatus.supported === false && <p className="m-0 rounded-md bg-[var(--theme-warning-soft)] px-2 py-1.5 text-[var(--theme-warning-text)]">当前平台不支持 LuckyLillia 自动安装。</p>}
			  {engine === 'napcat' && liveStatus.installed && !liveStatus.running && (
                <div className="flex gap-2">
                  <ActionButton label="一键重启" running={state === 'running'} onClick={() => void run('restart', {}, true)} />
                </div>
              )}
				{engine === 'napcat' && <div className="flex items-center gap-2 border-t border-[var(--theme-border-default)] pt-2">
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

		  <details className="rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3">
			<summary className="cursor-pointer text-xs font-semibold text-[var(--theme-text-secondary)]">其他操作与诊断（更新、日志、重启、卸载）</summary>
			<div className="mt-3 flex flex-wrap gap-2">
			{engine === 'napcat' ? <>
				<ActionButton label="查看状态" variant="secondary" running={state === 'running'} onClick={() => void run('napcat-status')} />
				<ActionButton label="安装" running={state === 'running'} onClick={() => confirm('安装 NapCat', '会自动下载并解压 NapCat 到本机（首次较大，请耐心）。', () => run('install', {}, true))} />
				<ActionButton label="启动" variant="secondary" running={state === 'running'} onClick={() => confirm('启动 NapCat', '启动后台进程，用手机 QQ 扫码登录。', () => run('start', {}, true))} />
				<ActionButton label="停止" variant="secondary" running={state === 'running'} onClick={() => confirm('停止 NapCat', '停止后台的 NapCat 进程。', () => run('stop', {}, true))} />
				<ActionButton label="重启" variant="secondary" running={state === 'running'} onClick={() => confirm('重启 NapCat', '停止后重新启动。', () => run('restart', {}, true))} />
				<ActionButton label="卸载" variant="danger" running={state === 'running'} onClick={() => confirm('卸载 NapCat', '会停止并删除已安装的 NapCat。', () => run('uninstall', {}, true))} />
				<ActionButton label="看日志" variant="secondary" running={state === 'running'} onClick={() => void run('log')} />
				<ActionButton label="检查更新" variant="secondary" running={state === 'running'} onClick={() => void run('update-check')} />
				<ActionButton label="更新" variant="secondary" running={state === 'running'} onClick={() => confirm('更新 NapCat', '会先停止 NapCat，下载新版，再让你重新启动。', () => run('update', {}, true))} />
			</> : <>
				<ActionButton label="查看状态" variant="secondary" running={state === 'running'} onClick={() => void run(luckyAction('status'))} />
				<ActionButton label="安装" running={state === 'running'} disabled={!liveStatus?.verified} onClick={() => confirm('安装 LuckyLillia', '将从官方 Release 下载并验证 Linux ARM64 安装包。', () => run(luckyAction('install'), {}, true))} />
				<ActionButton label="重装" variant="secondary" running={state === 'running'} disabled={!liveStatus?.verified} onClick={() => confirm('重装 LuckyLillia', '会停止旧进程，原子替换并在失败时回滚。', () => run(luckyAction('reinstall'), {}, true))} />
				<ActionButton label="启动" variant="secondary" running={state === 'running'} disabled={!liveStatus?.verified} onClick={() => confirm('启动 LuckyLillia', '将检查 Node.js 22+，然后启动并等待 WebUI。', () => run(luckyAction('start'), {}, true))} />
				<ActionButton label="停止" variant="secondary" running={state === 'running'} onClick={() => confirm('停止 LuckyLillia', '停止后台 LuckyLillia 进程。', () => run(luckyAction('stop'), {}, true))} />
				<ActionButton label="重启" variant="secondary" running={state === 'running'} disabled={!liveStatus?.verified} onClick={() => confirm('重启 LuckyLillia', '会使用 LuckyLillia 专属停止与启动流程。', () => run(luckyAction('restart'), {}, true))} />
				<ActionButton label="卸载" variant="danger" running={state === 'running'} onClick={() => confirm('卸载 LuckyLillia', '会停止并删除 LuckyLillia 安装目录。', () => run(luckyAction('uninstall'), {}, true))} />
				<ActionButton label="看日志" variant="secondary" running={state === 'running'} onClick={() => void run(luckyAction('log'))} />
				<ActionButton label="检查更新" variant="secondary" running={state === 'running'} disabled={!liveStatus?.verified} onClick={() => void run(luckyAction('update-check'))} />
				<ActionButton label="更新" variant="secondary" running={state === 'running'} disabled={!liveStatus?.verified} onClick={() => confirm('更新 LuckyLillia', '会停止旧进程，下载新版并在失败时恢复旧版本与进程。', () => run(luckyAction('update'), {}, true))} />
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
		  <section className="grid gap-1 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3 text-xs">
			<strong className="text-sm text-[var(--theme-text-strong)]">OneBot 连接健康</strong>
			<p className="m-0 text-[var(--theme-text-muted)]">{liveStatus?.oneBotReady ? `核心已在 ${liveStatus.oneBotUrl || '本机 WebSocket 地址'} 就绪。选择机器人并输入 Token 后即可同步。` : '核心 OneBot 尚未就绪。请先完成 QQ 登录，再同步到机器人。'}</p>
		  </section>
          <div className="flex flex-wrap gap-2">
			<ActionButton label="读取当前配置" variant="secondary" running={state === 'running'} onClick={() => void run(engine === 'napcat' ? 'onebot-config' : luckyAction('onebot-config'))} />
          </div>
          <ResultPanel state={state} result={result} />

          {engine === 'napcat' ? <><form
            className="grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3"
            onSubmit={(event) => {
              event.preventDefault()
              const data = new FormData(event.currentTarget)
              const params = {
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
            <Field label="启用" name="httpEnable" defaultValue="true">
              <select className={inputClass} name="httpEnable" defaultValue="true">
                <option value="true">是</option>
                <option value="false">否</option>
              </select>
            </Field>
            <Field label="端口" name="httpPort" type="number" defaultValue="3000" hint="默认 3000。" />
            <Field label="Token" name="httpToken" hint="留空不改动；填 **** 也视为不改动。" />
            <button className="primary-button self-end" type="submit">保存 HTTP</button>
          </form>

          <form
            className="grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3 rounded-panel border border-[var(--theme-border-default)] bg-[var(--theme-surface-panel)] p-3"
            onSubmit={(event) => {
              event.preventDefault()
              const data = new FormData(event.currentTarget)
              const params = {
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
            <Field label="启用" name="wsEnable" defaultValue="true">
              <select className={inputClass} name="wsEnable" defaultValue="true">
                <option value="true">是</option>
                <option value="false">否</option>
              </select>
            </Field>
            <Field label="端口" name="wsPort" type="number" defaultValue="3001" hint="默认 3001。" />
            <Field label="Token" name="wsToken" hint="留空不改动；填 **** 也视为不改动。" />
            <button className="primary-button self-end" type="submit">保存 WebSocket</button>
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
			<button className="primary-button self-end" type="submit" disabled={!liveStatus?.verified}>保存连接</button>
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
				<ActionButton label="同步连接" running={state === 'running'} disabled={!syncToken.trim()} onClick={() => confirm('同步 OneBot 配置', '将写入目标机器人的 OneBot URL、Token 并切换登录连接；不会重启机器人。', async () => {
					if (!robotRoot) { setResult({ output: '', error: '请选择目标机器人。' }); setState('failed'); return }
					if (!syncToken.trim()) { setResult({ output: '', error: '必须显式输入非空 OneBot Token。' }); setState('failed'); return }
					const url = liveStatus?.oneBotUrl || (engine === 'napcat' ? 'ws://127.0.0.1:3001' : 'ws://127.0.0.1:7199')
					try {
						await syncRobotOneBot(robotRoot, url, syncToken)
						setResult({ output: '✓ OneBot 配置已同步到目标机器人。请按需重启机器人使连接生效。' }); setState('done')
					} catch (reason) { setResult({ output: '', error: reason instanceof Error ? reason.message : String(reason) }); setState('failed') }
				})} />
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
    </div>
  )
}

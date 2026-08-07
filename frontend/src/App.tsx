import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { fetchStatus, runActionAndPoll, type ActionResult, type StatusPayload } from './api'
import { splitStatusLines, type StatusLine } from './status'

type View = 'manage' | 'config' | 'webui'

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
  onClick
}: {
  label: string
  variant?: 'primary' | 'secondary' | 'danger'
  running: boolean
  onClick: () => void
}) {
  const className =
    variant === 'primary'
      ? 'primary-button'
      : variant === 'danger'
        ? 'danger-button'
        : 'secondary-button'
  return (
    <button className={className} disabled={running} onClick={onClick}>
      {label}
    </button>
  )
}

export default function App() {
  const [view, setView] = useState<View>('manage')
  const [result, setResult] = useState<ActionResult | undefined>()
  const [state, setState] = useState<'idle' | 'running' | 'done' | 'failed'>(
    'idle'
  )
  const [pendingConfirm, setPendingConfirm] = useState<{
    title: string
    description: string
    action: () => Promise<void>
  } | null>(null)
  const [liveStatus, setLiveStatus] = useState<StatusPayload | null>(null)
  const webUrl =
    liveStatus?.portReachable ? 'http://127.0.0.1:6099/webui' : ''

  const run = async (action: string, params: Record<string, string> = {}, confirm = false) => {
    setState('running')
    setResult(undefined)
    try {
      const outcome = await runActionAndPoll(action, params, confirm)
      setResult(outcome)
      setState(outcome.error ? 'failed' : 'done')
    } catch (reason) {
      setResult({ output: '', error: reason instanceof Error ? reason.message : String(reason) })
      setState('failed')
    }
  }

  // Poll structured status every 5s so a dead NapCat surfaces as a warning.
  useEffect(() => {
    let stopped = false
    const tick = async () => {
      try {
        const status = await fetchStatus()
        if (!stopped) setLiveStatus(status)
      } catch {
        // probe failed; keep last known status
      }
    }
    void tick()
    const timer = setInterval(tick, 5000)
    return () => {
      stopped = true
      clearInterval(timer)
    }
  }, [])

  const confirm = (title: string, description: string, action: () => Promise<void>) => {
    setPendingConfirm({ title, description, action })
  }

  return (
    <div className="mx-auto grid max-w-[860px] gap-4 p-4">
      <header className="flex items-baseline justify-between gap-3 border-b border-[var(--theme-border-default)] pb-3">
        <div>
          <h1 className="m-0 text-base font-semibold tracking-tight text-[var(--theme-text-strong)]">
            QQ 机器人（NapCat）
          </h1>
          <p className="m-0 mt-0.5 text-xs text-[var(--theme-text-muted)]">
            安装、启动与管理 NapCat，让 QQ 变成机器人
          </p>
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
          {liveStatus && (
            <section className="grid gap-2 rounded-panel border p-3 text-xs"
              style={{ borderColor: 'var(--theme-border-default)', background: 'var(--theme-surface-panel)' }}
            >
              <div className="flex items-center gap-2">
                <span
                  className={
                    'inline-flex size-2.5 rounded-full ' +
                    (liveStatus.installed && liveStatus.running && liveStatus.portReachable
                      ? 'bg-[var(--theme-success)]'
                      : 'bg-[var(--theme-danger)]')
                  }
                />
                <strong className="text-sm font-semibold text-[var(--theme-text-strong)]">
                  {liveStatus.installed && liveStatus.running && liveStatus.portReachable
                    ? '运行正常'
                    : '需要关注'}
                </strong>
                <span className="text-[var(--theme-text-muted)]">
                  {liveStatus.installed ? `已安装 · 版本 ${liveStatus.version || '?'}` : '未安装'}
                  {liveStatus.running ? ' · 进程运行中' : ' · 进程未运行'}
                  {liveStatus.portReachable ? ' · 面板可访问' : ' · 面板不可达'}
                </span>
              </div>
              {liveStatus.error && (
                <p className="m-0 rounded-md bg-[var(--theme-danger-soft)] px-2 py-1.5 font-semibold text-[var(--theme-danger-text)]">
                  {liveStatus.error}
                </p>
              )}
              {liveStatus.installed && !liveStatus.running && (
                <div className="flex gap-2">
                  <ActionButton label="一键重启" running={state === 'running'} onClick={() => void run('restart', {}, true)} />
                </div>
              )}
              <div className="flex items-center gap-2 border-t border-[var(--theme-border-default)] pt-2">
                <span className="text-[var(--theme-text-muted)]">
                  守护模式（自动拉起）：
                </span>
                {liveStatus.watchdog ? (
                  <ActionButton label="关闭守护" variant="secondary" running={state === 'running'} onClick={() => confirm('关闭守护模式', 'NapCat 异常退出后不再自动拉起。', () => run('watchdog-off', {}, true))} />
                ) : (
                  <ActionButton label="开启守护" variant="secondary" running={state === 'running'} onClick={() => confirm('开启守护模式', 'NapCat 异常退出后约 15 秒会自动拉起。', () => run('watchdog-on', {}, true))} />
                )}
              </div>
            </section>
          )}

          <div className="flex flex-wrap gap-2">
            <ActionButton label="查看状态" variant="secondary" running={state === 'running'} onClick={() => void run('status')} />
            <ActionButton label="安装" running={state === 'running'} onClick={() => confirm('安装 NapCat', '会自动下载并解压 NapCat 到本机（首次较大，请耐心）。', () => run('install', {}, true))} />
            <ActionButton label="启动" variant="secondary" running={state === 'running'} onClick={() => confirm('启动 NapCat', '启动后台进程，用手机 QQ 扫码登录。', () => run('start', {}, true))} />
            <ActionButton label="停止" variant="secondary" running={state === 'running'} onClick={() => confirm('停止 NapCat', '停止后台的 NapCat 进程。', () => run('stop', {}, true))} />
            <ActionButton label="重启" variant="secondary" running={state === 'running'} onClick={() => confirm('重启 NapCat', '停止后重新启动。', () => run('restart', {}, true))} />
            <ActionButton label="卸载" variant="danger" running={state === 'running'} onClick={() => confirm('卸载 NapCat', '会停止并删除已安装的 NapCat。', () => run('uninstall', {}, true))} />
            <ActionButton label="看日志" variant="secondary" running={state === 'running'} onClick={() => void run('log')} />
            <ActionButton label="检查更新" variant="secondary" running={state === 'running'} onClick={() => void run('update-check')} />
            <ActionButton label="更新" variant="secondary" running={state === 'running'} onClick={() => confirm('更新 NapCat', '会先停止 NapCat，下载新版，再让你重新启动。', () => run('update', {}, true))} />
          </div>

          <ResultPanel state={state} result={result} />

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
          <div className="flex flex-wrap gap-2">
            <ActionButton label="读取当前配置" variant="secondary" running={state === 'running'} onClick={() => void run('onebot-config')} />
          </div>
          <ResultPanel state={state} result={result} />

          <form
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
          </form>
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

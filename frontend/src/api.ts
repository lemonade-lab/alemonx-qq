// Client for the plugin's action forward API. The UI is served same-origin by
// alx, so plain relative fetches work with the management session.

const PLUGIN_ID = 'alemonx-qq'

export type TaskStep = {
  at: string
  progress: number
  message: string
}

export type Task = {
  id: string
  status: 'running' | 'completed' | 'failed'
  output?: string
  error?: string
	progress?: number
	steps?: TaskStep[]
}

export type ActionResult = { output: string; error?: string }

async function json<T>(response: Response): Promise<T> {
  if (!response.ok) {
    let message = `请求失败（${response.status}）`
    try {
      const payload = await response.json()
      if (typeof payload.error === 'string' && payload.error) message = payload.error
      else if (typeof payload.message === 'string' && payload.message) message = payload.message
    } catch {
      // keep the generic message
    }
    throw new Error(message)
  }
  return response.json() as Promise<T>
}

// runAction submits an action and returns its task id. The web UI owns its own
// confirmation UX for dangerous actions; alx forwards the request to the
// executor, which whitelists supported action names.
export async function runAction(
  action: string,
  params: Record<string, string>,
  confirm = false,
  extra: { sudoPassword?: string; authorizationId?: string } = {}
): Promise<string> {
  const payload = await json<{ id: string }>(
    await fetch(`/api/v1/setup/plugins/${PLUGIN_ID}/actions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action, confirm, params, ...extra })
    })
  )
  return payload.id
}

async function getTask(id: string): Promise<Task | undefined> {
  const response = await fetch(`/api/v1/robot/tasks?id=${encodeURIComponent(id)}`)
  if (response.status === 404) return undefined
  return json<Task>(response)
}

async function pollTask(id: string, onUpdate?: (task: Task) => void): Promise<ActionResult> {
  return new Promise((resolve) => {
    let settled = false
    let source: EventSource | undefined
    const finish = (result: ActionResult) => {
      if (settled) return
      settled = true
      clearInterval(fallback)
      clearTimeout(longRunning)
      source?.close()
      resolve(result)
    }
    const apply = (task: Task) => {
      onUpdate?.(task)
      if (task.status === 'completed') finish({ output: task.output || '' })
      if (task.status === 'failed') finish({ output: task.output || '', error: task.error || '插件操作失败。' })
    }
    const refresh = () => { void getTask(id).then(task => { if (task) apply(task) }).catch(() => undefined) }
    const fallback = setInterval(refresh, 5000)
    const longRunning = setTimeout(() => onUpdate?.({ id, status: 'running', output: '操作仍在后台运行，可离开此页面后再返回查看。', progress: 0 }), 20 * 60 * 1000)
    try {
      source = new EventSource(`/api/v1/robot/events?taskId=${encodeURIComponent(id)}`)
      source.onmessage = event => {
        try {
          const payload = JSON.parse(event.data) as { type?: string; task?: Task }
          if (payload.type === 'task' && payload.task) apply(payload.task)
        } catch { /* fallback polling remains active */ }
      }
      source.onerror = refresh
    } catch { /* browser fallback polling remains active */ }
    refresh()
  })
}

// runActionAndPoll is the single entry used by the views.
export async function runActionAndPoll(
  action: string,
  params: Record<string, string>,
  confirm = false,
  onUpdate?: (task: Task) => void,
  extra: { sudoPassword?: string; authorizationId?: string } = {}
): Promise<ActionResult> {
  const id = await runAction(action, params, confirm, extra)
  return pollTask(id, onUpdate)
}

export type PrivilegePreflight = {
  available: boolean
  authorization: string
  title: string
  description: string
  reason?: string
  intentId?: string
  expiresAt?: string
}

// privilegePreflight asks the host to authorize a manifest-declared system
// operation. For password-authorized operations it returns a one-time intent
// id that the sudo action request must carry.
export async function privilegePreflight(pluginId: string, action: string): Promise<PrivilegePreflight> {
  return json<PrivilegePreflight>(await fetch('/api/v1/system/privileged/preflight', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ pluginId, action })
  }))
}

export type PrivilegedAuditItem = {
  id: string
  action: string
  operation: string
  output: string
  createdAt: string
  legacy?: boolean
}

// fetchPrivilegedAudit reads the host's audit trail for this plugin's
// manifest-declared system operations (for example the Linux dependency
// install). Output text is host-validated and truncated by the host.
export async function fetchPrivilegedAudit(pluginId: string): Promise<PrivilegedAuditItem[]> {
  const payload = await json<{ items?: PrivilegedAuditItem[] }>(await fetch(`/api/v1/system/privileged/audit?plugin=${encodeURIComponent(pluginId)}`))
  return payload.items ?? []
}

export type StatusPayload = {
	engine: 'napcat' | 'luckylillia' | 'snowluma'
  installed: boolean
	installHealthy?: boolean
  running: boolean
  portReachable: boolean
	webUiReady?: boolean
	oneBotReady?: boolean
	loginPending?: boolean
  watchdog: boolean
  version?: string
	pid?: number
	webUiUrl?: string
	oneBotUrl?: string
	qrCodeAvailable?: boolean
	qrCodeUpdatedAt?: string
	installerReady?: boolean
	installerPath?: string
	launcherPath?: string
	logPath?: string
	diagnosticHint?: string
	supported?: boolean
	platform?: string
	managed?: boolean
	authTokenReady?: boolean
	journey?: {
		phase: 'unsupported' | 'install' | 'repair' | 'external' | 'needs-auth-token' | 'start' | 'starting' | 'scan-qq' | 'connecting' | 'ready'
		title: string
		detail: string
		nextAction: 'manual' | 'install' | 'repair' | 'open-webui' | 'auth-token' | 'start' | 'view-log' | 'scan-qq' | 'configure'
	}
	accounts?: Array<{ qq: string; oneBotUrl?: string; oneBotReady: boolean }>
	selectedAccount?: string
	state?: 'not-installed' | 'installing' | 'needs-auth-token' | 'starting' | 'running' | 'login-pending' | 'stopped' | 'failed' | 'unsupported'
	updatedAt?: string
  error?: string
}

export type RobotProject = { root: string; name: string }
export type LocalService = { pluginId: string; id: string; name: string; reachable: boolean; proxyUrl: string; embed: boolean }
type HostContext = { robot?: RobotProject | null }

export function napcatQRCodeURL(updatedAt?: string): string {
	return `/api/v1/setup/plugins/${PLUGIN_ID}/media/napcat-qrcode?updatedAt=${encodeURIComponent(updatedAt || '')}`
}

export function luckyQRCodeURL(updatedAt?: string): string {
	return `/api/v1/setup/plugins/${PLUGIN_ID}/media/luckylillia-qrcode?updatedAt=${encodeURIComponent(updatedAt || '')}`
}

export async function fetchLocalServices(): Promise<LocalService[]> {
	const payload = await json<{ items: LocalService[] }>(await fetch(`/api/v1/services?plugin=${PLUGIN_ID}`))
	return payload.items
}

// Finder is a host capability. The browser only names this plugin's declared
// picker; it never submits a filesystem path or native-dialog command.
export async function chooseSystemPath(pickerId: string): Promise<string> {
	const payload = await json<{ paths: string[] }>(await fetch('/api/v1/system/capabilities/finder', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ pluginId: PLUGIN_ID, pickerId })
	}))
	if (!Array.isArray(payload.paths) || payload.paths.length !== 1 || !payload.paths[0]) {
		throw new Error('请选择一个目录。')
	}
	return payload.paths[0]
}

// fetchStatus uses the workbench's read-only status endpoint. Unlike actions,
// it does not allocate or persist an operation task on each refresh.
export async function fetchStatus(engine: 'napcat' | 'luckylillia' | 'snowluma' = 'napcat'): Promise<StatusPayload> {
	const action = engine === 'napcat' ? 'napcat-status' : engine === 'luckylillia' ? 'luckylillia-status' : 'snowluma-status'
	return json<StatusPayload>(await fetch(`/api/v1/setup/plugins/${PLUGIN_ID}/status?action=${encodeURIComponent(action)}`))
}

// Logs use the same read-only runner route as status. This stays available
// while an installation task owns the normal actions route.
export async function fetchPluginLog(engine: 'napcat' | 'luckylillia' | 'snowluma'): Promise<string> {
	const action = engine === 'napcat' ? 'napcat-log-status' : engine === 'luckylillia' ? 'luckylillia-log-status' : 'snowluma-log-status'
	const payload = await json<{ output?: string }>(await fetch(`/api/v1/setup/plugins/${PLUGIN_ID}/status?action=${encodeURIComponent(action)}`))
	return payload.output || '（日志为空）'
}

// This is the current operation trace, reset by the runner when an install,
// update or start begins. It must not be confused with the historical core log.
export async function fetchOperationLog(engine: 'napcat' | 'luckylillia' | 'snowluma'): Promise<string> {
	const action = engine === 'napcat' ? 'napcat-operation-status' : engine === 'luckylillia' ? 'luckylillia-operation-status' : 'snowluma-operation-status'
	const payload = await json<{ output?: string }>(await fetch(`/api/v1/setup/plugins/${PLUGIN_ID}/status?action=${action}`))
	return payload.output || ''
}

export async function fetchRobotProjects(refresh = false): Promise<RobotProject[]> {
	const payload = await json<{ items: RobotProject[] }>(await fetch(`/api/v1/robot/projects${refresh ? '?refresh=true' : ''}`))
	return payload.items
}

// The host owns the active workspace selection. A plugin may use this narrow,
// validated context as a default but still lets the user choose another
// managed robot for an explicit sync action.
export async function fetchHostRobotContext(): Promise<RobotProject | null> {
	const payload = await json<HostContext>(await fetch(`/api/v1/system/capabilities/context?${new URLSearchParams({ pluginId: PLUGIN_ID, keys: 'robot' })}`))
	return payload.robot ?? null
}

export async function syncRobotOneBot(root: string, url: string, token: string): Promise<void> {
	await json(await fetch('/api/v1/robot/onebot-sync', {
		method: 'POST', headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ root, url, token })
	}))
}

export type HostWebviewOptions = {
	title?: string
	url?: string
	resource?: string
	width?: number
	height?: number
}

// openHostWebview opens a host-managed floating window instead of an inline
// iframe: the workbench owns drag/resize/minimize/maximize and position
// memory, so the plugin never re-implements window chrome.
export async function openHostWebview(options: HostWebviewOptions): Promise<string | undefined> {
	const host = window.ALXHost
	if (!host?.webview) throw new Error('当前 ALemonX 版本不支持宿主 WebView。')
	const result = await host.webview.open(PLUGIN_ID, options)
	if (!result?.ok) throw new Error(result?.error || '打开宿主 WebView 失败。')
	return result.id
}

export async function closeHostWebview(webviewID: string | null | undefined): Promise<void> {
	if (!webviewID) return
	const host = window.ALXHost
	if (!host?.webview) return
	try {
		await host.webview.close(PLUGIN_ID, webviewID)
	} catch {
		// The window may already have been closed by the user.
	}
}

// hostConfirm asks through the host's global modal, falling back to the
// browser confirm dialog on older hosts.
export async function hostConfirm(
	title: string,
	message: string,
	confirmText = '确认执行',
	cancelText = '取消'
): Promise<boolean> {
	const host = window.ALXHost
	if (host?.ui?.modal) {
		const result = await host.ui.modal(PLUGIN_ID, { title, message, confirmText, cancelText })
		return result?.confirmed === true
	}
	return window.confirm(`${title}\n\n${message}`)
}

export function hostAlert(title: string, message: string, confirmText = '知道了'): void {
	const host = window.ALXHost
	if (host?.ui?.alert) {
		void host.ui.alert(PLUGIN_ID, { title, message, confirmText })
	} else {
		window.alert(`${title}\n\n${message}`)
	}
}

declare global {
	interface Window {
		ALXHost?: {
			webview?: {
				open: (
					pluginId: string,
					options: HostWebviewOptions
				) => Promise<{ ok: boolean; id?: string; error?: string }>
				close: (
					pluginId: string,
					webviewID?: string
				) => Promise<{ ok: boolean; closed?: number; error?: string }>
			}
			ui?: {
				alert: (
					pluginId: string,
					options: {
						title?: string
						message?: string
						confirmText?: string
					}
				) => Promise<{ ok: boolean; error?: string }>
				modal: (
					pluginId: string,
					options: {
						title?: string
						message?: string
						confirmText?: string
						cancelText?: string
					}
				) => Promise<{ ok: boolean; confirmed?: boolean; error?: string }>
			}
		}
	}
}

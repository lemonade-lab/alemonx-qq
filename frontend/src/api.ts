// Client for the plugin's action forward API. The UI is served same-origin by
// alx, so plain relative fetches work with the management session.

const PLUGIN_ID = 'alemonx-qq'

type Task = {
  id: string
  status: 'running' | 'completed' | 'failed'
  output?: string
  error?: string
	progress?: number
}

export type ActionResult = { output: string; error?: string }
export type HostPrivilegeStatus = { privilege: { enabled: boolean; mode: string; reason?: string; policyVersion: string } }
export type PrivilegePreflight = { available: boolean; authorization: 'password' | 'native-uac' | 'polkit' | 'unavailable'; title: string; description: string; reason?: string; intentId?: string; expiresAt?: string }

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
  confirm = false
): Promise<string> {
  const payload = await json<{ id: string }>(
    await fetch(`/api/v1/setup/plugins/${PLUGIN_ID}/actions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ action, confirm, params })
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
  onUpdate?: (task: Task) => void
): Promise<ActionResult> {
  const id = await runAction(action, params, confirm)
  return pollTask(id, onUpdate)
}

// NapCat's only password-bearing operation belongs to the host, not the
// plugin action forwarder. Keep the credential out of the generic API.
export async function runNapcatDependenciesAndPoll(
  sudoPassword: string,
	authorizationId: string,
  onUpdate?: (task: Task) => void
): Promise<ActionResult> {
  const payload = await json<{ id: string }>(await fetch('/api/v1/system/privileged/napcat-dependencies', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ action: 'napcat-install-dependencies', confirm: true, sudoPassword, authorizationId })
  }))
  return pollTask(payload.id, onUpdate)
}

export async function preflightPrivilege(action: string): Promise<PrivilegePreflight> {
	return json<PrivilegePreflight>(await fetch('/api/v1/system/privileged/preflight', {
		method: 'POST', headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ pluginId: PLUGIN_ID, action })
	}))
}

export type StatusPayload = {
	engine: 'napcat' | 'luckylillia'
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
	logPath?: string
	diagnosticHint?: string
	supported?: boolean
	verified?: boolean
	verificationReason?: string
	platform?: string
	assetName?: string
	entrypoint?: string
	installMode?: 'verified-release' | 'managed' | 'external'
	managed?: boolean
	managedActions?: boolean
	linuxDependencies?: {
		supported: boolean
		ready: boolean
		packageManager?: string
		systemAccount?: string
		missing?: string[]
		hint?: string
	}
	releaseTag?: string
	archiveSha256?: string
	fingerprint?: string
	validatedAt?: string
	accounts?: Array<{ qq: string; oneBotUrl?: string; oneBotReady: boolean }>
	selectedAccount?: string
	state?: 'not-installed' | 'installing' | 'starting' | 'running' | 'login-pending' | 'stopped' | 'failed' | 'unsupported'
	updatedAt?: string
  error?: string
}

export type RobotProject = { root: string; name: string }
export type LocalService = { pluginId: string; id: string; name: string; reachable: boolean; proxyUrl: string; embed: boolean }

export function napcatQRCodeURL(updatedAt?: string): string {
	return `/api/v1/setup/plugins/${PLUGIN_ID}/qrcode?updatedAt=${encodeURIComponent(updatedAt || '')}`
}

export async function fetchLocalServices(): Promise<LocalService[]> {
	const payload = await json<{ items: LocalService[] }>(await fetch(`/api/v1/services?plugin=${PLUGIN_ID}`))
	return payload.items
}

export async function fetchHostPrivilegeStatus(): Promise<HostPrivilegeStatus> {
	return json<HostPrivilegeStatus>(await fetch('/api/v1/system/privileged/status'))
}

// fetchStatus uses the workbench's read-only status endpoint. Unlike actions,
// it does not allocate or persist an operation task on each refresh.
export async function fetchStatus(engine: 'napcat' | 'luckylillia' = 'napcat'): Promise<StatusPayload> {
	const action = engine === 'napcat' ? 'napcat-status' : 'luckylillia-status'
	return json<StatusPayload>(await fetch(`/api/v1/setup/plugins/${PLUGIN_ID}/status?action=${encodeURIComponent(action)}`))
}

export async function fetchRobotProjects(refresh = false): Promise<RobotProject[]> {
	const payload = await json<{ items: RobotProject[] }>(await fetch(`/api/v1/robot/projects${refresh ? '?refresh=true' : ''}`))
	return payload.items
}

export async function syncRobotOneBot(root: string, url: string, token: string): Promise<void> {
	if (!token.trim()) throw new Error('必须显式输入非空 OneBot Token，空 Token 不会覆盖机器人配置。')
	await json(await fetch('/api/v1/robot/onebot-sync', {
		method: 'POST', headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ root, url, token })
	}))
}

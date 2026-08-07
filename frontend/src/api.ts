// Client for the plugin's action forward API. The UI is served same-origin by
// alx, so plain relative fetches work with the management session.

const PLUGIN_ID = 'alemonx-qq'

type Task = {
  id: string
  status: 'running' | 'completed' | 'failed'
  output?: string
  error?: string
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

async function pollTask(id: string): Promise<ActionResult> {
  for (let attempt = 0; attempt < 60; attempt += 1) {
    await new Promise((resolve) => setTimeout(resolve, 800))
    const data = await json<Task[]>(
      await fetch('/api/v1/robot/tasks')
    ).catch(() => [])
    const task = data.find((item) => item.id === id)
    if (!task) continue
    if (task.status === 'completed') return { output: task.output || '' }
    if (task.status === 'failed')
      return { output: task.output || '', error: task.error || '插件操作失败。' }
  }
  return { output: '', error: '操作超时。' }
}

// runActionAndPoll is the single entry used by the views.
export async function runActionAndPoll(
  action: string,
  params: Record<string, string>,
  confirm = false
): Promise<ActionResult> {
  const id = await runAction(action, params, confirm)
  return pollTask(id)
}

export type StatusPayload = {
  installed: boolean
  running: boolean
  portReachable: boolean
  watchdog: boolean
  version?: string
  error?: string
}

// fetchStatus runs the structured `status` action and parses its JSON output.
export async function fetchStatus(): Promise<StatusPayload> {
  const result = await runActionAndPoll('status', {})
  if (result.error) {
    throw new Error(result.error)
  }
  return JSON.parse(result.output) as StatusPayload
}

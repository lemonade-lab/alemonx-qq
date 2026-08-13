export type QQEngine = 'napcat' | 'luckylillia'
export type QQView = 'manage' | 'config' | 'webui'

export type QQSession = {
  version: 1
  engine: QQEngine
  view: QQView
  robotRoot: string
  napcatQQ: string
}

const storageKey = 'alemonx-qq:ui-session:v1'

export const defaultSession: QQSession = {
  version: 1,
  engine: 'napcat',
  view: 'manage',
  robotRoot: '',
  napcatQQ: ''
}

// Only interface choices are persisted. Runtime state is always read from the
// runner, and tokens, QR images, results and in-flight tasks never enter web
// storage.
export function loadSession(): QQSession {
  try {
    const value = JSON.parse(window.localStorage.getItem(storageKey) || '') as Partial<QQSession>
    if (value.version !== 1) return defaultSession
    return {
      version: 1,
      engine: value.engine === 'luckylillia' ? 'luckylillia' : 'napcat',
      view: value.view === 'config' || value.view === 'webui' ? value.view : 'manage',
      robotRoot: typeof value.robotRoot === 'string' ? value.robotRoot : '',
      napcatQQ: typeof value.napcatQQ === 'string' ? value.napcatQQ : ''
    }
  } catch {
    return defaultSession
  }
}

export function saveSession(session: QQSession) {
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(session))
  } catch {
    // Private browsing or a restricted WebView must not prevent QQ management.
  }
}

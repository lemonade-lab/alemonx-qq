export type QQEngine = 'napcat' | 'luckylillia' | 'snowluma'
export type QQView = 'manage' | 'config'

export type QQSession = {
  version: 2
  engine: QQEngine
  view: QQView
  robotRoot: string
  napcatQQ: string
}

const storageKey = 'alemonx-qq:ui-session:v1'

export const defaultSession: QQSession = {
  version: 2,
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
    const value = JSON.parse(window.localStorage.getItem(storageKey) || '') as {
      version?: number
      engine?: QQEngine
      view?: string
      robotRoot?: string
      napcatQQ?: string
    }
    // Version 1 contained `webui` and `background` routes. Both were
    // transitional surfaces, so returning users land on the new overview.
    if (value.version !== 1 && value.version !== 2) return defaultSession
    return {
      version: 2,
      engine: value.engine === 'luckylillia' || value.engine === 'snowluma' ? value.engine : 'napcat',
      view: value.view === 'config' ? 'config' : 'manage',
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

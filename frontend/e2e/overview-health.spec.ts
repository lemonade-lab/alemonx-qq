import { expect, test } from '@playwright/test'
import { getOverviewHealth } from '../src/components/OverviewCards'
import { defaultSession, loadSession } from '../src/session'

test('maps installation and connection states to a single actionable health summary', () => {
  expect(getOverviewHealth({ engine: 'napcat', installed: false, running: false, portReachable: false, watchdog: false })).toMatchObject({ label: '尚未安装', tone: 'neutral' })
  expect(getOverviewHealth({ engine: 'luckylillia', installed: true, running: true, portReachable: true, loginPending: true, watchdog: true })).toMatchObject({ label: '等待 QQ 登录', tone: 'warning' })
  expect(getOverviewHealth({ engine: 'snowluma', installed: true, running: true, portReachable: true, qqLoggedIn: true, oneBotReady: true, watchdog: true })).toMatchObject({ label: '运行正常', tone: 'success' })
  expect(getOverviewHealth({ engine: 'napcat', installed: true, running: true, portReachable: true, watchdog: true, error: '端口被占用' })).toMatchObject({ label: '需要处理', tone: 'danger' })
})

test('migrates legacy management-panel and runtime tabs to overview', () => {
  const previousWindow = globalThis.window
  const storage = new Map<string, string>()
  Object.defineProperty(globalThis, 'window', {
    configurable: true,
    value: { localStorage: { getItem: (key: string) => storage.get(key) ?? null, setItem: (key: string, value: string) => storage.set(key, value) } }
  })
  try {
    for (const view of ['webui', 'background']) {
      storage.set('alemonx-qq:ui-session:v1', JSON.stringify({ version: 1, engine: 'napcat', view }))
      expect(loadSession()).toMatchObject({ version: 2, view: 'manage' })
    }
  } finally {
    Object.defineProperty(globalThis, 'window', { configurable: true, value: previousWindow })
  }
  expect(defaultSession.view).toBe('manage')
})

test('opens a ready core WebUI directly from overview without a management-panel tab', async ({ page }) => {
  await page.addInitScript(() => {
    ;(window as Window & { openedWebview?: unknown }).ALXHost = {
      webview: {
        open: async (_pluginId, options) => {
          ;(window as Window & { openedWebview?: unknown }).openedWebview = options
          return { ok: true, id: 'webview-1' }
        },
        close: async () => ({ ok: true })
      },
      ui: { modal: async () => ({ ok: true, confirmed: true }), alert: async () => ({ ok: true }) }
    }
  })
  await page.route('**/api/v1/**', route => {
    const url = route.request().url()
    const payload = url.includes('/services')
      ? { items: [{ pluginId: 'alemonx-qq', id: 'napcat-webui', name: 'NapCat WebUI', reachable: true, embed: true, proxyUrl: '/services/napcat' }] }
      : url.includes('/status')
        ? { engine: 'napcat', installed: true, running: true, portReachable: true, webUiReady: true, qqLoggedIn: true, oneBotReady: true, watchdog: true }
        : url.includes('/projects') ? { items: [] }
          : url.includes('/context') ? { robot: null }
            : { items: [] }
    void route.fulfill({ contentType: 'application/json', body: JSON.stringify(payload) })
  })
  await page.goto('/')
  await expect(page.getByRole('button', { name: '打开 NapCat 管理面板 ↗' })).toBeVisible()
  await expect(page.getByRole('button', { name: '管理面板', exact: true })).toHaveCount(0)
  await page.getByRole('button', { name: '打开 NapCat 管理面板 ↗' }).click()
  await expect.poll(() => page.evaluate(() => (window as Window & { openedWebview?: { title?: string } }).openedWebview?.title)).toBe('NapCat 管理面板')
})

test('offers a clear cancel-and-stop exit during QR login', async ({ page }) => {
  await page.route('**/api/v1/**', route => {
    const url = route.request().url()
    const payload = url.includes('/services')
      ? { items: [] }
      : url.includes('/status')
        ? { engine: 'napcat', installed: true, running: true, portReachable: true, loginPending: true, qrCodeAvailable: false, watchdog: true }
        : url.includes('/projects') ? { items: [] }
          : url.includes('/context') ? { robot: null }
            : { items: [] }
    void route.fulfill({ contentType: 'application/json', body: JSON.stringify(payload) })
  })
  await page.goto('/')
  await expect(page.getByRole('heading', { name: '请用手机 QQ 扫码登录' })).toBeVisible()
  await expect(page.getByRole('button', { name: '取消登录并停止' })).toBeVisible()
  await expect(page.getByLabel('核心健康状态')).toHaveCount(0)
})

test('expands and collapses core logs inside more operations', async ({ page }) => {
  await page.route('**/api/v1/**', route => {
    const url = route.request().url()
    const payload = url.includes('napcat-log-status')
      ? { output: 'example core log' }
      : url.includes('/services')
        ? { items: [] }
        : url.includes('/status')
          ? { engine: 'napcat', installed: true, managed: true, running: true, portReachable: true, qqLoggedIn: true, oneBotReady: true, watchdog: true }
          : url.includes('/projects') ? { items: [] }
            : url.includes('/context') ? { robot: null }
              : { items: [] }
    void route.fulfill({ contentType: 'application/json', body: JSON.stringify(payload) })
  })
  await page.goto('/')
  await page.getByText('更多操作', { exact: true }).click()
  await page.getByRole('button', { name: '展开日志' }).click()
  await expect(page.getByLabel('NapCat 核心日志')).toContainText('example core log')
  await expect(page.getByRole('button', { name: '折叠日志' })).toBeVisible()
  await page.getByRole('button', { name: '折叠日志' }).click()
  await expect(page.getByLabel('NapCat 核心日志')).toHaveCount(0)
})

import type { QQEngine, QQView } from '../session'

const engines: QQEngine[] = ['napcat', 'luckylillia', 'snowluma']

const engineLabel: Record<QQEngine, string> = {
  napcat: 'NapCat',
  luckylillia: 'LuckyLillia',
  snowluma: 'SnowLuma'
}

export function PluginNavigation({
  engine,
  view,
  onEngineChange,
  onViewChange
}: {
  engine: QQEngine
  view: QQView
  onEngineChange: (engine: QQEngine) => void
  onViewChange: (view: QQView) => void
}) {
  return (
    <aside className="qq-plugin-sidebar" aria-label="QQ 插件导航">
      <nav className="qq-plugin-nav" aria-label="QQ 内核">
        {engines.map(item => (
          <button key={item} type="button" className="qq-plugin-nav-button" aria-current={engine === item ? 'page' : undefined} onClick={() => onEngineChange(item)}>
            <span className="qq-plugin-nav-icon" data-icon={item} aria-hidden />
            {engineLabel[item]}
          </button>
        ))}
      </nav>
      <nav className="qq-plugin-nav qq-plugin-view-nav" aria-label="QQ 功能">
        <button type="button" className="qq-plugin-nav-button" aria-current={view === 'manage' ? 'page' : undefined} onClick={() => onViewChange('manage')}>
          <span className="qq-plugin-nav-icon" data-icon="manage" aria-hidden />概览
        </button>
        <button type="button" className="qq-plugin-nav-button" aria-current={view === 'config' ? 'page' : undefined} onClick={() => onViewChange('config')}>
          <span className="qq-plugin-nav-icon" data-icon="config" aria-hidden />网络配置
        </button>
      </nav>
    </aside>
  )
}

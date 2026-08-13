import { Component, StrictMode, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import './styles.css'
import App from './App'

class ErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
	state = { error: null as Error | null }

	static getDerivedStateFromError(error: Error) {
		return { error }
	}

	render() {
		if (this.state.error) {
			return (
				<div style={{ margin: '0 auto', maxWidth: 480, padding: 24, fontFamily: 'system-ui, sans-serif', display: 'grid', gap: 12 }}>
					<h1 style={{ margin: 0, fontSize: 16 }}>界面出现异常</h1>
					<p style={{ margin: 0, fontSize: 13, lineHeight: 1.6, color: '#b23a3a' }}>
						页面渲染失败：{String(this.state.error.message || this.state.error)}
					</p>
					<pre style={{ margin: 0, padding: 12, borderRadius: 8, background: '#f5f4f1', fontSize: 12, whiteSpace: 'pre-wrap', wordBreak: 'break-word', maxHeight: 240, overflow: 'auto' }}>
						{this.state.error.stack}
					</pre>
					<button style={{ justifySelf: 'start', padding: '8px 16px', borderRadius: 8, border: '1px solid #d5d2ca', background: '#fff', fontSize: 13, cursor: 'pointer' }} onClick={() => window.location.reload()}>
						重新加载
					</button>
				</div>
			)
		}
		return this.props.children
	}
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ErrorBoundary>
      <App />
    </ErrorBoundary>
  </StrictMode>
)

import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'
import './i18n' // Initialize i18n

async function enableMocking() {
  if (import.meta.env.VITE_USE_MOCK !== 'true') {
    return
  }

  const { worker } = await import('./mocks/browser')
  await worker.start({
    onUnhandledRequest: 'bypass',
  })

  // Auto-inject a mock auth token so the app skips the login screen in mock mode.
  // Only sets it if nothing is already stored (preserves any manually-set token).
  if (!localStorage.getItem('user_auth_token')) {
    localStorage.setItem('user_auth_token', 'mock-token')
  }
}

enableMocking().then(() => {
  createRoot(document.getElementById('root')!).render(
    <StrictMode>
      <App />
    </StrictMode>,
  )
})

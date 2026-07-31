import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import './index.css'
// Registers the live prefers-color-scheme listener as early as possible —
// index.html's inline script already applied the correct class before this
// module even runs, this is what keeps it in sync afterwards
// (documentation/09-ui-ux-design-system.md §2.10).
import './stores/themeStore'
import App from './App.tsx'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
)

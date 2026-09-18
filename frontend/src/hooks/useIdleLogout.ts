import { useEffect, useRef } from 'react'
import { useAuthStore } from '../stores/authStore'

/**
 * Client-side backstop for real inactivity. The backend's session idle
 * timeout (GUARDPIPE_REFRESH_TOKEN_TTL, 30m by default — internal/platform/
 * config/config.go) is sliding: it resets on *any* authenticated request,
 * including AppShell's own background polling (NotificationPanel's 20s
 * interval) — so a tab left open with no real user interaction never
 * actually hit it. This hook tracks genuine input events and force-logs-out
 * after this many ms of no real activity, independent of whatever
 * background requests kept the server-side session alive.
 */
const IDLE_TIMEOUT_MS = 30 * 60 * 1000
const CHECK_INTERVAL_MS = 30_000
const ACTIVITY_EVENTS = ['mousedown', 'mousemove', 'keydown', 'wheel', 'touchstart', 'scroll'] as const

export function useIdleLogout(): void {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated)
  const logout = useAuthStore((s) => s.logout)
  const lastActivityRef = useRef(0)

  useEffect(() => {
    if (!isAuthenticated) return

    lastActivityRef.current = Date.now()
    const markActive = () => {
      lastActivityRef.current = Date.now()
    }
    for (const event of ACTIVITY_EVENTS) {
      window.addEventListener(event, markActive, { passive: true })
    }

    const interval = window.setInterval(() => {
      if (Date.now() - lastActivityRef.current >= IDLE_TIMEOUT_MS) {
        void logout()
      }
    }, CHECK_INTERVAL_MS)

    return () => {
      for (const event of ACTIVITY_EVENTS) {
        window.removeEventListener(event, markActive)
      }
      window.clearInterval(interval)
    }
  }, [isAuthenticated, logout])
}

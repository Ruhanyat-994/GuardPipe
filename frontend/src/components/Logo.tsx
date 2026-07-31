import logoSrc from '../assets/logo.png'

/**
 * GuardPipe's mark — a shield (guard) containing a connected
 * run/checkpoint/stop pipeline graphic (pipe), user-supplied artwork with
 * its background removed (documentation/09-ui-ux-design-system.md §4.7).
 * Transparent PNG, `object-contain` so callers control size purely via
 * height and the aspect ratio never distorts. Paired with the "GuardPipe"
 * wordmark at every call site — this is the icon half of the lockup, not
 * a full logo-with-text asset.
 */
export function Logo({ className }: { className?: string }) {
  return <img src={logoSrc} alt="" className={className} />
}

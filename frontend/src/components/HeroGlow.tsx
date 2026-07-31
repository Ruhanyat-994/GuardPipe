/**
 * Animated hero background — direction adapted from tridentsecurity.io
 * (documentation/09-ui-ux-design-system.md §5.9, §2.9). The drift animation
 * is disabled globally under prefers-reduced-motion (see index.css) — this
 * component doesn't need its own media query for that.
 */
export function HeroGlow() {
  return (
    <div
      aria-hidden="true"
      className="absolute inset-0 overflow-hidden"
      style={{ backgroundColor: 'var(--public-bg-hero)' }}
    >
      <div className="hero-glow-blob" />
    </div>
  )
}

// Tiny spring physics for gesture-driven motion (no dependencies).
// CSS handles simple transitions (see --spring-* in styles.css); this is for
// things that must continue from a finger's velocity, like swiping photos.

export type SpringConfig = { stiffness: number; damping: number; mass?: number }

export const SNAPPY: SpringConfig = { stiffness: 520, damping: 38 }
export const BOUNCY: SpringConfig = { stiffness: 380, damping: 24 }

export const reducedMotion = () => window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false

/**
 * Animates from `from` to `to`, starting with `velocity` (units/second).
 * Calls onUpdate every frame; returns a function that stops the animation.
 */
export function animateSpring(
  from: number,
  to: number,
  velocity: number,
  cfg: SpringConfig,
  onUpdate: (v: number) => void,
  onDone?: () => void,
): () => void {
  if (reducedMotion()) {
    onUpdate(to)
    onDone?.()
    return () => {}
  }
  const mass = cfg.mass ?? 1
  let x = from
  let v = velocity
  let last = performance.now()
  let raf = 0
  let stopped = false

  const step = (now: number) => {
    if (stopped) return
    // Integrate in small fixed steps for stability, whatever the frame rate.
    let dt = Math.min((now - last) / 1000, 0.064)
    last = now
    while (dt > 0) {
      const h = Math.min(dt, 1 / 240)
      const a = (-cfg.stiffness * (x - to) - cfg.damping * v) / mass
      v += a * h
      x += v * h
      dt -= h
    }
    // "At rest" once within ~0.1% of the distance and moving slowly — for a
    // full-screen swipe that's under a pixel, so the eye can't tell.
    const scale = Math.max(1, Math.abs(to - from))
    if (Math.abs(x - to) < 0.001 * scale && Math.abs(v) < 0.05 * scale) {
      onUpdate(to)
      onDone?.()
      return
    }
    onUpdate(x)
    raf = requestAnimationFrame(step)
  }
  raf = requestAnimationFrame(step)
  return () => {
    stopped = true
    cancelAnimationFrame(raf)
  }
}

/** Tracks recent pointer positions to estimate release velocity (px/s). */
export class VelocityTracker {
  private samples: { t: number; x: number; y: number }[] = []
  add(x: number, y: number) {
    const t = performance.now()
    this.samples.push({ t, x, y })
    while (this.samples.length > 2 && t - this.samples[0].t > 100) this.samples.shift()
  }
  velocity(): { x: number; y: number } {
    const s = this.samples
    if (s.length < 2) return { x: 0, y: 0 }
    const a = s[0]
    const b = s[s.length - 1]
    const dt = (b.t - a.t) / 1000 || 1
    return { x: (b.x - a.x) / dt, y: (b.y - a.y) / dt }
  }
  reset() {
    this.samples = []
  }
}

/** iOS-style rubber band: resistance grows the further you pull. */
export function rubberBand(offset: number, dimension: number, constant = 0.55): number {
  const sign = Math.sign(offset)
  const abs = Math.abs(offset)
  return sign * (1 - 1 / ((abs * constant) / dimension + 1)) * dimension
}

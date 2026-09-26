export class ElapsedClock {
  private requestId = ''
  private anchorServerMs = 0
  private anchorLocalMs = 0
  private lastMs = 0

  reset(): void {
    this.requestId = ''
    this.anchorServerMs = 0
    this.anchorLocalMs = 0
    this.lastMs = 0
  }

  observe(requestId: string, serverMs: number, localMs: number): number {
    const previous = requestId === this.requestId ? this.tick(localMs) : null
    const current = previous === null ? serverMs : Math.max(previous, serverMs)
    this.requestId = requestId
    this.anchorServerMs = current
    this.anchorLocalMs = localMs
    this.lastMs = current
    return current
  }

  tick(localMs: number): number | null {
    if (!this.requestId) return null
    this.lastMs = Math.max(this.lastMs, this.anchorServerMs + Math.max(0, localMs - this.anchorLocalMs))
    return this.lastMs
  }
}

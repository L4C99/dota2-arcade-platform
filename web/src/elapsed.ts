import type { Allocation, ServerRequest } from './api'
import type { PlayerState } from './state'

export interface ElapsedInfo {
  summary: string
  detail?: string
}

function timestamp(value: string | undefined): number | null {
  if (!value) return null
  const parsed = Date.parse(value)
  return Number.isFinite(parsed) ? parsed : null
}

function liveSeconds(start: string | undefined, nowMs: number): number | null {
  const startMs = timestamp(start)
  return startMs === null ? null : Math.floor(Math.max(0, nowMs - startMs) / 1000)
}

export function elapsedFor(request: ServerRequest | null, allocation: Allocation | null, phase: PlayerState['phase'], nowMs: number): ElapsedInfo | null {
  if (!request) return null
  if (phase === 'waiting' || phase === 'allocating') {
    const seconds = liveSeconds(request.requestedAt, nowMs)
    if (seconds === null) return null
    return { summary: `${phase === 'waiting' ? '正在等待服务器' : '正在分配服务器'} · 已等待 ${seconds} 秒` }
  }
  if (phase === 'creating') {
    const started = allocation?.createStartedAt
    const seconds = liveSeconds(request.requestedAt, nowMs)
    if (seconds === null) return null
    const info: ElapsedInfo = { summary: `${started ? '正在启动服务器' : '正在准备服务器'} · 已用时 ${seconds} 秒` }
    if (started) {
      const startupSeconds = liveSeconds(started, nowMs)
      if (startupSeconds !== null) info.detail = `其中启动阶段 ${startupSeconds} 秒`
    }
    return info
  }
  if (phase !== 'ready' || !allocation?.joinInfo) return null
  const requestedMs = timestamp(request.requestedAt)
  const joinedMs = timestamp(allocation.joinInfoAvailableAt)
  if (requestedMs === null || joinedMs === null || joinedMs < requestedMs) return null
  const info: ElapsedInfo = { summary: `服务器已就绪 · 本次开服用时 ${((joinedMs - requestedMs) / 1000).toFixed(1)} 秒` }
  const assignedMs = timestamp(allocation.assignedAt)
  if (assignedMs !== null && assignedMs >= requestedMs + 5000 && assignedMs <= joinedMs) {
    info.detail = `排队等待 ${((assignedMs - requestedMs) / 1000).toFixed(1)} 秒 · 分配后至可进入 ${((joinedMs - assignedMs) / 1000).toFixed(1)} 秒`
  }
  return info
}

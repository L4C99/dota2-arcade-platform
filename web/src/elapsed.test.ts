import { describe, expect, it } from 'vitest'
import type { Allocation, ServerRequest } from './api'
import { elapsedFor } from './elapsed'

const request: ServerRequest = {
  id: 'request', arcadeGameId: 'game', gamePresetId: 'preset', state: 'running',
  requestedAt: '2026-09-26T10:00:00.000Z', updatedAt: '2026-09-26T10:00:00.000Z', nodeSelectionMode: 'auto',
}
const allocation: Allocation = {
  id: 'allocation', serverRequestId: 'request', attemptSequence: 1, nodeId: 'node', nodeDisplayName: 'node',
  contentVersionId: 'version', state: 'running', assignedAt: '2026-09-26T10:00:12.000Z',
  createStartedAt: '2026-09-26T10:00:15.000Z', readyAt: '2026-09-26T10:00:33.000Z',
  joinInfoAvailableAt: '2026-09-26T10:00:33.200Z',
  joinInfo: { connectCommand: 'connect 203.0.113.1:28000', connectHost: '203.0.113.1', publicPort: 28000 },
}

describe('player elapsed time', () => {
  it('uses the request and actual create-start timestamps for live states', () => {
    expect(elapsedFor(request, null, 'waiting', Date.parse('2026-09-26T10:00:12.300Z'))?.summary).toBe('正在等待服务器 · 已等待 12 秒')
    expect(elapsedFor(request, allocation, 'creating', Date.parse('2026-09-26T10:00:38.000Z'))?.summary).toBe('正在启动服务器 · 已用时 23 秒')
    expect(elapsedFor(request, { ...allocation, createStartedAt: undefined }, 'creating', Date.parse('2026-09-26T10:00:20.000Z'))?.summary).toBe('正在准备服务器 · 已用时 8 秒')
  })

  it('uses Platform JoinInfo time for the final total and separates a real queue', () => {
    expect(elapsedFor(request, allocation, 'ready', Date.parse('2026-09-26T11:00:00.000Z'))).toEqual({
      summary: '服务器已就绪 · 本次开服用时 33.2 秒',
      detail: '排队等待 12.0 秒 · 分配后至可进入 21.2 秒',
    })
    expect(elapsedFor(request, { ...allocation, joinInfo: undefined }, 'ready', Date.now())).toBeNull()
    expect(elapsedFor({ ...request, requestedAt: 'bad-time' }, allocation, 'ready', Date.now())).toBeNull()
  })
})

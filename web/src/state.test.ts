import { describe, expect, it } from 'vitest'
import type { Allocation, NextGameIntent, NodeChoice, ServerRequest } from './api'
import { statusFor } from './state'

const request = { id: 'one', arcadeGameId: 'game', gamePresetId: 'preset', state: 'running', requestedAt: '', updatedAt: '', nodeSelectionMode: 'auto' } satisfies ServerRequest
const allocation = { id: 'allocation', serverRequestId: 'one', attemptSequence: 1, nodeId: 'node', nodeDisplayName: 'node', contentVersionId: 'version', state: 'running', assignedAt: '', readyAt: 'time', joinInfoAvailableAt: 'time', joinInfo: { connectCommand: 'connect 203.0.113.1:28000', connectHost: '203.0.113.1', publicPort: 28000 } } satisfies Allocation

describe('player state', () => {
  it('requires both Ready and JoinInfo before showing can join', () => {
    expect(statusFor(request, allocation).phase).toBe('ready')
    expect(statusFor(request, { ...allocation, joinInfo: undefined, joinInfoErrorCode: 'PORT_MAPPING_UNAVAILABLE' }).phase).toBe('unavailable')
    expect(statusFor(request, { ...allocation, readyAt: undefined }).phase).toBe('creating')
  })
  it('keeps stopping distinct from ended', () => {
    expect(statusFor({ ...request, state: 'stopping' }, allocation).phase).toBe('stopping')
    expect(statusFor({ ...request, state: 'ended' }, allocation).phase).toBe('ended')
  })
  it('shows durable next-game progress and quarantine pause', () => {
    const intent: NextGameIntent = { sourceRequestId: request.id, state: 'pending', createdAt: '' }
    expect(statusFor({ ...request, state: 'stopping' }, allocation, [], intent).title).toContain('下一局已记录')
    const paused: NextGameIntent = { ...intent, state: 'paused' }
    expect(statusFor({ ...request, state: 'quarantined' }, { ...allocation, state: 'quarantined' }, [], paused).title).toContain('下一局暂停')
    expect(statusFor({ ...request, state: 'quarantined' }, { ...allocation, state: 'reclaimed' }, [], paused).description).toContain('完整回收')
  })
  it('explains manual waiting without promising a switch or an ETA', () => {
    const manual: ServerRequest = { ...request, state: 'waiting', nodeSelectionMode: 'manual', manualNodeId: 'node' }
    const node: NodeChoice = { id: 'node', displayName: '选定节点', status: 'full', connectivity: 'online', reason: 'full', availableSlots: 0, selectable: true }
    expect(statusFor(manual, null, [node]).description).toContain('等待所选节点容量')
    expect(statusFor(manual, null, [{ ...node, status: 'unavailable', reason: 'unreachable' }]).description).toContain('暂不可达')
    expect(statusFor({ ...manual, state: 'cancelled' }, null).phase).toBe('ended')
  })
})

import type { Allocation, Catalog, ServerRequest } from './api'

export interface PlayerState {
  title: string
  description: string
  phase: 'idle' | 'waiting' | 'allocating' | 'creating' | 'ready' | 'unavailable' | 'stopping' | 'ended' | 'error'
}

export function statusFor(request: ServerRequest | null, allocation: Allocation | null): PlayerState {
  if (!request) return { phase: 'idle', title: '准备好开一局？', description: '选择游廊地图和玩法，申请一台专属服务器。' }
  if (request.state === 'waiting') return { phase: 'waiting', title: '等待服务器', description: '申请已保存；有空位时会继续分配。刷新页面也能恢复。' }
  if (request.state === 'allocating' || (request.state === 'creating' && !allocation))
    return { phase: 'allocating', title: '正在分配', description: '正在确认节点内容和可用容量。' }
  if (request.state === 'creating') return { phase: 'creating', title: '正在启动', description: '专服正在启动 Dota 2，请稍候。' }
  if (request.state === 'running') {
    if (allocation?.joinInfo && allocation.joinInfoAvailableAt && allocation.readyAt)
      return { phase: 'ready', title: '可以进入', description: '复制下方命令，在 Dota 2 控制台连接服务器。' }
    if (allocation?.joinInfoErrorCode)
      return { phase: 'unavailable', title: '连接信息不可用', description: '服务器仍在运行并占用容量。请稍后刷新，或结束这台服务器。' }
    return { phase: 'creating', title: '正在启动', description: '已收到服务器状态，正在确认连接信息。' }
  }
  if (request.state === 'stopping') return { phase: 'stopping', title: '正在结束', description: '正在等待专服完整回收；容量会在确认后释放。' }
  if (request.state === 'ended' || request.state === 'cancelled')
    return { phase: 'ended', title: '已结束', description: '服务器已完整回收，可以重新申请。' }
  return { phase: 'error', title: '服务器状态需要处理', description: '当前资源仍可能占用容量。请保留此页面并联系管理员，勿重复申请。' }
}

export function maintenanceFor(catalog: Catalog, gameId: string, presetId: string): string {
  if (!catalog.globalAcceptingNewRequests) return catalog.globalMaintenanceMessage || '平台暂不接受新申请。'
  const game = catalog.games.find((item) => item.id === gameId)
  if (!game || !game.enabled || !game.acceptingNewRequests) return game?.maintenanceMessage || '这张地图正在维护。'
  const preset = catalog.presets.find((item) => item.id === presetId && item.arcadeGameId === gameId)
  if (!preset || !preset.enabled || !preset.acceptingNewRequests) return preset?.maintenanceMessage || '这个玩法正在维护。'
  return ''
}

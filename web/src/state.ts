import type { Allocation, Catalog, NextGameIntent, NodeChoice, ServerRequest } from './api'

export interface PlayerState {
  title: string
  description: string
  phase: 'idle' | 'waiting' | 'allocating' | 'creating' | 'ready' | 'unavailable' | 'stopping' | 'unknown' | 'quarantined' | 'ended' | 'error'
}

export function statusFor(request: ServerRequest | null, allocation: Allocation | null, nodes: NodeChoice[] = [], nextGame: NextGameIntent | null = null): PlayerState {
  if (!request) return { phase: 'idle', title: '准备好开一局？', description: '选择游廊地图和玩法，申请一台专属服务器。' }
  if (request.state === 'abandoned')
    return { phase: 'ended', title: '已放弃异常服务器', description: '业务申请已解除；旧服务器状态仍待管理员确认，原节点容量没有释放。你可以创建新的申请。' }
  if (request.state === 'quarantined' && nextGame?.state === 'paused')
    return allocation?.state === 'reclaimed'
      ? { phase: 'quarantined', title: '下一局等待确认', description: '旧服务器已完整回收。下一局仍暂停，等待队长确认继续。' }
      : { phase: 'quarantined', title: '下一局暂停，等待异常处理', description: '旧服务器状态无法确认，原节点容量仍占用。队长放弃此异常服务器后，才会创建下一局的新申请。' }
  if (request.state === 'quarantined' || allocation?.state === 'quarantined')
    return { phase: 'quarantined', title: '服务器清理异常', description: '旧服务器是否已关闭暂时无法确认，原节点容量仍被占用。队长可以放弃此异常服务器并继续申请；管理员会处理旧资源。' }
  const assignedNode = nodes.find((item) => item.id === allocation?.nodeId)
  if (allocation && assignedNode && assignedNode.connectivity !== 'online' &&
    ['reserved', 'create_unknown', 'creating', 'running', 'stopping', 'failed_unreclaimed'].includes(allocation.state))
    return { phase: 'unknown', title: '节点暂不可达', description: '当前服务器状态无法确认，旧资源仍占用容量。平台不会在其他节点重复开服；节点恢复后会先对账。' }
  if (allocation?.state === 'create_unknown')
    return { phase: 'unknown', title: '状态暂时无法确认', description: '开服响应丢失，平台正在与原节点对账。当前资源仍占用容量，不会重复创建服务器。' }
  if (request.state === 'waiting') {
    if (request.nodeSelectionMode === 'manual') {
      const node = nodes.find((item) => item.id === request.manualNodeId)
      const reason = node?.reason
      const description = reason === 'full' ? '等待所选节点容量；不会自动切换到其他节点。'
        : reason === 'maintenance' ? '所选节点维护中；恢复后会继续等待该节点。'
          : reason === 'unreachable' ? '所选节点暂不可达；恢复后会继续等待该节点。'
            : reason === 'content_unready' || reason === 'template_unready' ? '所选节点内容尚未就绪；不会自动切换到其他节点。'
              : reason === 'incompatible' ? '所选节点暂不可用；恢复兼容后会继续等待。'
                : '等待所选节点分配；刷新页面也能恢复申请。'
      return { phase: 'waiting', title: '等待所选节点', description }
    }
    return { phase: 'waiting', title: '等待服务器', description: nodes.length > 0 && nodes.every((item) => item.reason === 'full')
      ? '等待可用服务器容量；申请时间已保留。' : '等待可用服务器；申请时间已保留。刷新页面也能恢复。' }
  }
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
  if (request.state === 'stopping') return nextGame?.state === 'pending'
    ? { phase: 'stopping', title: '正在结束，下一局已记录', description: '先等待旧服务器完整回收，再创建新的申请并按正常顺序排队。' }
    : { phase: 'stopping', title: '正在结束', description: '正在等待专服完整回收；容量会在确认后释放。' }
  if (request.state === 'failed_unreclaimed') return { phase: 'unknown', title: '正在处理启动异常', description: '服务器可能仍有底层资源。平台正在安全停止并回收，确认前不会释放容量。' }
  if (request.state === 'cancelled')
    return { phase: 'ended', title: '申请已取消', description: '等待申请已安全取消，没有占用服务器。可以重新选择节点申请。' }
  if (request.state === 'ended')
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

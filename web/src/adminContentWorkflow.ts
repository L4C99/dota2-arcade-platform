export interface WorkflowGame { id: string; displayName: string; workshopId: string; currentContentVersionId: string; maintenanceMessage?: string; enabled: boolean; acceptingNewRequests: boolean }
export interface WorkflowVersion { id: string; arcadeGameId: string; contentSha256: string; createdAt?: string }
export interface WorkflowPreset { id: string; arcadeGameId: string; displayName: string; templateRevisionId: string; maintenanceMessage?: string; maxPlayers: number; enabled: boolean; acceptingNewRequests: boolean }
export interface WorkflowNode { id: string; displayName: string; connectivity: string; compatibility: string; enabled: boolean; acceptingNewRequests: boolean; draining: boolean; occupied: number }
export interface WorkflowBinding { nodeId: string; arcadeGameId: string; reportedContentVersionId: string; reportedContentSha256: string; reportedState: string; acceptingNewAllocations: boolean }
export interface WorkflowValidation { nodeId: string; arcadeGameId: string; contentVersionId: string; verifiedAt: string }
export interface WorkflowTemplateBinding { nodeId: string; templateRevisionId: string; bindingKey: string }
export interface WorkflowTemplateRevision { id: string; arcadeGameId: string; description: string }
export interface WorkflowOverview {
  games: WorkflowGame[]
  contentVersions: WorkflowVersion[]
  presets: WorkflowPreset[]
  nodes: WorkflowNode[]
  bindings: WorkflowBinding[]
  contentValidations: WorkflowValidation[]
  templateBindings: WorkflowTemplateBinding[]
  templateRevisions: WorkflowTemplateRevision[]
}
export type WorkflowMode = 'new' | 'update' | 'manage'

export interface NodeContentProgress {
  binding?: WorkflowBinding
  reportedVersion: boolean
  digestMatches: boolean
  confirmed: boolean
  validation?: WorkflowValidation
  mappedTemplates: number
  requiredTemplates: number
  canValidate: boolean
  readyToPublish: boolean
  published: boolean
  admissionOpen: boolean
  complete: boolean
  next: string
}

export function deriveNodeContentProgress(data: WorkflowOverview, node: WorkflowNode, game: WorkflowGame, version: WorkflowVersion, mode: WorkflowMode): NodeContentProgress {
  const binding = data.bindings.find(item => item.nodeId === node.id && item.arcadeGameId === game.id)
  const reportedVersion = !!binding && binding.reportedContentVersionId === version.id
  const digestMatches = reportedVersion && binding.reportedContentSha256 === version.contentSha256
  const confirmed = digestMatches && binding?.reportedState === 'confirmed'
  const validation = data.contentValidations.find(item => item.nodeId === node.id && item.arcadeGameId === game.id && item.contentVersionId === version.id)
  const revisions = [...new Set(data.presets.filter(item => item.arcadeGameId === game.id).map(item => item.templateRevisionId))]
  const mappedTemplates = revisions.filter(revisionId => data.templateBindings.some(item => item.nodeId === node.id && item.templateRevisionId === revisionId)).length
  const requiredTemplates = mode === 'new' ? revisions.length : 0
  const compatible = node.enabled && node.connectivity === 'online' && node.compatibility === 'compatible'
  const canValidate = confirmed && compatible && node.draining && node.occupied === 0 && !validation
  const readyToPublish = confirmed && compatible && node.acceptingNewRequests && !node.draining && !!validation && (mode !== 'new' || mappedTemplates === requiredTemplates)
  const published = game.currentContentVersionId === version.id
  const admissionOpen = !!binding?.acceptingNewAllocations
  const openPresets = data.presets.filter(item => item.arcadeGameId === game.id).every(item => item.enabled && item.acceptingNewRequests)
  let next: string
  if (!binding) next = '先在节点配置此 Workshop 的 Controller 内容绑定，准备 VPK 后刷新状态。'
  else if (mode === 'update' && admissionOpen && !reportedVersion) next = '先暂停此节点这张图的新分配，等待旧实例完整回收。'
  else if (!reportedVersion && node.occupied > 0) next = '等待节点上的旧实例结束，并在 d2core 确认完整回收。'
  else if (!reportedVersion) next = '在节点执行 Content Tool prepare / switch / status，随后请求 Controller 核对并刷新状态。'
  else if (!digestMatches) next = '上报版本的 SHA256 不匹配；核对节点 VPK 与平台登记值，暂勿验收或发布。'
  else if (!confirmed) next = '版本与 SHA256 已匹配；请求 Controller 核对，等待状态变为已确认。'
  else if (requiredTemplates > mappedTemplates) next = '先在节点准备正式模板和 Controller 绑定键，再在网页补齐玩法模板映射。'
  else if (!compatible) next = '节点启用状态、心跳或组件兼容性未满足；恢复节点后再继续验收。'
  else if (!validation && !node.draining) next = '进入节点维护，确认占用为零；再逐玩法做本地临时实例真人测试。'
  else if (!validation && node.occupied > 0) next = '节点仍有占用；等待完整回收后才能确认内容版本验收。'
  else if (!validation) next = '在节点按正式模板逐玩法真人测试，停止并完整回收临时实例、Controller 核对后确认内容版本验收。'
  else if (node.draining) next = '内容版本验收已记录；先结束节点维护，再发布或恢复新分配。'
  else if (!node.acceptingNewRequests) next = '节点本身暂停新申请；先在节点调度设置恢复接单，再发布或开放此图。'
  else if (!published) next = '内容版本验收已完成；现在可以发布为新开服版本。'
  else if (!admissionOpen) next = '平台已发布；打开此节点这张图的新分配。'
  else if (!game.enabled || !game.acceptingNewRequests) next = '节点已准备好；启用地图并开放新申请。'
  else if (mode === 'new' && !openPresets) next = '启用经过真人测试的玩法并开放新申请，然后从玩家页面做一次完整开服与回收。'
  else next = '此节点已使用当前发布版本并允许新分配；仍需以玩家页面确认完整开服与回收。'
  const complete = published && admissionOpen && confirmed && !!validation && compatible && node.acceptingNewRequests && !node.draining && (mode !== 'new' || (mappedTemplates === requiredTemplates && game.enabled && game.acceptingNewRequests && openPresets))
  return { binding, reportedVersion, digestMatches, confirmed, validation, mappedTemplates, requiredTemplates, canValidate, readyToPublish, published, admissionOpen, complete, next }
}

export function canPublishContent(data: WorkflowOverview, game: WorkflowGame, version: WorkflowVersion, mode: WorkflowMode, selectedNodeIds?: string[]): boolean {
  if (game.currentContentVersionId === version.id) return false
  return data.nodes.some(node => (!selectedNodeIds || selectedNodeIds.includes(node.id)) && deriveNodeContentProgress(data, node, game, version, mode).readyToPublish)
}

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'

interface Settings { acceptingNewRequests: boolean; maintenanceMessage: string; siteAnnouncement: string }
interface Game { id: string; displayName: string; workshopId: string; currentContentVersionId: string; maintenanceMessage: string; enabled: boolean; acceptingNewRequests: boolean }
interface Preset { id: string; arcadeGameId: string; displayName: string; templateRevisionId: string; maintenanceMessage: string; enabled: boolean; acceptingNewRequests: boolean; maxPlayers: number }
interface ContentVersion { id: string; arcadeGameId: string; contentSha256: string; createdAt: string }
interface TemplateRevision { id: string; arcadeGameId: string; description: string }
interface TemplateBinding { nodeId: string; templateRevisionId: string; bindingKey: string }
interface ContentValidation { nodeId: string; arcadeGameId: string; contentVersionId: string; verifiedAt: string; verifiedBy: string }
interface Node { id: string; displayName: string; os: string; connectivity: string; controllerVersion: string; d2coreVersion: string; d2coreCommit: string; compatibility: string; recentErrorCode: string; enabled: boolean; acceptingNewRequests: boolean; draining: boolean; priority: number; hard: number; desired: number; occupied: number; lastHeartbeat?: string; reconcileRequested: number; reconcileCompleted: number }
interface Binding { nodeId: string; arcadeGameId: string; reportedContentVersionId: string; reportedContentSha256: string; reportedState: string; reportedAt?: string; acceptingNewAllocations: boolean }
interface Entry { nodeId: string; entryConfigRevision: string; publicPorts: number[]; steamVerifiedPorts: number[]; steamChinaVerifiedPorts: number[]; steamVerificationNote: string; steamChinaVerificationNote: string; a2sEnabled: boolean; a2sQueryOk: boolean; steamVerified: boolean; steamEnabled: boolean; steamChinaVerified: boolean; steamChinaEnabled: boolean; steamVerifiedAt?: string; steamChinaVerifiedAt?: string }
interface ServerRequest { id: string; arcadeGameId: string; gamePresetId: string; state: string; ownerPartyId?: string; nodeSelectionMode: string; manualNodeId?: string; requestedAt: string }
interface Party { id: string; leaderDisplayName: string; memberCount: number; dissolvedAt?: string }
interface Allocation { id: string; serverRequestId: string; nodeId: string; contentVersionId: string; templateRevisionId: string; state: string; errorCode: string; attemptSequence: number; assignedAt: string }
interface Job { id: string; nodeId: string; allocationId: string; kind: string; state: string; errorCode: string; updatedAt: string }
interface Audit { id: string; actorUsername: string; actorKind: string; action: string; targetType: string; targetId: string; result: string; stateChange: Record<string, unknown>; createdAt: string }
interface Counts { waiting: number; creating: number; running: number; stopping: number; quarantined: number; failedUnreclaimed: number }
interface Overview { settings: Settings; counts: Counts; games: Game[]; presets: Preset[]; contentVersions: ContentVersion[]; templateRevisions: TemplateRevision[]; templateBindings: TemplateBinding[]; contentValidations: ContentValidation[]; nodes: Node[]; bindings: Binding[]; entries: Entry[]; requests: ServerRequest[]; parties: Party[]; allocations: Allocation[]; jobs: Job[]; audit: Audit[] }
interface Action { action: string; targetId?: string; gameId?: string; accepting?: boolean; enabled?: boolean; draining?: boolean; priority?: number; desired?: number; message?: string; entry?: string; verified?: boolean; confirmed?: boolean; verifiedPorts?: number[]; verificationNote?: string; workshopId?: string; displayName?: string; contentVersionId?: string; contentSha256?: string; templateRevisionId?: string; bindingKey?: string; description?: string; maxPlayers?: number }

const username = ref('')
const password = ref('')
const authenticated = ref(false)
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const overview = ref<Overview | null>(null)
const tab = ref<'overview' | 'content' | 'nodes' | 'instances' | 'audit'>('overview')
const globalDraft = reactive({ accepting: true, maintenance: '', announcement: '' })
const nodeDraft = reactive<Record<string, { priority: number; desired: number }>>({})
const selectedNodeId = ref('')
const selectedRequestId = ref('')
const requestView = ref<'active' | 'history'>('active')
const selectedAuditId = ref('')
const catalogDraft = reactive({ workshopId: '', gameName: '', gameId: '', versionId: '', sha256: '', templateId: '', templateDescription: '', presetName: '', presetMaxPlayers: 1, presetTemplateId: '', publishVersionId: '' })
const templateBindingDraft = reactive({ revisionId: '', bindingKey: '' })

const requestStates: Record<string, string> = { waiting: '排队中', allocating: '分配中', creating: '启动中', running: '运行中', stopping: '停止中', quarantined: '异常隔离', failed_unreclaimed: '回收异常', unavailable: '无法继续分配', ended: '已结束', reclaimed: '已回收', abandoned: '已放弃', cancelled: '已取消' }
const activeRequestStates = ['waiting', 'allocating', 'creating', 'running', 'stopping', 'quarantined', 'failed_unreclaimed']
const allocationStates: Record<string, string> = { reserved: '资源已预留', creating: '启动中', create_unknown: '启动结果待确认', running: '运行中', stopping: '停止中', unknown: '状态待确认', failed_unreclaimed: '回收异常', quarantined: '异常隔离', reclaimed: '已回收', released_no_effect: '未启动，名额已释放' }
const jobStates: Record<string, string> = { pending: '待执行', claimed: '节点已领取', accepted: '已接收', running: '执行中', unknown: '状态待确认', succeeded: '已完成', failed: '失败' }
const auditActions: Record<string, string> = { 'global.update': '调整全站申请设置', 'announcement.update': '更新站点公告', 'game.create': '登记游廊游戏', 'game.update': '调整游廊游戏', 'template.create': '登记模板修订', 'content.create': '登记内容版本', 'preset.create': '登记玩法预设', 'template_binding.upsert': '设置节点模板映射', 'content.validate': '记录内容真人验证', 'content.publish': '发布内容版本', 'preset.update': '调整玩法预设', 'node.update': '调整节点设置', 'node.reconcile': '请求节点重新核对', 'binding.update': '调整地图分配', 'entry.update': '调整一键入口', 'request.cancel': '取消等待申请', 'request.stop': '请求结束服务器', 'allocation.quarantine': '标记异常隔离', 'allocation.quarantine.unreachable': '失联后自动隔离', 'admin.create': '创建管理员', 'admin.reset_password': '重设管理员密码', 'admin.disable': '停用管理员' }
const auditFields: Record<string, string> = { acceptingBefore: '原申请状态', acceptingAfter: '新申请状态', enabledBefore: '原启用状态', enabledAfter: '新启用状态', drainingBefore: '原维护状态', drainingAfter: '新维护状态', priorityBefore: '原优先级', priorityAfter: '新优先级', desiredBefore: '原期望容量', desiredAfter: '新期望容量', verifiedBefore: '原验证状态', verifiedAfter: '新验证状态', requestBefore: '原申请状态', requestAfter: '新申请状态', before: '原状态', after: '新状态', messageBefore: '原提示', messageAfter: '新提示', capacityReleased: '释放容量', sessionsInvalidated: '旧会话失效', requestedGeneration: '核对批次', revision: '配置版本', username: '用户名', entry: '入口', jobId: '节点任务', stopJobId: '停止任务编号', allocationId: '资源分配编号' }
function stateLabel(state: string): string { return requestStates[state] || allocationStates[state] || jobStates[state] || state || '未知' }
function connectivityLabel(value: string): string { return ({ online: '在线', stale: '心跳延迟', offline: '离线' } as Record<string, string>)[value] || '状态未知' }
function compatibilityLabel(value: string): string { return ({ compatible: '兼容', incompatible: '不兼容' } as Record<string, string>)[value] || '兼容性待确认' }
function reportedStateLabel(value: string): string { return ({ confirmed: '已确认', unknown: '待确认', missing: '未准备', error: '异常' } as Record<string, string>)[value] || value || '待确认' }
function jobKindLabel(value: string): string { return ({ create: '启动服务器', stop: '停止服务器' } as Record<string, string>)[value] || value }
function selectionLabel(value: string): string { return value === 'manual' ? '指定节点' : '自动分配' }
function nodeAdmissionLabel(node: Node): string {
  if (!node.enabled) return '节点已停用'
  if (node.connectivity !== 'online') return `${connectivityLabel(node.connectivity)}，暂停新分配`
  if (node.compatibility !== 'compatible') return '组件待确认，暂停新分配'
  if (node.draining) return '维护中，暂停新分配'
  if (!node.acceptingNewRequests) return '暂停新分配'
  if (node.occupied >= node.desired) return '当前已满'
  return '可接收新分配'
}
function auditValue(value: unknown): string {
  if (typeof value === 'boolean') return value ? '是' : '否'
  if (typeof value === 'string') return stateLabel(value)
  return String(value)
}
function auditTarget(event: Audit): string {
  const names: Record<string, string> = { platform_settings: '全站设置', site_announcement: '站点公告', arcade_game: '游廊游戏', game_preset: '玩法预设', node: '游戏节点', node_content_binding: '节点地图', node_entry: '一键入口', server_request: '开服申请', allocation: '资源分配', admin_user: '管理员' }
  const label = names[event.targetType] || event.targetType
  if (event.targetId === 'global') return label
  const id = event.targetId.slice(0, 8)
  return `${label} · ${id}`
}
const selectedNode = computed(() => overview.value?.nodes.find(node => node.id === selectedNodeId.value) || overview.value?.nodes[0])
const filteredRequests = computed(() => overview.value?.requests.filter(request => requestView.value === 'active'
  ? activeRequestStates.includes(request.state)
  : !activeRequestStates.includes(request.state)) || [])
const activeRequestCount = computed(() => overview.value?.requests.filter(request => activeRequestStates.includes(request.state)).length || 0)
const selectedRequest = computed(() => filteredRequests.value.find(request => request.id === selectedRequestId.value) || filteredRequests.value[0])
const selectedAudit = computed(() => overview.value?.audit.find(event => event.id === selectedAuditId.value) || overview.value?.audit[0])
function bindingsFor(nodeId: string): Binding[] { return overview.value?.bindings.filter(binding => binding.nodeId === nodeId) || [] }
function entriesFor(nodeId: string): Entry[] { return overview.value?.entries.filter(entry => entry.nodeId === nodeId) || [] }
function versionsFor(gameId: string): ContentVersion[] { return overview.value?.contentVersions.filter(version => version.arcadeGameId === gameId) || [] }
function validationFor(nodeId: string, gameId: string, versionId: string): ContentValidation | undefined { return overview.value?.contentValidations.find(v => v.nodeId === nodeId && v.arcadeGameId === gameId && v.contentVersionId === versionId) }

async function api<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  const response = await fetch(`/api/v1/admin${path}`, { method, credentials: 'same-origin',
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body) })
  if (!response.ok) {
    if (response.status === 401) { authenticated.value = false; overview.value = null }
    throw new Error(response.status === 401 ? '管理员会话已失效，请重新登录。' :
      response.status === 403 ? '没有执行此操作的权限。' :
        response.status === 409 ? '当前状态不允许此操作，请刷新后再试。' :
          response.status === 429 ? '登录尝试过多，请稍后重试。' : '请求失败，请检查服务状态。')
  }
  return response.status === 204 ? undefined as T : response.json() as Promise<T>
}

function stamp(value?: string): string { return value ? new Date(value).toLocaleString('zh-CN') : '未上报' }
function gameName(id: string): string { return overview.value?.games.find(g => g.id === id)?.displayName || id.slice(0, 8) }
function nodeName(id: string): string { return overview.value?.nodes.find(n => n.id === id)?.displayName || id.slice(0, 8) }
const counts = computed(() => overview.value?.counts || { waiting: 0, creating: 0, running: 0, stopping: 0, quarantined: 0, failedUnreclaimed: 0 })
const activeParties = computed(() => overview.value?.parties.filter(party => !party.dissolvedAt) || [])
const dissolvedParties = computed(() => overview.value?.parties.filter(party => !!party.dissolvedAt) || [])

async function refresh(): Promise<void> {
  const data = await api<Overview>('/overview')
  overview.value = data
  Object.assign(globalDraft, { accepting: data.settings.acceptingNewRequests,
    maintenance: data.settings.maintenanceMessage, announcement: data.settings.siteAnnouncement })
  for (const node of data.nodes) nodeDraft[node.id] = { priority: node.priority, desired: node.desired }
  if (!data.nodes.some(node => node.id === selectedNodeId.value)) selectedNodeId.value = data.nodes[0]?.id || ''
  if (!data.games.some(game => game.id === catalogDraft.gameId)) catalogDraft.gameId = data.games[0]?.id || ''
}

onMounted(async () => {
  try { await api('/me'); authenticated.value = true; await refresh() }
  catch (cause) { if (authenticated.value) error.value = String(cause) }
  finally { loading.value = false }
})

async function login(): Promise<void> {
  if (busy.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try { await api('/login', 'POST', { username: username.value, password: password.value });
    password.value = ''; authenticated.value = true; await refresh() }
  catch (cause) { password.value = ''; error.value = String(cause) }
  finally { busy.value = false }
}

async function logout(): Promise<void> {
  try { await api('/logout', 'POST') } catch { /* local state still clears */ }
  authenticated.value = false; overview.value = null; password.value = ''
}

async function act(action: Action, confirmation = ''): Promise<void> {
  if (busy.value || confirmation && !window.confirm(confirmation)) return
  busy.value = true; error.value = ''; notice.value = ''
  try { await api('/actions', 'POST', action); await refresh(); notice.value = '更改已保存并记录审计。' }
  catch (cause) { error.value = String(cause) }
  finally { busy.value = false }
}

async function saveGlobal(): Promise<void> {
  await act({ action: 'global.update', accepting: globalDraft.accepting, message: globalDraft.maintenance })
}
async function saveAnnouncement(): Promise<void> { await act({ action: 'announcement.update', message: globalDraft.announcement }) }
async function createGame(): Promise<void> {
  await act({ action: 'game.create', workshopId: catalogDraft.workshopId, displayName: catalogDraft.gameName })
}
async function createContentVersion(): Promise<void> {
  await act({ action: 'content.create', targetId: catalogDraft.gameId, contentVersionId: catalogDraft.versionId, contentSha256: catalogDraft.sha256 })
}
async function createTemplateRevision(): Promise<void> {
  await act({ action: 'template.create', targetId: catalogDraft.gameId, templateRevisionId: catalogDraft.templateId, description: catalogDraft.templateDescription })
}
async function createPreset(): Promise<void> {
  await act({ action: 'preset.create', targetId: catalogDraft.gameId, displayName: catalogDraft.presetName, maxPlayers: catalogDraft.presetMaxPlayers, templateRevisionId: catalogDraft.presetTemplateId })
}
async function publishVersion(): Promise<void> {
  await act({ action: 'content.publish', targetId: catalogDraft.gameId, contentVersionId: catalogDraft.publishVersionId, confirmed: true }, '确认目标版本已在至少一台已恢复、兼容的节点完成准备、真人验证、完整回收和 Controller 核对？发布只更新新 Allocation 的目标版本，不切换节点文件，也不改变已有 Allocation；随后可开放该节点的地图分配。')
}
function editMessage(kind: 'game.update' | 'preset.update', id: string, current: string): void {
  const message = window.prompt(kind === 'game.update' ? '地图维护提示' : '玩法维护提示', current)
  if (message !== null) void act({ action: kind, targetId: id, message })
}

function entryAction(node: Node, entry: Entry, kind: 'steam' | 'steamchina', field: 'verified' | 'enabled', value: boolean): void {
  const base: Action = { action: 'entry.update', targetId: node.id, entry: kind }
  if (field === 'verified') {
    base.verified = value
    base.confirmed = value
    if (value) {
      const ports = window.prompt(`请填写已用真实客户端逐一成功进入的全部公网端口，逗号分隔。当前配置要求：${entry.publicPorts.join(', ')}`, '')
      if (ports === null) return
      base.verifiedPorts = ports.split(',').map(item => Number(item.trim()))
      const note = window.prompt('请记录真人验证结果与测试窗口（不要填写凭据或私人玩家信息）', '')
      if (note === null) return
      base.verificationNote = note.trim()
    }
    void act(base, value ? '确认你已经在当前节点、当前网络配置、所有可能分配的公网端口映射上，用真实客户端验证此入口可以进入游戏？此操作会记录你的管理员身份。' : '撤销此入口的真人验证并关闭入口？')
  } else { base.enabled = value; void act(base, value ? '确认开启此入口给玩家？请先核对真人验证状态。' : '') }
}
</script>

<template>
  <header class="site-header"><div class="header-inner"><div class="brand"><span class="brand-mark" aria-hidden="true"><i/><i/><i/><i/></span><span>Dota 2 <strong>游廊联机</strong></span></div><div class="admin-header-tools"><span class="identity">平台管理</span><a href="/">玩家页面</a><button v-if="authenticated" type="button" class="admin-text-button" @click="logout">退出</button></div></div></header>
  <main class="admin-shell">
    <div v-if="error" class="notice notice-error" role="alert">{{ error }}</div>
    <div v-if="notice" class="notice notice-announcement" role="status">{{ notice }}</div>
    <section v-if="loading" class="panel admin-login" role="status">正在检查管理员会话…</section>
    <section v-else-if="!authenticated" class="panel admin-login">
      <span class="eyebrow">独立管理员会话</span><h1>登录管理后台</h1><p>用于维护申请、节点和异常服务器。登录后操作会记录审计。</p>
      <form @submit.prevent="login"><label>用户名<input v-model.trim="username" required autocomplete="username" maxlength="128" /></label><label>密码<input v-model="password" required type="password" autocomplete="current-password" /></label><button type="submit" class="primary-button" :disabled="busy">{{ busy ? '正在登录…' : '登录' }}</button></form>
    </section>
    <template v-else-if="overview">
      <div class="admin-heading"><div><span class="eyebrow">运维控制台</span><h1>管理后台</h1><p>查看平台运行情况，处理申请和异常。</p></div><button type="button" class="secondary-button" :disabled="busy" @click="refresh">刷新状态</button></div>
      <nav class="admin-tabs" aria-label="管理页面"><button v-for="item in ([['overview','总览'],['content','内容与维护'],['nodes','节点'],['instances','申请与实例'],['audit','审计']] as const)" :key="item[0]" type="button" :class="{ selected: tab === item[0] }" @click="tab = item[0]">{{ item[1] }}</button></nav>
      <template v-if="tab === 'overview'">
        <div class="admin-metrics"><div v-for="(value, label) in counts" :key="label" class="panel admin-metric"><span>{{ { waiting:'排队', creating:'创建中', running:'运行中', stopping:'停止中', quarantined:'隔离中', failedUnreclaimed:'待回收异常' }[label] }}</span><strong>{{ value }}</strong></div></div>
        <div class="admin-columns"><section class="panel admin-card"><span class="eyebrow">全局维护</span><h2>{{ overview.settings.acceptingNewRequests ? '正在接受新申请' : '已暂停新申请' }}</h2><label class="admin-check"><input v-model="globalDraft.accepting" type="checkbox" /> 接受新申请</label><label>维护提示<textarea v-model="globalDraft.maintenance" maxlength="1000" rows="3" /></label><button type="button" class="primary-button" :disabled="busy" @click="saveGlobal">保存全局状态</button></section>
          <section class="panel admin-card"><span class="eyebrow">站点公告</span><h2>玩家可见消息</h2><p>公告独立于是否暂停新申请。</p><label>公告内容<textarea v-model="globalDraft.announcement" maxlength="1000" rows="4" placeholder="留空可撤下公告" /></label><button type="button" class="secondary-button" :disabled="busy" @click="saveAnnouncement">保存公告</button></section></div>
        <section class="panel admin-card"><span class="eyebrow">节点摘要</span><h2>容量与连接</h2><div class="admin-list"><div v-for="node in overview.nodes" :key="node.id" class="admin-row"><div><strong>{{ node.displayName }}</strong><small>{{ node.os === 'windows' ? 'Windows' : 'Linux' }} · {{ compatibilityLabel(node.compatibility) }} · 最近心跳 {{ stamp(node.lastHeartbeat) }}</small></div><div class="admin-row-side"><span class="admin-badge" :class="node.connectivity">{{ connectivityLabel(node.connectivity) }}</span><span class="admin-capacity"><strong>{{ node.occupied }} / {{ node.desired }}</strong><small>已占用 / 期望容量</small></span></div></div></div></section>
      </template>
      <template v-else-if="tab === 'content'">
        <div class="admin-section-heading"><div><span class="eyebrow">内容与维护</span><h2>地图、玩法与版本</h2><p>平台登记和发布内容版本；节点磁盘由运维使用 Content Tool 单独准备和切换。</p></div></div>
        <section class="panel admin-card"><span class="eyebrow">新增内容</span><h2>登记游戏与玩法</h2><p>新游戏和玩法默认停用且暂停申请。内容文件不会从这里上传或修改。</p>
          <form class="admin-form-row" @submit.prevent="createGame"><label>Workshop ID<input v-model.trim="catalogDraft.workshopId" required pattern="[0-9]+" maxlength="32" /></label><label>游戏名称<input v-model.trim="catalogDraft.gameName" required maxlength="128" /></label><button class="secondary-button" type="submit" :disabled="busy">登记游戏</button></form>
          <div v-if="overview.games.length" class="admin-catalog-create"><label>目标游戏<select v-model="catalogDraft.gameId"><option v-for="game in overview.games" :key="game.id" :value="game.id">{{ game.displayName }} · {{ game.workshopId }}</option></select></label>
            <form class="admin-form-row" @submit.prevent="createContentVersion"><label>新内容版本 ID<input v-model.trim="catalogDraft.versionId" required maxlength="128" /></label><label>VPK SHA256<input v-model.trim="catalogDraft.sha256" required pattern="[0-9a-f]{64}" maxlength="64" /></label><button class="secondary-button" type="submit" :disabled="busy">登记不可变版本</button></form>
            <form class="admin-form-row" @submit.prevent="createTemplateRevision"><label>模板修订 ID<input v-model.trim="catalogDraft.templateId" required maxlength="128" /></label><label>启动语义说明<input v-model.trim="catalogDraft.templateDescription" maxlength="1000" /></label><button class="secondary-button" type="submit" :disabled="busy">登记模板修订</button></form>
            <form class="admin-form-row" @submit.prevent="createPreset"><label>玩法名称<input v-model.trim="catalogDraft.presetName" required maxlength="128" /></label><label>最大玩家数<input v-model.number="catalogDraft.presetMaxPlayers" required type="number" min="1" step="1" /></label><label>模板修订<select v-model="catalogDraft.presetTemplateId" required><option value="" disabled>请选择</option><option v-for="revision in overview.templateRevisions.filter(item => item.arcadeGameId === catalogDraft.gameId)" :key="revision.id" :value="revision.id">{{ revision.id }}</option></select></label><button class="secondary-button" type="submit" :disabled="busy">登记玩法</button></form>
          </div>
        </section>
        <section v-if="catalogDraft.gameId" class="panel admin-card"><span class="eyebrow">正式发布</span><h2>切换新分配的内容目标</h2><p>必须先在节点完成排空、Content Tool 切换、Controller readback、本地真人验证与完整回收。发布不会修改节点磁盘或已有分配记录。</p><div class="admin-form-row"><label>目标版本<select v-model="catalogDraft.publishVersionId"><option value="" disabled>请选择</option><option v-for="version in versionsFor(catalogDraft.gameId)" :key="version.id" :value="version.id">{{ version.id }}</option></select></label><button type="button" class="primary-button" :disabled="busy || !catalogDraft.publishVersionId" @click="publishVersion">发布目标版本</button></div><div v-for="version in versionsFor(catalogDraft.gameId)" :key="version.id" class="admin-compact-row"><div><strong>{{ version.id }}</strong><small>SHA256 {{ version.contentSha256 }} · {{ stamp(version.createdAt) }}</small></div><span class="admin-badge">{{ overview.games.find(game => game.id === catalogDraft.gameId)?.currentContentVersionId === version.id ? '当前目标' : '保留版本' }}</span></div></section>
        <section class="panel admin-card">
          <div class="admin-card-heading"><div><span class="eyebrow">游廊游戏</span><h2>地图设置</h2></div><span class="admin-count">{{ overview.games.length }} 张地图</span></div>
          <div v-for="game in overview.games" :key="game.id" class="admin-entity">
            <div class="admin-entity-head"><div><h3>{{ game.displayName }}</h3><p>创意工坊 {{ game.workshopId }} · 当前内容版本 <code>{{ game.currentContentVersionId || '未设置' }}</code></p></div><span class="admin-badge">{{ !game.enabled ? '已停用' : game.acceptingNewRequests ? '可申请' : '暂停申请' }}</span></div>
            <p v-if="game.maintenanceMessage" class="admin-note">维护提示：{{ game.maintenanceMessage }}</p>
            <div class="admin-actions"><button type="button" class="secondary-button" :disabled="busy" @click="act({action:'game.update',targetId:game.id,accepting:!game.acceptingNewRequests})">{{ game.acceptingNewRequests ? '暂停申请' : '恢复申请' }}</button><button type="button" class="admin-text-button" :disabled="busy" @click="editMessage('game.update',game.id,game.maintenanceMessage)">编辑维护提示</button><button type="button" class="admin-text-button admin-muted-action" :disabled="busy" @click="act({action:'game.update',targetId:game.id,enabled:!game.enabled})">{{ game.enabled ? '停用地图' : '启用地图' }}</button></div>
          </div>
          <p v-if="overview.games.length === 0" class="admin-empty">还没有游廊游戏。</p>
        </section>
        <section class="panel admin-card">
          <div class="admin-card-heading"><div><span class="eyebrow">玩法预设</span><h2>玩法设置</h2></div><span class="admin-count">{{ overview.presets.length }} 个玩法</span></div>
          <div v-for="preset in overview.presets" :key="preset.id" class="admin-entity">
            <div class="admin-entity-head"><div><h3>{{ preset.displayName }} <small>· {{ gameName(preset.arcadeGameId) }}</small></h3><p>最多 {{ preset.maxPlayers }} 人 · 启动模板 <code>{{ preset.templateRevisionId }}</code></p></div><span class="admin-badge">{{ !preset.enabled ? '已停用' : preset.acceptingNewRequests ? '可申请' : '暂停申请' }}</span></div>
            <p v-if="preset.maintenanceMessage" class="admin-note">维护提示：{{ preset.maintenanceMessage }}</p>
            <div class="admin-actions"><button type="button" class="secondary-button" :disabled="busy" @click="act({action:'preset.update',targetId:preset.id,accepting:!preset.acceptingNewRequests})">{{ preset.acceptingNewRequests ? '暂停申请' : '恢复申请' }}</button><button type="button" class="admin-text-button" :disabled="busy" @click="editMessage('preset.update',preset.id,preset.maintenanceMessage)">编辑维护提示</button><button type="button" class="admin-text-button admin-muted-action" :disabled="busy" @click="act({action:'preset.update',targetId:preset.id,enabled:!preset.enabled})">{{ preset.enabled ? '停用玩法' : '启用玩法' }}</button></div>
          </div>
          <p v-if="overview.presets.length === 0" class="admin-empty">还没有玩法预设。</p>
        </section>
      </template>
      <template v-else-if="tab === 'nodes'">
        <div class="admin-section-heading"><div><span class="eyebrow">游戏节点</span><h2>节点与入口</h2><p>先选择节点，再查看调度设置、地图分配和一键入口。</p></div></div>
        <div class="admin-choice-grid" aria-label="选择节点">
          <button v-for="node in overview.nodes" :key="node.id" type="button" class="admin-choice" :aria-pressed="selectedNode?.id === node.id" @click="selectedNodeId = node.id">
            <span class="admin-choice-top"><strong>{{ node.displayName }}</strong><span class="admin-badge" :class="node.connectivity">{{ connectivityLabel(node.connectivity) }}</span></span>
            <span class="admin-choice-meta">{{ node.os === 'windows' ? 'Windows' : 'Linux' }} · {{ compatibilityLabel(node.compatibility) }} · {{ node.occupied }}/{{ node.desired }} 已占用</span>
            <span class="admin-choice-foot">{{ nodeAdmissionLabel(node) }} · 最近心跳 {{ stamp(node.lastHeartbeat) }}</span>
          </button>
        </div>
        <div v-if="selectedNode" class="admin-node-detail">
          <div class="admin-section-heading"><div><span class="eyebrow">当前选中节点</span><h2>{{ selectedNode.displayName }}</h2></div><span class="admin-badge" :class="selectedNode.connectivity">{{ connectivityLabel(selectedNode.connectivity) }}</span></div>
          <div class="admin-detail-grid">
            <section class="panel admin-card">
              <span class="eyebrow">整台节点</span><h2>调度与容量</h2>
              <div class="admin-fact-grid"><span>系统<strong>{{ selectedNode.os === 'windows' ? 'Windows' : 'Linux' }}</strong></span><span>最后心跳<strong>{{ stamp(selectedNode.lastHeartbeat) }}</strong></span><span>运行组件<strong>{{ compatibilityLabel(selectedNode.compatibility) }}</strong></span><span>维护模式<strong>{{ selectedNode.draining ? '已开启' : '已关闭' }}</strong></span><span>新分配<strong>{{ nodeAdmissionLabel(selectedNode) }}</strong></span><span>分配优先级<strong>{{ selectedNode.priority }}</strong></span></div>
              <div class="admin-capacity-strip" aria-label="节点容量"><span>已占用<strong>{{ selectedNode.occupied }}</strong></span><span>期望容量<strong>{{ selectedNode.desired }}</strong></span><span>节点上报上限<strong>{{ selectedNode.hard }}</strong></span></div>
              <p class="admin-capacity-note">期望容量是平台允许使用的名额，不得超过节点上报上限。</p>
              <p v-if="selectedNode.recentErrorCode" class="admin-warning">最近错误代码：{{ selectedNode.recentErrorCode }}</p>
              <div class="admin-control-row"><button type="button" class="secondary-button" :disabled="busy" @click="act({action:'node.update',targetId:selectedNode.id,draining:!selectedNode.draining})">{{ selectedNode.draining ? '结束节点维护' : '进入节点维护' }}</button><span>维护时暂停新分配，已有服务器继续运行。</span></div>
              <div class="admin-form-row"><label>分配优先级<input v-model.number="nodeDraft[selectedNode.id].priority" type="number" step="1" /></label><label>期望容量<input v-model.number="nodeDraft[selectedNode.id].desired" type="number" min="0" :max="selectedNode.hard" /></label></div>
              <button type="button" class="secondary-button" :disabled="busy" @click="act({action:'node.update',targetId:selectedNode.id,priority:nodeDraft[selectedNode.id].priority,desired:nodeDraft[selectedNode.id].desired})">保存调度设置</button>
              <details class="admin-technical"><summary>组件版本与核对</summary><p>节点控制器 {{ selectedNode.controllerVersion || '未上报' }} · d2core {{ selectedNode.d2coreVersion || '未上报' }}</p><p>核对批次 {{ selectedNode.reconcileCompleted }}/{{ selectedNode.reconcileRequested }}{{ selectedNode.reconcileRequested > selectedNode.reconcileCompleted ? ' · 等待节点处理' : '' }}</p><button type="button" class="secondary-button" :disabled="busy" @click="act({action:'node.reconcile',targetId:selectedNode.id})">请求完整核对</button></details>
            </section>
            <section class="panel admin-card">
              <span class="eyebrow">节点 × 地图</span><h2>地图分配</h2><p>内容版本和准备状态由节点上报；这里仅控制是否继续向该节点分配这张地图。</p>
              <div v-for="binding in bindingsFor(selectedNode.id)" :key="binding.arcadeGameId" class="admin-entity admin-binding-detail"><div class="admin-entity-head"><div><h3>{{ gameName(binding.arcadeGameId) }}</h3><p>已上报版本 <code>{{ binding.reportedContentVersionId || '未知' }}</code> · {{ reportedStateLabel(binding.reportedState) }}</p><small>上报时间 {{ stamp(binding.reportedAt) }}</small></div><span class="admin-badge">{{ binding.acceptingNewAllocations ? '允许分配' : '暂停分配' }}</span></div><button type="button" class="secondary-button" :disabled="busy" @click="act({action:'binding.update',targetId:selectedNode.id,gameId:binding.arcadeGameId,accepting:!binding.acceptingNewAllocations})">{{ binding.acceptingNewAllocations ? '暂停这张地图' : '恢复这张地图' }}</button></div>
              <p v-if="bindingsFor(selectedNode.id).length === 0" class="admin-empty">这台节点没有地图分配记录。</p>
              <div v-for="binding in bindingsFor(selectedNode.id)" :key="`${binding.arcadeGameId}-validation`" class="admin-content-validation"><strong>{{ gameName(binding.arcadeGameId) }} · {{ binding.reportedContentVersionId || '版本未知' }}</strong><small v-if="validationFor(selectedNode.id,binding.arcadeGameId,binding.reportedContentVersionId)">真人验证记录：{{ stamp(validationFor(selectedNode.id,binding.arcadeGameId,binding.reportedContentVersionId)?.verifiedAt) }}</small><small v-else>尚无此节点此版本的真人验证记录</small><button type="button" class="secondary-button" :disabled="busy || !selectedNode.draining || selectedNode.occupied !== 0 || binding.reportedState !== 'confirmed'" @click="act({action:'content.validate',targetId:selectedNode.id,gameId:binding.arcadeGameId,contentVersionId:binding.reportedContentVersionId,confirmed:true},'确认该节点已 Drain，目标版本完成本地真人进房验证，临时实例已 stop 并完整 reclaimed/stopped/complete，Controller 已 resync 且无遗留实例？此操作仅记录人工验证，不修改节点内容。')">记录真人验证</button></div>
            </section>
          </div>
          <section class="panel admin-card"><span class="eyebrow">启动模板</span><h2>节点模板映射</h2><p>这里只登记逻辑绑定键；请先确认 Controller 本地配置和正式模板文件已准备好。</p><div v-for="binding in overview.templateBindings.filter(item => item.nodeId === selectedNode?.id)" :key="binding.templateRevisionId" class="admin-compact-row"><div><strong>{{ binding.templateRevisionId }}</strong><small>绑定键 {{ binding.bindingKey }}</small></div></div><form class="admin-form-row" @submit.prevent="act({action:'template_binding.upsert',targetId:selectedNode?.id,templateRevisionId:templateBindingDraft.revisionId,bindingKey:templateBindingDraft.bindingKey})"><label>模板修订<select v-model="templateBindingDraft.revisionId" required><option value="" disabled>请选择</option><option v-for="revision in overview.templateRevisions" :key="revision.id" :value="revision.id">{{ gameName(revision.arcadeGameId) }} · {{ revision.id }}</option></select></label><label>Controller 绑定键<input v-model.trim="templateBindingDraft.bindingKey" required maxlength="128" /></label><button type="submit" class="secondary-button" :disabled="busy">保存映射</button></form></section>
          <section v-for="entry in entriesFor(selectedNode.id)" :key="entry.nodeId" class="panel admin-card">
            <span class="eyebrow">玩家入口</span><h2>一键进入游戏</h2><p>A2S 诊断：{{ entry.a2sEnabled ? (entry.a2sQueryOk ? '查询成功' : '查询失败') : '未启用' }}。诊断结果不代替真人一键进入验证；真人验证须覆盖当前配置和全部可分配的公网端口。</p><p>当前入口配置修订 {{ entry.entryConfigRevision.slice(0, 12) }} · 需逐一测试的公网端口：{{ entry.publicPorts.join('、') || '未配置' }}</p>
            <div class="admin-entry-grid"><div v-for="kind in (['steam','steamchina'] as const)" :key="kind" class="admin-entry-option"><div><h3>{{ kind === 'steam' ? 'Steam' : '蒸汽平台' }}</h3><p>真人验证 {{ kind === 'steam' ? (entry.steamVerified ? '已确认' : '未确认') : (entry.steamChinaVerified ? '已确认' : '未确认') }} · 玩家入口 {{ kind === 'steam' ? (entry.steamEnabled ? '开启' : '关闭') : (entry.steamChinaEnabled ? '开启' : '关闭') }}</p><small>已验证端口 {{ (kind === 'steam' ? entry.steamVerifiedPorts : entry.steamChinaVerifiedPorts).join('、') || '无' }} · 验证时间 {{ stamp(kind === 'steam' ? entry.steamVerifiedAt : entry.steamChinaVerifiedAt) }}</small></div><div class="admin-actions"><button type="button" class="secondary-button" :disabled="busy" @click="entryAction(selectedNode,entry,kind,'verified',kind === 'steam' ? !entry.steamVerified : !entry.steamChinaVerified)">{{ kind === 'steam' ? (entry.steamVerified ? '撤销验证' : '确认真人验证') : (entry.steamChinaVerified ? '撤销验证' : '确认真人验证') }}</button><button type="button" class="secondary-button" :disabled="busy || !(kind === 'steam' ? entry.steamVerified : entry.steamChinaVerified)" @click="entryAction(selectedNode,entry,kind,'enabled',kind === 'steam' ? !entry.steamEnabled : !entry.steamChinaEnabled)">{{ kind === 'steam' ? (entry.steamEnabled ? '关闭入口' : '开启入口') : (entry.steamChinaEnabled ? '关闭入口' : '开启入口') }}</button></div></div></div>
          </section>
        </div>
        <p v-else class="admin-empty">还没有游戏节点。</p>
      </template>
      <template v-else-if="tab === 'instances'">
        <div class="admin-section-heading"><div><span class="eyebrow">申请与实例</span><h2>服务器处理</h2><p>先查看需要处理的申请，再展开该申请的分配记录和操作。</p></div></div>
        <div class="admin-subtabs" aria-label="申请范围"><button type="button" :aria-pressed="requestView === 'active'" @click="requestView = 'active'">进行中 <span>{{ activeRequestCount }}</span></button><button type="button" :aria-pressed="requestView === 'history'" @click="requestView = 'history'">历史记录 <span>{{ overview.requests.length - activeRequestCount }}</span></button></div>
        <div class="admin-workspace">
          <section class="panel admin-card admin-workspace-list"><span class="eyebrow">最近 100 条申请</span><h2>{{ requestView === 'active' ? '进行中的申请' : '历史申请' }}</h2>
            <button v-for="request in filteredRequests" :key="request.id" type="button" class="admin-record" :aria-pressed="selectedRequest?.id === request.id" @click="selectedRequestId = request.id"><span class="admin-record-top"><strong>{{ gameName(request.arcadeGameId) }}</strong><span class="admin-badge" :class="{ 'admin-badge-warm': ['quarantined','failed_unreclaimed'].includes(request.state) }">{{ stateLabel(request.state) }}</span></span><small>{{ request.ownerPartyId ? '队伍申请' : '单人申请' }} · {{ stamp(request.requestedAt) }}</small><small>申请编号 {{ request.id.slice(0, 8) }}</small></button>
            <p v-if="filteredRequests.length === 0" class="admin-empty">{{ requestView === 'active' ? '目前没有进行中的申请。' : '暂无历史申请。' }}</p>
          </section>
          <section v-if="selectedRequest" class="panel admin-card admin-record-detail"><span class="eyebrow">当前选中申请</span><div class="admin-card-heading"><div><h2>{{ gameName(selectedRequest.arcadeGameId) }} · {{ stateLabel(selectedRequest.state) }}</h2><p>{{ selectedRequest.ownerPartyId ? '队伍申请' : '单人申请' }} · {{ selectionLabel(selectedRequest.nodeSelectionMode) }} · {{ stamp(selectedRequest.requestedAt) }}</p></div><span class="admin-badge" :class="{ 'admin-badge-warm': ['quarantined','failed_unreclaimed'].includes(selectedRequest.state) }">{{ stateLabel(selectedRequest.state) }}</span></div>
            <div class="admin-detail-facts"><span>玩法<strong>{{ overview.presets.find(p => p.id === selectedRequest?.gamePresetId)?.displayName || '未知' }}</strong></span><span>节点选择<strong>{{ selectionLabel(selectedRequest.nodeSelectionMode) }}{{ selectedRequest.manualNodeId ? ' · ' + nodeName(selectedRequest.manualNodeId) : '' }}</strong></span><span>申请编号<strong>{{ selectedRequest.id.slice(0, 8) }}</strong></span></div>
            <div class="admin-actions"><button v-if="selectedRequest.state === 'waiting' && !overview.allocations.some(a => a.serverRequestId === selectedRequest?.id)" type="button" class="secondary-button" :disabled="busy" @click="act({action:'request.cancel',targetId:selectedRequest.id},'仅取消尚未占用节点的等待申请。确定继续？')">取消等待申请</button><button v-if="selectedRequest.state === 'running' || selectedRequest.state === 'quarantined' || (selectedRequest.state === 'abandoned' && overview.allocations.some(a => a.serverRequestId === selectedRequest?.id && a.state === 'quarantined'))" type="button" class="secondary-button danger" :disabled="busy" @click="act({action:'request.stop',targetId:selectedRequest.id},'向原节点请求停止并完整回收。确定继续？')">请求停止服务器</button></div>
            <div class="admin-detail-divider"><span class="eyebrow">资源分配记录</span><span>{{ overview.allocations.filter(a => a.serverRequestId === selectedRequest?.id).length }} 次尝试</span></div>
            <div v-for="allocation in overview.allocations.filter(a => a.serverRequestId === selectedRequest?.id)" :key="allocation.id" class="admin-attempt"><div class="admin-entity-head"><div><h3>第 {{ allocation.attemptSequence }} 次分配 · {{ stateLabel(allocation.state) }}</h3><p>{{ nodeName(allocation.nodeId) }} · 内容版本 <code>{{ allocation.contentVersionId }}</code> · 模板 <code>{{ allocation.templateRevisionId }}</code></p><small v-if="allocation.errorCode">诊断代码 {{ allocation.errorCode }}</small></div><button v-if="!['reclaimed','released_no_effect','quarantined'].includes(allocation.state)" type="button" class="secondary-button danger" :disabled="busy" @click="act({action:'allocation.quarantine',targetId:allocation.id},'依据诊断提前隔离这次分配？容量和端口仍占用，不能据此认定旧服务器已停止。')">标记异常隔离</button></div></div>
            <p v-if="!overview.allocations.some(a => a.serverRequestId === selectedRequest?.id)" class="admin-empty">此申请尚未分配节点。</p>
          </section>
        </div>
        <div class="admin-columns admin-lower-panels"><section class="panel admin-card"><div class="admin-card-heading"><div><span class="eyebrow">节点任务</span><h2>待处理与状态待确认</h2></div><span class="admin-count">{{ overview.jobs.length }} 条</span></div><div v-for="job in overview.jobs" :key="job.id" class="admin-compact-row"><div><strong>{{ jobKindLabel(job.kind) }} · {{ stateLabel(job.state) }}</strong><small>{{ nodeName(job.nodeId) }} · 分配 {{ job.allocationId ? job.allocationId.slice(0, 8) : '关联任务' }}</small><small v-if="job.errorCode">诊断代码 {{ job.errorCode }}</small></div><time>{{ stamp(job.updatedAt) }}</time></div><p v-if="overview.jobs.length === 0" class="admin-empty">当前没有待处理任务。</p></section><section class="panel admin-card"><div class="admin-card-heading"><div><span class="eyebrow">队伍</span><h2>队伍概况</h2></div><span class="admin-count">{{ activeParties.length }} 个活动中</span></div><div v-for="partyItem in activeParties.slice(0, 6)" :key="partyItem.id" class="admin-compact-row"><div><strong>{{ partyItem.leaderDisplayName || '未命名队伍' }}</strong><small>{{ partyItem.memberCount }} 人 · 队伍 {{ partyItem.id.slice(0,8) }}</small></div></div><p v-if="activeParties.length === 0" class="admin-empty">目前没有活动队伍。</p><details v-if="activeParties.length > 6" class="admin-collapsible"><summary>查看其余 {{ activeParties.length - 6 }} 个活动队伍</summary><div class="admin-party-scroll"><div v-for="partyItem in activeParties.slice(6)" :key="partyItem.id" class="admin-compact-row"><div><strong>{{ partyItem.leaderDisplayName || '未命名队伍' }}</strong><small>{{ partyItem.memberCount }} 人 · 队伍 {{ partyItem.id.slice(0,8) }}</small></div></div></div></details><details v-if="dissolvedParties.length" class="admin-collapsible"><summary>已解散队伍 · {{ dissolvedParties.length }} 个</summary><div class="admin-party-scroll"><div v-for="partyItem in dissolvedParties" :key="partyItem.id" class="admin-compact-row"><div><strong>{{ partyItem.leaderDisplayName || '已解散队伍' }}</strong><small>{{ partyItem.memberCount }} 人 · 队伍 {{ partyItem.id.slice(0,8) }}</small></div></div></div></details><p class="admin-list-limit">显示最近记录中的队伍，最多 100 个。</p></section></div>
      </template>
      <template v-else>
        <div class="admin-section-heading"><div><span class="eyebrow">操作审计</span><h2>操作记录</h2><p>选择一条记录，查看操作人、对象和具体状态变化。显示最近 100 条。</p></div></div>
        <div class="admin-workspace">
          <section class="panel admin-card admin-workspace-list"><h2>最近操作</h2><button v-for="event in overview.audit" :key="event.id" type="button" class="admin-record" :aria-pressed="selectedAudit?.id === event.id" @click="selectedAuditId = event.id"><span class="admin-record-top"><strong>{{ auditActions[event.action] || event.action }}</strong><span class="admin-badge">{{ event.result === 'succeeded' ? '成功' : '失败' }}</span></span><small>{{ event.actorUsername || (event.actorKind === 'system' ? '系统自动处理' : '命令行管理员') }} · {{ stamp(event.createdAt) }}</small><small>{{ auditTarget(event) }}</small></button><p v-if="overview.audit.length === 0" class="admin-empty">还没有操作记录。</p></section>
          <section v-if="selectedAudit" class="panel admin-card admin-record-detail"><span class="eyebrow">操作详情</span><h2>{{ auditActions[selectedAudit.action] || selectedAudit.action }}</h2><div class="admin-detail-facts"><span>操作人<strong>{{ selectedAudit.actorUsername || (selectedAudit.actorKind === 'system' ? '系统自动处理' : '命令行管理员') }}</strong></span><span>操作时间<strong>{{ stamp(selectedAudit.createdAt) }}</strong></span><span>处理结果<strong>{{ selectedAudit.result === 'succeeded' ? '成功' : '失败' }}</strong></span><span>操作对象<strong>{{ auditTarget(selectedAudit) }}</strong></span></div><div class="admin-detail-divider"><span class="eyebrow">状态变化</span></div><dl class="admin-change-list"><div v-for="([key, value]) in Object.entries(selectedAudit.stateChange)" :key="key"><dt>{{ auditFields[key] || key }}</dt><dd>{{ auditValue(value) }}</dd></div></dl><p v-if="Object.keys(selectedAudit.stateChange).length === 0" class="admin-empty">这条记录没有额外状态字段。</p><details class="admin-technical"><summary>记录编号</summary><p>{{ selectedAudit.id }}</p><p>对象编号 {{ selectedAudit.targetId }}</p></details></section>
        </div>
      </template>
    </template>
  </main>
  <footer><span>Dota 2 Arcade Platform</span><span>平台管理</span></footer>
</template>








<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'

interface Settings { acceptingNewRequests: boolean; maintenanceMessage: string; siteAnnouncement: string }
interface Game { id: string; displayName: string; workshopId: string; currentContentVersionId: string; maintenanceMessage: string; enabled: boolean; acceptingNewRequests: boolean }
interface Preset { id: string; arcadeGameId: string; displayName: string; templateRevisionId: string; maintenanceMessage: string; enabled: boolean; acceptingNewRequests: boolean; maxPlayers: number }
interface Node { id: string; displayName: string; os: string; connectivity: string; controllerVersion: string; d2coreVersion: string; d2coreCommit: string; compatibility: string; recentErrorCode: string; enabled: boolean; acceptingNewRequests: boolean; draining: boolean; priority: number; hard: number; desired: number; occupied: number; lastHeartbeat?: string; reconcileRequested: number; reconcileCompleted: number }
interface Binding { nodeId: string; arcadeGameId: string; reportedContentVersionId: string; reportedState: string; reportedAt?: string; acceptingNewAllocations: boolean }
interface Entry { nodeId: string; a2sEnabled: boolean; a2sQueryOk: boolean; steamVerified: boolean; steamEnabled: boolean; steamChinaVerified: boolean; steamChinaEnabled: boolean; steamVerifiedAt?: string; steamChinaVerifiedAt?: string }
interface ServerRequest { id: string; arcadeGameId: string; gamePresetId: string; state: string; ownerPartyId?: string; nodeSelectionMode: string; manualNodeId?: string; requestedAt: string }
interface Party { id: string; leaderDisplayName: string; memberCount: number; dissolvedAt?: string }
interface Allocation { id: string; serverRequestId: string; nodeId: string; contentVersionId: string; templateRevisionId: string; state: string; errorCode: string; attemptSequence: number; assignedAt: string }
interface Job { id: string; nodeId: string; allocationId: string; kind: string; state: string; errorCode: string; updatedAt: string }
interface Audit { id: string; actorUsername: string; actorKind: string; action: string; targetType: string; targetId: string; result: string; stateChange: Record<string, unknown>; createdAt: string }
interface Counts { waiting: number; creating: number; running: number; stopping: number; quarantined: number; failedUnreclaimed: number }
interface Overview { settings: Settings; counts: Counts; games: Game[]; presets: Preset[]; nodes: Node[]; bindings: Binding[]; entries: Entry[]; requests: ServerRequest[]; parties: Party[]; allocations: Allocation[]; jobs: Job[]; audit: Audit[] }
interface Action { action: string; targetId?: string; gameId?: string; accepting?: boolean; enabled?: boolean; draining?: boolean; priority?: number; desired?: number; message?: string; entry?: string; verified?: boolean; confirmed?: boolean }

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

async function refresh(): Promise<void> {
  const data = await api<Overview>('/overview')
  overview.value = data
  Object.assign(globalDraft, { accepting: data.settings.acceptingNewRequests,
    maintenance: data.settings.maintenanceMessage, announcement: data.settings.siteAnnouncement })
  for (const node of data.nodes) nodeDraft[node.id] = { priority: node.priority, desired: node.desired }
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
function editMessage(kind: 'game.update' | 'preset.update', id: string, current: string): void {
  const message = window.prompt(kind === 'game.update' ? '地图维护提示' : '玩法维护提示', current)
  if (message !== null) void act({ action: kind, targetId: id, message })
}

function entryAction(node: Node, kind: 'steam' | 'steamchina', field: 'verified' | 'enabled', value: boolean): void {
  const base: Action = { action: 'entry.update', targetId: node.id, entry: kind }
  if (field === 'verified') {
    base.verified = value
    base.confirmed = value
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
      <div class="admin-heading"><div><span class="eyebrow">运维控制台</span><h1>平台状态</h1><p>控制新申请，观察资源，并处理需要人工判断的异常。</p></div><button type="button" class="secondary-button" :disabled="busy" @click="refresh">刷新状态</button></div>
      <nav class="admin-tabs" aria-label="管理页面"><button v-for="item in ([['overview','总览'],['content','内容与维护'],['nodes','节点'],['instances','申请与实例'],['audit','审计']] as const)" :key="item[0]" type="button" :class="{ selected: tab === item[0] }" @click="tab = item[0]">{{ item[1] }}</button></nav>
      <template v-if="tab === 'overview'">
        <div class="admin-metrics"><div v-for="(value, label) in counts" :key="label" class="panel admin-metric"><span>{{ { waiting:'排队', creating:'创建中', running:'运行中', stopping:'停止中', quarantined:'隔离中', failedUnreclaimed:'待回收异常' }[label] }}</span><strong>{{ value }}</strong></div></div>
        <div class="admin-columns"><section class="panel admin-card"><span class="eyebrow">全局维护</span><h2>{{ overview.settings.acceptingNewRequests ? '正在接受新申请' : '已暂停新申请' }}</h2><label class="admin-check"><input v-model="globalDraft.accepting" type="checkbox" /> 接受新申请</label><label>维护提示<textarea v-model="globalDraft.maintenance" maxlength="1000" rows="3" /></label><button type="button" class="primary-button" :disabled="busy" @click="saveGlobal">保存全局状态</button></section>
          <section class="panel admin-card"><span class="eyebrow">站点公告</span><h2>玩家可见消息</h2><p>公告独立于是否暂停新申请。</p><label>公告内容<textarea v-model="globalDraft.announcement" maxlength="1000" rows="4" placeholder="留空可撤下公告" /></label><button type="button" class="secondary-button" :disabled="busy" @click="saveAnnouncement">保存公告</button></section></div>
        <section class="panel admin-card"><span class="eyebrow">节点摘要</span><h2>容量与连接</h2><div class="admin-list"><div v-for="node in overview.nodes" :key="node.id" class="admin-row"><div><strong>{{ node.displayName }}</strong><small>{{ node.os }} · {{ node.compatibility || '未报告兼容性' }} · 最近心跳 {{ stamp(node.lastHeartbeat) }}</small></div><div class="admin-row-side"><span class="admin-badge" :class="node.connectivity">{{ node.connectivity }}</span><strong>{{ node.occupied }} / {{ node.desired }} <small>期望</small></strong></div></div></div></section>
      </template>
      <template v-else-if="tab === 'content'"><section class="panel admin-card"><span class="eyebrow">ArcadeGame</span><h2>地图与内容目标</h2><div v-for="game in overview.games" :key="game.id" class="admin-item"><div class="admin-item-head"><div><h3>{{ game.displayName }}</h3><p>Workshop {{ game.workshopId }} · 目标 ContentVersion <code>{{ game.currentContentVersionId || '未设置' }}</code></p></div><span class="admin-badge">{{ game.enabled ? '已启用' : '已停用' }}</span></div><div class="admin-actions"><button type="button" class="secondary-button" :disabled="busy" @click="act({action:'game.update',targetId:game.id,enabled:!game.enabled})">{{ game.enabled ? '停用地图' : '启用地图' }}</button><button type="button" class="secondary-button" :disabled="busy" @click="act({action:'game.update',targetId:game.id,accepting:!game.acceptingNewRequests})">{{ game.acceptingNewRequests ? '暂停新申请' : '恢复新申请' }}</button><button type="button" class="admin-text-button" :disabled="busy" @click="editMessage('game.update',game.id,game.maintenanceMessage)">编辑提示</button></div><p v-if="game.maintenanceMessage" class="admin-note">{{ game.maintenanceMessage }}</p></div></section>
        <section class="panel admin-card"><span class="eyebrow">GamePreset</span><h2>玩法与模板</h2><div v-for="preset in overview.presets" :key="preset.id" class="admin-item"><div class="admin-item-head"><div><h3>{{ preset.displayName }} <small>· {{ gameName(preset.arcadeGameId) }}</small></h3><p>TemplateRevision <code>{{ preset.templateRevisionId }}</code> · 最多 {{ preset.maxPlayers }} 人</p></div><span class="admin-badge">{{ preset.enabled ? '已启用' : '已停用' }}</span></div><div class="admin-actions"><button type="button" class="secondary-button" :disabled="busy" @click="act({action:'preset.update',targetId:preset.id,enabled:!preset.enabled})">{{ preset.enabled ? '停用玩法' : '启用玩法' }}</button><button type="button" class="secondary-button" :disabled="busy" @click="act({action:'preset.update',targetId:preset.id,accepting:!preset.acceptingNewRequests})">{{ preset.acceptingNewRequests ? '暂停新申请' : '恢复新申请' }}</button><button type="button" class="admin-text-button" :disabled="busy" @click="editMessage('preset.update',preset.id,preset.maintenanceMessage)">编辑提示</button></div><p v-if="preset.maintenanceMessage" class="admin-note">{{ preset.maintenanceMessage }}</p></div></section></template>
      <template v-else-if="tab === 'nodes'"><section v-for="node in overview.nodes" :key="node.id" class="panel admin-card admin-node"><div class="admin-item-head"><div><span class="eyebrow">{{ node.os }} 节点 · {{ node.connectivity }}</span><h2>{{ node.displayName }}</h2><p>Controller {{ node.controllerVersion || '未上报' }} · d2core {{ node.d2coreVersion || '未上报' }} · {{ node.compatibility || '兼容性未知' }}</p></div><span class="admin-badge" :class="node.connectivity">{{ node.occupied }}/{{ node.desired }} 占用</span></div><div class="admin-node-facts"><span>最后心跳<strong>{{ stamp(node.lastHeartbeat) }}</strong></span><span>硬上限<strong>{{ node.hard }}</strong></span><span>期望上限<strong>{{ node.desired }}</strong></span><span>优先级<strong>{{ node.priority }}</strong></span><span>Drain<strong>{{ node.draining ? '开启' : '关闭' }}</strong></span><span>接受新申请<strong>{{ node.acceptingNewRequests ? '是' : '否' }}</strong></span><span>最近错误<strong>{{ node.recentErrorCode || '无' }}</strong></span></div><div class="admin-actions"><button type="button" class="secondary-button" :disabled="busy" @click="act({action:'node.update',targetId:node.id,draining:!node.draining})">{{ node.draining ? '恢复分配' : 'Drain 节点' }}</button><label>优先级<input v-model.number="nodeDraft[node.id].priority" type="number" step="1" /></label><label>期望容量<input v-model.number="nodeDraft[node.id].desired" type="number" min="0" :max="node.hard" /></label><button type="button" class="primary-button" :disabled="busy" @click="act({action:'node.update',targetId:node.id,priority:nodeDraft[node.id].priority,desired:nodeDraft[node.id].desired})">保存节点设置</button><button type="button" class="secondary-button" :disabled="busy" @click="act({action:'node.reconcile',targetId:node.id})">请求完整对账</button><small class="admin-reconcile-state">对账 {{ node.reconcileCompleted }}/{{ node.reconcileRequested }}{{ node.reconcileRequested > node.reconcileCompleted ? ' · 等待节点处理' : '' }}</small></div><div v-for="binding in overview.bindings.filter(b => b.nodeId === node.id)" :key="binding.arcadeGameId" class="admin-binding"><div><strong>{{ gameName(binding.arcadeGameId) }}</strong><small>Controller 报告 {{ binding.reportedContentVersionId || '未知' }} · {{ binding.reportedState }} · {{ stamp(binding.reportedAt) }}</small></div><button type="button" class="secondary-button" :disabled="busy" @click="act({action:'binding.update',targetId:node.id,gameId:binding.arcadeGameId,accepting:!binding.acceptingNewAllocations})">{{ binding.acceptingNewAllocations ? '暂停此地图分配' : '恢复此地图分配' }}</button></div><div v-for="entry in overview.entries.filter(e => e.nodeId === node.id)" :key="entry.nodeId" class="admin-entry"><h3>一键入口</h3><p>A2S {{ entry.a2sEnabled ? (entry.a2sQueryOk ? '已报告可用' : '查询失败') : '未启用' }}。真人验证必须覆盖当前配置和全部可分配公网端口。</p><div v-for="kind in (['steam','steamchina'] as const)" :key="kind" class="admin-binding"><div><strong>{{ kind === 'steam' ? 'Steam' : '蒸汽平台' }}</strong><small>真人验证：{{ kind === 'steam' ? (entry.steamVerified ? '已确认' : '未确认') : (entry.steamChinaVerified ? '已确认' : '未确认') }} · 玩家入口：{{ kind === 'steam' ? (entry.steamEnabled ? '开启' : '关闭') : (entry.steamChinaEnabled ? '开启' : '关闭') }}</small></div><div class="admin-actions"><button type="button" class="secondary-button" :disabled="busy || (!(entry.a2sEnabled && entry.a2sQueryOk) && !(kind === 'steam' ? entry.steamVerified : entry.steamChinaVerified))" @click="entryAction(node,kind,'verified',kind === 'steam' ? !entry.steamVerified : !entry.steamChinaVerified)">{{ kind === 'steam' ? (entry.steamVerified ? '撤销验证' : '确认真人验证') : (entry.steamChinaVerified ? '撤销验证' : '确认真人验证') }}</button><button type="button" class="secondary-button" :disabled="busy || !(kind === 'steam' ? entry.steamVerified : entry.steamChinaVerified)" @click="entryAction(node,kind,'enabled',kind === 'steam' ? !entry.steamEnabled : !entry.steamChinaEnabled)">{{ kind === 'steam' ? (entry.steamEnabled ? '关闭入口' : '开启入口') : (entry.steamChinaEnabled ? '关闭入口' : '开启入口') }}</button></div></div></div></section></template>
      <template v-else-if="tab === 'instances'"><section class="panel admin-card"><span class="eyebrow">最近 100 条</span><h2>申请与分配</h2><div v-for="request in overview.requests" :key="request.id" class="admin-item"><div class="admin-item-head"><div><h3>{{ gameName(request.arcadeGameId) }} · {{ request.state }}</h3><p>申请 {{ request.id.slice(0,8) }} · {{ request.ownerPartyId ? 'Party ' + request.ownerPartyId.slice(0,8) : '单人' }} · {{ request.nodeSelectionMode }} · {{ stamp(request.requestedAt) }}</p></div><div class="admin-actions"><button v-if="request.state === 'waiting'" type="button" class="secondary-button" :disabled="busy" @click="act({action:'request.cancel',targetId:request.id},'只取消没有 Allocation 的安全等待申请。确定继续？')">安全取消</button><button v-if="request.state === 'running' || request.state === 'quarantined'" type="button" class="secondary-button danger" :disabled="busy" @click="act({action:'request.stop',targetId:request.id},'向原节点请求正式 stop/full reclaim。确定继续？')">请求停止</button></div></div><div v-for="allocation in overview.allocations.filter(a => a.serverRequestId === request.id)" :key="allocation.id" class="admin-allocation"><div><strong>尝试 {{ allocation.attemptSequence }} · {{ allocation.state }}</strong><small>{{ nodeName(allocation.nodeId) }} · Content {{ allocation.contentVersionId }} · Template {{ allocation.templateRevisionId }} · {{ allocation.errorCode || '无错误代码' }}</small></div><button v-if="!['reclaimed','released_no_effect','quarantined'].includes(allocation.state)" type="button" class="secondary-button danger" :disabled="busy" @click="act({action:'allocation.quarantine',targetId:allocation.id},'依据诊断提前隔离此 Allocation？容量与端口继续占用，绝不表示旧 Dota 已停止。')">标记隔离</button></div></div></section><section class="panel admin-card"><span class="eyebrow">待处理 NodeJob</span><h2>任务与未知状态</h2><div v-for="job in overview.jobs" :key="job.id" class="admin-row"><div><strong>{{ job.kind }} · {{ job.state }}</strong><small>{{ nodeName(job.nodeId) }} · Allocation {{ job.allocationId ? job.allocationId.slice(0,8) : '集成任务' }} · {{ job.errorCode || '无错误代码' }}</small></div><span>{{ stamp(job.updatedAt) }}</span></div><p v-if="overview.jobs.length === 0">当前没有待处理任务。</p></section><section class="panel admin-card"><span class="eyebrow">最近 100 个队伍</span><h2>Party</h2><div v-for="partyItem in overview.parties" :key="partyItem.id" class="admin-row"><div><strong>{{ partyItem.leaderDisplayName || '已解散队伍' }}</strong><small>Party {{ partyItem.id.slice(0,8) }} · {{ partyItem.memberCount }} 人 · {{ partyItem.dissolvedAt ? '已解散' : '活动中' }}</small></div></div><p v-if="overview.parties.length === 0">没有队伍记录。</p></section></template>
      <section v-else class="panel admin-card"><span class="eyebrow">最近 100 条</span><h2>AuditEvent</h2><div v-for="event in overview.audit" :key="event.id" class="admin-row admin-audit"><div><strong>{{ event.action }} · {{ event.result }}</strong><small>{{ event.actorUsername || event.actorKind }} · {{ event.targetType }} / {{ event.targetId }} · {{ JSON.stringify(event.stateChange) }}</small></div><span>{{ stamp(event.createdAt) }}</span></div><p v-if="overview.audit.length === 0">还没有管理员操作记录。</p></section>
    </template>
  </main>
  <footer><span>Dota 2 Arcade Platform</span><span>平台管理</span></footer>
</template>








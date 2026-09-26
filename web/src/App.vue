<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ApiError, api } from './api'
import type { Allocation, Catalog, NextGameIntent, NodeChoice, Party, PartyInvite, ServerRequest } from './api'
import { maintenanceFor, statusFor } from './state'
import { elapsedFor } from './elapsed'
import { inviteTokenFromHash, partyInviteLink, tokenFromInviteInput } from './partyInvite'

const savedRequestKey = 'arcade.lastRequestId'
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const copied = ref(false)
const copiedConsole = ref(false)
const copiedProtocol = ref('')
const catalog = ref<Catalog | null>(null)
const userId = ref('')
const displayName = ref('')
const party = ref<Party | null>(null)
const invite = ref<PartyInvite | null>(null)
const inviteToken = ref(inviteTokenFromHash(window.location.hash))
const partyView = ref(!!inviteToken.value)
const partyNotice = ref('')
const copiedInvite = ref(false)
const currentRequest = ref<ServerRequest | null>(null)
const allocation = ref<Allocation | null>(null)
const nextGameIntent = ref<NextGameIntent | null>(null)
const gameId = ref('')
const presetId = ref('')
const nodes = ref<NodeChoice[]>([])
const nodeSelectionMode = ref<'auto' | 'manual'>('auto')
const manualNodeId = ref('')
let timer: number | undefined
let polling = false
let elapsedRequestId = ''
const observedServerMs = ref(Date.now())

const game = computed(() => catalog.value?.games.find((item) => item.id === (currentRequest.value?.arcadeGameId || gameId.value)))
const preset = computed(() => catalog.value?.presets.find((item) => item.id === (currentRequest.value?.gamePresetId || presetId.value)))
const selectedPresets = computed(() => catalog.value?.presets.filter((item) => item.arcadeGameId === gameId.value) || [])
const state = computed(() => statusFor(currentRequest.value, allocation.value, nodes.value, nextGameIntent.value))
const elapsed = computed(() => elapsedFor(currentRequest.value, allocation.value, state.value.phase, observedServerMs.value))
const selectedNode = computed(() => nodes.value.find((item) => item.id === (currentRequest.value?.manualNodeId || manualNodeId.value)))
const assignedNodeName = computed(() => allocation.value?.nodeDisplayName || (currentRequest.value?.nodeSelectionMode === 'manual' ? selectedNode.value?.displayName : '') || '等待分配')
const selectionName = computed(() => currentRequest.value?.nodeSelectionMode === 'manual'
  ? (selectedNode.value?.displayName || '所选节点') : '自动选择')
const selectionSummary = computed(() => nodeSelectionMode.value === 'auto'
  ? '平台自动选择当前可用节点'
  : manualNodeId.value
    ? `只等待 ${selectedNode.value?.displayName || '所选节点'}，不会自动切换`
    : '手动指定；请先选择要等待的节点')
const nodeStatus = (node: NodeChoice): string => node.status === 'available'
  ? `可申请 · ${node.availableSlots} 个空位`
  : node.status === 'full' ? '满载 · 可排队'
    : node.reason === 'maintenance' ? '维护中 · 可等待恢复'
      : node.reason === 'unreachable' ? '暂不可达 · 可等待恢复'
        : node.reason === 'content_unready' || node.reason === 'template_unready' ? '内容尚未就绪'
          : '暂不可申请'
const serverCardTitle = computed(() => {
  if (!currentRequest.value) return '暂无活动服务器'
  if (['ended', 'cancelled', 'abandoned'].includes(currentRequest.value.state)) return '本局已结束'
  return party.value ? '队伍服务器' : '单人服务器'
})
const serverCardDescription = computed(() => {
  if (!currentRequest.value) return party.value
    ? '队长可以到“服务器”页面选择地图与玩法并申请。'
    : '加入或创建队伍后，这里会显示共同的服务器状态。'
  return state.value.phase === 'ready'
    ? '连接方式已准备好，前往服务器页面查看并复制命令。'
    : state.value.description
})
const maintenance = computed(() => catalog.value ? maintenanceFor(catalog.value, gameId.value, presetId.value) : '')
const isLeader = computed(() => !party.value || party.value.currentRole === 'leader')
const partyOverPreset = computed(() => !!party.value && !!preset.value && party.value.members.length > preset.value.maxPlayers)
const blockingParty = computed(() => !!party.value && !!currentRequest.value &&
  ['waiting', 'allocating', 'creating', 'running', 'stopping', 'failed_unreclaimed', 'quarantined'].includes(currentRequest.value.state))
const inviteLink = computed(() => invite.value ? partyInviteLink(window.location.origin, invite.value.token) : '')
const canSubmit = computed(() => !!gameId.value && !!presetId.value && !maintenance.value && !busy.value && isLeader.value && !partyOverPreset.value &&
  (nodeSelectionMode.value === 'auto' || !!manualNodeId.value))

function describeError(cause: unknown): string {
  if (cause instanceof ApiError) return cause.message.trim() || `服务暂时不可用 (${cause.status})`
  return '网络暂时不可用。当前申请仍会保留，请刷新或稍后重试。'
}

async function ensureSession(): Promise<void> {
  try {
    const me = await api.me()
    userId.value = me.userId
    displayName.value = me.displayName
  } catch (cause) {
    if (cause instanceof ApiError && cause.status === 401) {
      const me = await api.session()
      userId.value = me.userId
      displayName.value = me.displayName
    }
    else throw cause
  }
}

async function refreshParty(): Promise<void> {
  party.value = await api.party()
  if (party.value?.currentRole === 'leader') {
    if (!invite.value || invite.value.partyId !== party.value.id) invite.value = await api.currentInvite()
  } else invite.value = null
}

async function refresh(): Promise<void> {
  if (polling || busy.value) return
  polling = true
  let observedAtMs = Date.now()
  const observe = (serverMs: number) => { observedAtMs = serverMs }
  try {
    await refreshParty()
    let request = await api.current(observe)
    if (!request) {
      const lastId = localStorage.getItem(savedRequestKey)
      if (lastId) {
        try { request = await api.getRequest(lastId, observe) }
        catch (cause) { if (cause instanceof ApiError && cause.status === 404) localStorage.removeItem(savedRequestKey); else throw cause }
      }
    }
    currentRequest.value = request
    const choiceGame = request?.arcadeGameId || gameId.value
    const choicePreset = request?.gamePresetId || presetId.value
    if (choiceGame && choicePreset) nodes.value = await api.nodes(choiceGame, choicePreset)
    if (request) {
      localStorage.setItem(savedRequestKey, request.id)
      allocation.value = await api.allocation(request.id, observe)
      nextGameIntent.value = await api.nextGameIntent(request.id)
    } else { allocation.value = null; nextGameIntent.value = null }
    observedServerMs.value = request?.id === elapsedRequestId ? Math.max(observedServerMs.value, observedAtMs) : observedAtMs
    elapsedRequestId = request?.id || ''
    error.value = ''
  } catch (cause) { error.value = describeError(cause) }
  finally { polling = false }
}

async function start(): Promise<void> {
  if (!canSubmit.value) return
  busy.value = true
  error.value = ''
  try {
    const request = await api.createRequest(gameId.value, presetId.value, nodeSelectionMode.value,
      nodeSelectionMode.value === 'manual' ? manualNodeId.value : '')
    currentRequest.value = request
    allocation.value = null
    localStorage.setItem(savedRequestKey, request.id)
  } catch (cause) {
    const submissionError = describeError(cause)
    // A lost POST response may still have created the request.
    busy.value = false
    await refresh()
    if (!currentRequest.value) error.value = submissionError
    return
  }
  busy.value = false
  await refresh()
}

async function stop(): Promise<void> {
  const request = currentRequest.value
  if (!request || request.state !== 'running' || busy.value || !isLeader.value) return
  busy.value = true
  error.value = ''
  try { currentRequest.value = await api.stop(request.id) }
  catch (cause) { error.value = describeError(cause) }
  finally { busy.value = false }
  await refresh()
}

async function nextGame(): Promise<void> {
  const request = currentRequest.value
  if (!request || request.state !== 'running' || busy.value || !isLeader.value) return
  busy.value = true
  error.value = ''
  try { currentRequest.value = await api.nextGame(request.id) }
  catch (cause) { error.value = describeError(cause) }
  finally { busy.value = false }
  await refresh()
}

async function abandonQuarantined(): Promise<void> {
  const request = currentRequest.value
  const recovered = allocation.value?.state === 'reclaimed' && nextGameIntent.value?.state === 'paused'
  if (!request || request.state !== 'quarantined' || busy.value || !isLeader.value ||
    !window.confirm(recovered
      ? '旧服务器已完整回收。确定继续下一局并创建一份新申请吗？新申请会按当前时间正常排队。'
      : '确定放弃此异常服务器并继续？旧 Dota 可能仍在运行，原节点容量不会释放；管理员之后仍需确认并清理旧资源。')) return
  busy.value = true
  error.value = ''
  try {
    await api.abandon(request.id)
    localStorage.removeItem(savedRequestKey)
    currentRequest.value = null
    allocation.value = null
  } catch (cause) { error.value = describeError(cause) }
  finally { busy.value = false }
  await refresh()
}

async function cancelWaiting(): Promise<void> {
  const request = currentRequest.value
  if (!request || request.state !== 'waiting' || allocation.value || busy.value || !isLeader.value) return
  busy.value = true
  error.value = ''
  try { currentRequest.value = await api.cancel(request.id) }
  catch (cause) { error.value = describeError(cause) }
  finally { busy.value = false }
  await refresh()
}

async function partyAction(action: () => Promise<unknown>, notice: string): Promise<void> {
  if (busy.value) return
  busy.value = true
  error.value = ''
  partyNotice.value = ''
  let actionError = ''
  try {
    await action()
    partyNotice.value = notice
    invite.value = null
  } catch (cause) { actionError = describeError(cause) }
  finally { busy.value = false }
  await refresh()
  if (actionError) error.value = actionError
}

async function joinByInvite(): Promise<void> {
  const token = tokenFromInviteInput(inviteToken.value)
  if (!token) { error.value = '请粘贴有效的邀请链接。'; return }
  await partyAction(async () => {
    await api.joinParty(token)
    inviteToken.value = ''
    window.history.replaceState(null, '', window.location.pathname)
  }, '已加入队伍。')
}

async function resetInvite(): Promise<void> {
  if (busy.value || !window.confirm('确定重置邀请链接？旧链接会立即失效，已加入的成员不受影响。')) return
  busy.value = true
  error.value = ''
  try { invite.value = await api.resetInvite(); partyNotice.value = '邀请链接已重置，旧链接已失效。' }
  catch (cause) { error.value = describeError(cause) }
  finally { busy.value = false }
}

async function copyInvite(): Promise<void> {
  if (!inviteLink.value) return
  try {
    await navigator.clipboard.writeText(inviteLink.value)
    copiedInvite.value = true
    window.setTimeout(() => { copiedInvite.value = false }, 2500)
  } catch { error.value = '复制失败。请手动选中并复制邀请链接。' }
}

async function removeMember(member: Party['members'][number]): Promise<void> {
  if (!window.confirm(`确定将「${member.displayName}」移出队伍？若已有运行中的服务器，它会继续运行；此操作不会将其踢出 Dota。`)) return
  await partyAction(() => api.removeMember(member.userId), `已将「${member.displayName}」移出队伍；若已有运行中的服务器，它会继续运行。`)
}

async function leaveParty(): Promise<void> {
  if (!window.confirm('确定退出队伍？若已有运行中的服务器，它会继续运行；退队不会将你踢出已经进入的 Dota 游戏。')) return
  await partyAction(() => api.leaveParty(), '已退出队伍；若已有运行中的服务器，它会继续运行。')
}

async function disband(): Promise<void> {
  if (!party.value || blockingParty.value || !window.confirm('确定解散队伍？所有成员将退出，邀请链接立即失效。')) return
  await partyAction(() => api.disbandParty(), '队伍已解散。')
}

async function copyConnect(): Promise<void> {
  const command = allocation.value?.joinInfo?.connectCommand
  if (!command) return
  try {
    await navigator.clipboard.writeText(command)
    copied.value = true
    window.setTimeout(() => { copied.value = false }, 2500)
  } catch { error.value = '复制失败。请手动选中并复制下方命令。' }
}

async function copyProtocol(uri: string, scheme: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(uri)
    copiedProtocol.value = scheme
    window.setTimeout(() => { copiedProtocol.value = '' }, 2500)
  } catch { error.value = '复制入口链接失败。请手动复制下方链接。' }
}

async function copyConsoleOption(): Promise<void> {
  try {
    await navigator.clipboard.writeText('-console')
    copiedConsole.value = true
    window.setTimeout(() => { copiedConsole.value = false }, 2500)
  } catch { error.value = '复制启动项失败。请手动选中并复制 -console。' }
}

function newRequest(): void {
  if (!currentRequest.value || !['ended', 'cancelled', 'abandoned'].includes(currentRequest.value.state)) return
  currentRequest.value = null
  allocation.value = null
  nextGameIntent.value = null
  localStorage.removeItem(savedRequestKey)
  error.value = ''
}

function syncInviteHash(): void {
  const token = inviteTokenFromHash(window.location.hash)
  if (token && !party.value) {
    inviteToken.value = token
    partyView.value = true
    error.value = ''
  }
}

onMounted(async () => {
  window.addEventListener('hashchange', syncInviteHash)
  try {
    await ensureSession()
    catalog.value = await api.catalog()
    gameId.value = catalog.value.games[0]?.id || ''
    presetId.value = catalog.value.presets.find((item) => item.arcadeGameId === gameId.value)?.id || ''
    await refresh()
  } catch (cause) { error.value = describeError(cause) }
  finally { loading.value = false }
  timer = window.setInterval(refresh, 2500)
})
onUnmounted(() => { if (timer) window.clearInterval(timer); window.removeEventListener('hashchange', syncInviteHash) })
</script>

<template>
  <header class="site-header">
    <div class="header-inner">
      <div class="brand"><span class="brand-mark" aria-hidden="true"><i/><i/><i/><i/></span><span>Dota 2 <strong>游廊联机</strong></span></div>
      <span class="identity"><span class="identity-dot"/> {{ displayName || '匿名玩家' }}<span class="identity-note">· 此浏览器会保留申请</span></span>
    </div>
  </header>

  <main>
    <div v-if="error" class="notice notice-error" role="alert">{{ error }}</div>
    <div v-if="catalog?.siteAnnouncement" class="notice notice-announcement" role="status">
      <strong>站点公告</strong><span>{{ catalog.siteAnnouncement }}</span>
    </div>
    <div v-if="catalog && !catalog.globalAcceptingNewRequests" class="notice notice-maintenance" role="status">
      <strong>平台维护</strong><span>{{ catalog.globalMaintenanceMessage || '暂不接受新申请。' }}</span>
    </div>

    <header class="page-heading page-heading-with-nav">
      <h1>{{ partyView ? '我的队伍' : (currentRequest ? '当前服务器' : '申请服务器') }}</h1>
      <nav class="page-tabs" aria-label="玩家页面"><button type="button" :class="{ selected: !partyView }" @click="partyView = false">服务器</button><button type="button" :class="{ selected: partyView }" @click="partyView = true">我的队伍<span v-if="party" class="tab-count">{{ party.members.length }}</span></button></nav>
    </header>

    <div v-if="loading" class="panel loading-panel" role="status">正在恢复你的会话与申请…</div>
    <div v-else-if="!catalog" class="panel loading-panel">页面暂时无法连接平台。刷新后会继续恢复当前申请。</div>
    <div v-else-if="partyView" class="party-layout">
      <div class="party-main-column">
        <section v-if="!party" class="panel party-card">
          <span class="eyebrow">当前队伍</span><h2>还没有加入队伍</h2>
          <p class="party-lead">你可以单人申请服务器，也可以创建一个会长期保留的队伍。</p>
          <button type="button" class="primary-button party-create" :disabled="busy" @click="partyAction(() => api.createParty(), '队伍已创建，可以邀请朋友加入。')">创建队伍 <span aria-hidden="true">↗</span></button>
          <div class="party-join-form"><label for="invite-token">通过邀请链接加入</label><div><input id="invite-token" v-model="inviteToken" type="text" autocomplete="off" placeholder="粘贴队伍邀请链接" /><button type="button" class="secondary-button" :disabled="busy || !inviteToken.trim()" @click="joinByInvite">加入队伍</button></div><small>邀请链接由队长私下分享；每个匿名用户同一时间只能加入一个队伍。</small></div>
        </section>
        <section v-else class="panel party-card">
          <div class="party-card-heading"><div><span class="eyebrow">当前队伍</span><h2>当前成员</h2></div><span class="party-count">{{ party.members.length }} / {{ party.maxSize }} 人</span></div>
          <p class="party-lead">队伍会保留；结束一局服务器不会解散队伍。{{ party.currentRole === 'leader' ? '你是队长，可以申请和结束服务器。' : '你是队员，可查看当前服务器并随时退出。' }}</p>
          <div class="party-members" aria-label="队伍成员"><div v-for="member in party.members" :key="member.userId" class="party-member"><span class="party-avatar">{{ Array.from(member.displayName)[0] }}</span><span class="party-member-name"><strong>{{ member.displayName }}<span v-if="member.userId === userId" class="party-self">（你）</span></strong><small>{{ member.role === 'leader' ? '队长' : '队员' }}</small></span><span class="party-role">{{ member.role === 'leader' ? '队长' : '队员' }}</span><button v-if="party.currentRole === 'leader' && member.role !== 'leader'" type="button" class="party-text-button warm" :disabled="busy" @click="removeMember(member)">移除</button></div></div>
          <div class="party-manage"><button v-if="party.currentRole === 'member'" type="button" class="secondary-button" :disabled="busy" @click="leaveParty">退出队伍</button><button v-else type="button" class="secondary-button danger" :disabled="busy || blockingParty" @click="disband">解散队伍</button><span v-if="party.currentRole === 'leader' && blockingParty">当前有活动申请或服务器，完整回收后才可解散；V1 不支持队长退队或转让。</span></div>
        </section>
        <section v-if="party?.currentRole === 'leader' && invite" class="panel party-card party-invite-card">
          <span class="eyebrow">邀请朋友</span><h2>分享队伍邀请</h2><p class="party-lead">复制链接发给朋友。重置后旧链接立即失效；活动服务器期间也可以邀请新成员，直到达到队伍人数上限。</p>
          <div class="party-link"><code>{{ inviteLink }}</code><button type="button" class="secondary-button" @click="copyInvite">{{ copiedInvite ? '已复制' : '复制链接' }}</button></div>
          <button type="button" class="party-text-button" :disabled="busy" @click="resetInvite">重置邀请链接</button>
        </section>
      </div>
      <aside class="panel party-card party-server-card"><div class="party-card-heading"><span class="eyebrow">当前服务器</span><span class="party-pill">{{ state.title }}</span></div><h2>{{ serverCardTitle }}</h2><p class="party-lead">{{ serverCardDescription }}</p><p v-if="elapsed" class="elapsed-line">{{ elapsed.summary }}</p><p v-if="elapsed?.detail" class="elapsed-detail">{{ elapsed.detail }}</p><div v-if="currentRequest" class="party-facts"><div><span>地图</span><strong>{{ game?.displayName || '加载中' }}</strong></div><div><span>玩法</span><strong>{{ preset?.displayName || '加载中' }}</strong></div><div><span>节点</span><strong>{{ assignedNodeName }}</strong></div></div><button type="button" class="secondary-button" @click="partyView = false">{{ allocation?.joinInfo ? '查看连接方式' : '前往服务器页面' }}</button></aside>
      <p v-if="partyNotice" class="party-notice" role="status">{{ partyNotice }}</p>
    </div>
    <div v-else-if="!currentRequest" class="request-layout">
      <div class="selection-stack">
        <section class="panel section-panel">
          <div class="section-heading"><div><span class="eyebrow">01 / 选择内容</span><h2>全部地图</h2></div></div>
          <div class="game-grid">
            <label v-for="(item, index) in catalog.games" :key="item.id" class="game-card" :class="{ selected: gameId === item.id, disabled: !item.acceptingNewRequests }">
              <input v-model="gameId" type="radio" name="game" :value="item.id" @change="presetId = catalog.presets.find(p => p.arcadeGameId === item.id)?.id || ''" />
              <span class="game-art" aria-hidden="true"><small>{{ String(index + 1).padStart(2, '0') }}</small><b>✦</b></span>
              <span class="game-name"><strong>{{ item.displayName }}</strong><small>Workshop {{ item.workshopId }}</small><span class="card-pill">{{ item.acceptingNewRequests ? (gameId === item.id ? '已选择' : '可申请') : '维护中' }}</span></span>
            </label>
          </div>
          <p v-if="game && !game.acceptingNewRequests" class="inline-maintenance">{{ game.maintenanceMessage || '这张地图正在维护。' }}</p>
        </section>

        <section class="panel section-panel">
          <div class="section-heading"><div><span class="eyebrow">02 / 选择玩法</span><h2>游戏模式</h2></div></div>
          <div class="preset-grid">
            <label v-for="item in selectedPresets" :key="item.id" class="preset-card" :class="{ selected: presetId === item.id }">
              <input v-model="presetId" type="radio" name="preset" :value="item.id" />
              <span class="preset-main"><strong>{{ item.displayName }}</strong><span class="radio-dot"/></span>
              <small>最多 {{ item.maxPlayers }} 人</small>
              <span v-if="!item.acceptingNewRequests" class="preset-maintenance">{{ item.maintenanceMessage || '维护中' }}</span>
            </label>
          </div>
        </section>
      </div>

      <aside class="application-sidebar" aria-label="申请摘要">
        <section class="panel owner-panel"><span class="eyebrow">当前申请人</span><div class="owner-line"><span class="owner-avatar" aria-hidden="true">{{ party ? '队' : '我' }}</span><span><strong>{{ party ? '当前队伍' : '仅自己' }}</strong><small>{{ party ? (party.currentRole === 'leader' ? '你是队长 · 可申请' : '你是队员 · 只读状态') : '匿名玩家 · 单人申请' }}</small></span><span class="owner-count">{{ party?.members.length || 1 }} 人</span></div></section>
        <section class="panel apply-panel">
          <span class="eyebrow">03 / 申请服务器</span>
          <h2>申请确认</h2>
          <p>平台会检查节点内容和空位，再分配服务器。</p>
          <div class="summary-row"><span>地图</span><strong>{{ game?.displayName || '未选择' }}</strong></div>
          <div class="summary-row"><span>玩法</span><strong>{{ preset?.displayName || '未选择' }}</strong></div>
          <div class="summary-row"><span>人数上限</span><strong>{{ preset?.maxPlayers ?? '—' }} 人</strong></div>
          <fieldset class="node-mode-group">
            <legend>节点选择方式</legend>
            <label class="node-mode-choice" :class="{ selected: nodeSelectionMode === 'auto' }"><input v-model="nodeSelectionMode" type="radio" name="node-mode" value="auto" @change="manualNodeId = ''" /><span><strong>自动选择</strong><small>平台从当前可用节点分配</small></span></label>
            <label class="node-mode-choice" :class="{ selected: nodeSelectionMode === 'manual' }"><input v-model="nodeSelectionMode" type="radio" name="node-mode" value="manual" /><span><strong>手动指定</strong><small>只等待选定节点，不自动切换</small></span></label>
          </fieldset>
          <div v-if="nodeSelectionMode === 'manual'" class="manual-node-panel">
            <p class="manual-node-heading">选择要等待的节点</p>
            <div class="node-list"><label v-for="node in nodes" :key="node.id" :class="{ selected: manualNodeId === node.id, disabled: !node.selectable }"><input v-model="manualNodeId" type="radio" name="manual-node" :value="node.id" :disabled="!node.selectable" /><span class="node-state" :class="node.status" aria-hidden="true" /><span><strong>{{ node.displayName }}</strong><small>{{ nodeStatus(node) }}</small></span></label><p v-if="nodes.length === 0">暂无已登记节点，请改用自动选择并等待。</p></div>
            <p class="manual-node-hint">满载、维护或暂不可达时，这份申请会继续等待所选节点。</p>
          </div>
          <p class="node-selection-summary" role="status">本次分配：<strong>{{ selectionSummary }}</strong></p>
          <p v-if="maintenance" class="maintenance-callout" role="status">{{ maintenance }}</p>
          <p v-if="partyOverPreset" class="maintenance-callout" role="status">当前队伍 {{ party?.members.length }} 人，超过此玩法最多 {{ preset?.maxPlayers }} 人；选择其他玩法后再申请。</p>
          <p v-if="party && !isLeader" class="maintenance-callout" role="status">只有队长可以申请服务器。队员可查看状态与连接信息。</p>
          <button class="primary-button" type="button" :disabled="!canSubmit" @click="start">{{ busy ? '正在提交…' : '申请服务器' }} <span aria-hidden="true">↗</span></button>
          <p class="footnote">{{ nodeSelectionMode === 'manual' ? '改选节点需在安全等待阶段取消旧申请，再提交新申请。' : '已有活动申请时，会优先显示当前状态。' }}</p>
        </section>
      </aside>
    </div>

    <div v-else class="active-layout">
      <section class="panel status-panel" :class="`phase-${state.phase}`">
        <div class="status-top"><span class="eyebrow">当前申请</span><span class="status-pill"><span class="status-dot"/>{{ state.title }}</span></div>
        <h2>{{ state.title }}</h2>
        <p class="status-description">{{ state.description }}</p>
        <p v-if="elapsed" class="elapsed-line">{{ elapsed.summary }}</p>
        <p v-if="elapsed?.detail" class="elapsed-detail">{{ elapsed.detail }}</p>
        <div class="facts" aria-label="申请内容">
          <div><span>游廊地图</span><strong>{{ game?.displayName || '加载中' }}</strong></div>
          <div><span>游戏模式</span><strong>{{ preset?.displayName || '加载中' }}</strong></div>
          <div><span>申请人</span><strong>{{ party ? `当前队伍 · ${party.members.length} 人` : '仅自己' }}</strong></div>
          <div><span>服务器节点</span><strong>{{ assignedNodeName }}</strong></div>
        </div>
        <p v-if="currentRequest.state === 'waiting'" class="queue-note">{{ selectionName }} · 申请时间 {{ new Date(currentRequest.requestedAt).toLocaleString('zh-CN') }}。等待不会计入服务器启动时间。</p>
        <ol class="progress" aria-label="开服进度">
          <li :class="{ active: state.phase === 'waiting', done: ['allocating', 'creating', 'ready', 'unavailable', 'stopping', 'ended'].includes(state.phase) }"><span>01</span>等待资源</li>
          <li :class="{ active: ['allocating', 'creating'].includes(state.phase), done: ['ready', 'unavailable', 'stopping', 'ended'].includes(state.phase) }"><span>02</span>分配并启动</li>
          <li :class="{ active: ['ready', 'unavailable'].includes(state.phase), done: ['stopping', 'ended'].includes(state.phase) && !!allocation?.joinInfoAvailableAt }"><span>03</span>连接服务器</li>
        </ol>
        <p v-if="allocation?.errorCode || allocation?.joinInfoErrorCode" class="diagnostic">诊断代码：{{ allocation.joinInfoErrorCode || allocation.errorCode }}</p>
        <div class="status-actions">
          <button v-if="currentRequest.state === 'waiting' && !allocation && isLeader" type="button" class="secondary-button" :disabled="busy" @click="cancelWaiting">取消等待申请</button>
          <button v-if="currentRequest.state === 'running' && isLeader" type="button" class="secondary-button danger" :disabled="busy" @click="stop">{{ busy ? '正在提交…' : '结束服务器' }}</button>
          <button v-if="currentRequest.state === 'running' && isLeader" type="button" class="secondary-button next-game-button" :disabled="busy" @click="nextGame">下一局 <span aria-hidden="true">→</span></button>
          <button v-if="currentRequest.state === 'quarantined' && isLeader" type="button" class="secondary-button danger" :disabled="busy" @click="abandonQuarantined">{{ allocation?.state === 'reclaimed' && nextGameIntent?.state === 'paused' ? '继续下一局' : '放弃此异常服务器并继续' }}</button>
          <p v-if="currentRequest.state === 'quarantined' && !isLeader" class="action-hint">只有队长可以放弃异常服务器；旧资源仍需管理员处理。</p>
          <button v-if="state.phase === 'ended'" type="button" class="secondary-button" @click="newRequest">重新申请</button>
        </div>
      </section>

      <aside v-if="state.phase === 'ready' && allocation?.joinInfo" class="panel join-panel">
        <span class="eyebrow">服务器已就绪</span><h2>进入游戏</h2>
        <p>{{ allocation.joinInfo.steamUri || allocation.joinInfo.steamChinaUri ? '可以尝试已开放的一键入口，也可以在 Dota 2 控制台输入下方命令。' : '在 Dota 2 控制台输入下方命令，即可连接当前服务器。' }}</p>
        <div v-if="allocation.joinInfo.steamUri || allocation.joinInfo.steamChinaUri" class="protocol-entry">
          <div v-if="allocation.joinInfo.steamUri" class="protocol-entry-row"><a class="protocol-entry-button" :href="allocation.joinInfo.steamUri">通过 Steam 进入</a><button type="button" class="inline-button" @click="copyProtocol(allocation.joinInfo.steamUri, 'steam')">{{ copiedProtocol === 'steam' ? '已复制链接' : '复制链接' }}</button><code>{{ allocation.joinInfo.steamUri }}</code></div>
          <div v-if="allocation.joinInfo.steamChinaUri" class="protocol-entry-row"><a class="protocol-entry-button" :href="allocation.joinInfo.steamChinaUri">通过蒸汽平台进入</a><button type="button" class="inline-button" @click="copyProtocol(allocation.joinInfo.steamChinaUri, 'steamchina')">{{ copiedProtocol === 'steamchina' ? '已复制链接' : '复制链接' }}</button><code>{{ allocation.joinInfo.steamChinaUri }}</code></div>
        </div>
        <p class="protocol-help">一键入口没有反应？先启动对应客户端再试；Windows 可复制上方链接，按 <kbd>Win+R</kbd> 粘贴执行。仍无法进入时，使用下方手动连接命令。</p>
        <div class="connect-box"><div><span>手动连接</span><code>{{ allocation.joinInfo.connectCommand }}</code></div><button type="button" class="copy-button" @click="copyConnect">{{ copied ? '已复制' : '复制命令' }}</button></div>
        <details class="help"><summary>如何启用并打开 Dota 2 控制台 <span aria-hidden="true">⌄</span></summary>
          <ol>
            <li>在 Steam 游戏库中打开 Dota 2「属性 → 启动选项」，填入 <span class="help-copy-pair"><code>-console</code><button type="button" class="inline-button" @click="copyConsoleOption">{{ copiedConsole ? '已复制' : '复制启动项' }}</button></span>，然后重启 Dota 2。</li>
            <li>可先按默认键 <kbd>&#92;</kbd> 打开控制台；如果没有反应，请在 Dota 2 的按键设置中查看或重新绑定控制台热键，以实际设置为准。</li>
            <li>点击上方“复制命令”，在控制台粘贴，按回车。等待游戏载入服务器。</li>
          </ol>
        </details>
        <p class="join-note">结束服务器会停止当前游戏并等待完整回收。</p>
      </aside>
      <aside v-else-if="state.phase === 'unavailable'" class="panel side-panel"><span class="eyebrow">连接状态</span><h2>暂时无法给出连接地址</h2><p>服务器仍在运行。当前端口映射信息不足或已变化，页面不会显示未经确认的命令。你可以稍后刷新或结束服务器。</p></aside>
      <aside v-else-if="state.phase === 'quarantined'" class="panel side-panel"><span class="eyebrow">{{ allocation?.state === 'reclaimed' ? '资源已回收' : '异常隔离' }}</span><h2>{{ allocation?.state === 'reclaimed' ? '下一局等待你继续' : '旧资源仍被保留' }}</h2><p>{{ allocation?.state === 'reclaimed' ? '旧服务器已完成回收。队长确认后才会创建下一局的新申请，并按新申请时间正常排队。' : '放弃后只解除申请阻塞，不代表旧服务器已停止、端口空闲或容量释放。管理员确认完整回收后才会释放原节点资源。' }}</p></aside>
      <aside v-else-if="state.phase === 'unknown'" class="panel side-panel"><span class="eyebrow">等待对账</span><h2>不会重复开服</h2><p>节点恢复后，平台会核对原来的任务和实例。请稍后回来查看；当前申请和容量仍被保留。</p></aside>
      <aside v-else-if="state.phase === 'ended'" class="panel side-panel"><span class="eyebrow">{{ currentRequest.state === 'cancelled' ? '申请已取消' : '本局已结束' }}</span><h2>{{ currentRequest.state === 'cancelled' ? '未占用服务器' : currentRequest.state === 'abandoned' ? '旧资源仍待清理' : '服务器已回收' }}</h2><p>{{ currentRequest.state === 'abandoned' ? '旧异常服务器仍占用原节点容量。你可以新建申请，管理员会另行处理旧资源。' : '点击左侧“重新申请”即可回到地图、玩法和节点选择。刷新页面仍能看到此次申请的结束状态。' }}</p></aside>
      <aside v-else class="panel side-panel"><span class="eyebrow">申请已保存</span><h2>可以稍后回来</h2><p>关闭浏览器或刷新页面后，这台服务器的状态仍会保留在当前匿名会话中。</p></aside>
    </div>
  </main>
  <footer><span>Dota 2 Arcade Platform</span><span>玩家专服</span></footer>
</template>

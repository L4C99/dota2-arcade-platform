<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ApiError, api } from './api'
import type { Allocation, Catalog, ServerRequest } from './api'
import { maintenanceFor, statusFor } from './state'

const savedRequestKey = 'arcade.lastRequestId'
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const copied = ref(false)
const copiedConsole = ref(false)
const catalog = ref<Catalog | null>(null)
const currentRequest = ref<ServerRequest | null>(null)
const allocation = ref<Allocation | null>(null)
const gameId = ref('')
const presetId = ref('')
let timer: number | undefined
let polling = false

const game = computed(() => catalog.value?.games.find((item) => item.id === (currentRequest.value?.arcadeGameId || gameId.value)))
const preset = computed(() => catalog.value?.presets.find((item) => item.id === (currentRequest.value?.gamePresetId || presetId.value)))
const selectedPresets = computed(() => catalog.value?.presets.filter((item) => item.arcadeGameId === gameId.value) || [])
const state = computed(() => statusFor(currentRequest.value, allocation.value))
const maintenance = computed(() => catalog.value ? maintenanceFor(catalog.value, gameId.value, presetId.value) : '')
const canSubmit = computed(() => !!gameId.value && !!presetId.value && !maintenance.value && !busy.value)

function describeError(cause: unknown): string {
  if (cause instanceof ApiError) return cause.message.trim() || `服务暂时不可用 (${cause.status})`
  return '网络暂时不可用。当前申请仍会保留，请刷新或稍后重试。'
}

async function ensureSession(): Promise<void> {
  try {
    await api.me()
  } catch (cause) {
    if (cause instanceof ApiError && cause.status === 401) await api.session()
    else throw cause
  }
}

async function refresh(): Promise<void> {
  if (polling || busy.value) return
  polling = true
  try {
    let request = await api.current()
    if (!request) {
      const lastId = localStorage.getItem(savedRequestKey)
      if (lastId) {
        try { request = await api.getRequest(lastId) }
        catch (cause) { if (cause instanceof ApiError && cause.status === 404) localStorage.removeItem(savedRequestKey); else throw cause }
      }
    }
    currentRequest.value = request
    if (request) {
      localStorage.setItem(savedRequestKey, request.id)
      allocation.value = await api.allocation(request.id)
    } else allocation.value = null
    error.value = ''
  } catch (cause) { error.value = describeError(cause) }
  finally { polling = false }
}

async function start(): Promise<void> {
  if (!canSubmit.value) return
  busy.value = true
  error.value = ''
  try {
    const request = await api.createRequest(gameId.value, presetId.value)
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
  if (!request || request.state !== 'running' || busy.value) return
  busy.value = true
  error.value = ''
  try { currentRequest.value = await api.stop(request.id) }
  catch (cause) { error.value = describeError(cause) }
  finally { busy.value = false }
  await refresh()
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

async function copyConsoleOption(): Promise<void> {
  try {
    await navigator.clipboard.writeText('-console')
    copiedConsole.value = true
    window.setTimeout(() => { copiedConsole.value = false }, 2500)
  } catch { error.value = '复制启动项失败。请手动选中并复制 -console。' }
}

function newRequest(): void {
  if (currentRequest.value?.state !== 'ended' && currentRequest.value?.state !== 'cancelled') return
  currentRequest.value = null
  allocation.value = null
  localStorage.removeItem(savedRequestKey)
  error.value = ''
}

onMounted(async () => {
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
onUnmounted(() => { if (timer) window.clearInterval(timer) })
</script>

<template>
  <header class="site-header">
    <div class="header-inner">
      <div class="brand"><span class="brand-mark" aria-hidden="true"><i/><i/><i/><i/></span><span>Dota 2 <strong>游廊联机</strong></span></div>
      <span class="identity"><span class="identity-dot"/> 匿名玩家 · 此浏览器会保留申请</span>
    </div>
  </header>

  <main>
    <div v-if="error" class="notice notice-error" role="alert">{{ error }}</div>
    <div v-if="catalog && !catalog.globalAcceptingNewRequests" class="notice notice-maintenance" role="status">
      <strong>平台维护</strong><span>{{ catalog.globalMaintenanceMessage || '暂不接受新申请。' }}</span>
    </div>

    <section class="intro">
      <div class="intro-index"><span/> YOUR PRIVATE SERVER <span>01 / 01</span></div>
      <h1>申请一台<br><em>Dota 2 游廊专服</em></h1>
      <p>选择玩法，等待启动，再用页面提供的命令进入游戏。</p>
    </section>

    <div v-if="loading" class="panel loading-panel" role="status">正在恢复你的会话与申请…</div>
    <div v-else-if="!catalog" class="panel loading-panel">页面暂时无法连接平台。刷新后会继续恢复当前申请。</div>
    <div v-else-if="!currentRequest" class="request-layout">
      <div class="selection-stack">
        <section class="panel section-panel">
          <div class="section-heading"><div><span class="eyebrow">01 / 选择内容</span><h2>游廊地图</h2></div><span class="section-meta">当前开放</span></div>
          <div class="game-grid">
            <label v-for="(item, index) in catalog.games" :key="item.id" class="game-card" :class="{ selected: gameId === item.id, disabled: !item.acceptingNewRequests }">
              <input v-model="gameId" type="radio" name="game" :value="item.id" @change="presetId = catalog.presets.find(p => p.arcadeGameId === item.id)?.id || ''" />
              <span class="game-art" aria-hidden="true"><small>{{ String(index + 1).padStart(2, '0') }}</small><b>✦</b></span>
              <span class="game-name"><strong>{{ item.displayName }}</strong><small>Workshop {{ item.workshopId }}</small></span>
              <span class="card-pill">{{ item.acceptingNewRequests ? (gameId === item.id ? '已选择' : '可申请') : '维护中' }}</span>
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

      <aside class="panel apply-panel">
        <span class="eyebrow">03 / 申请服务器</span>
        <h2>开一局吧</h2>
        <p>当前仅开放一张地图和一个玩法。系统会为你确认开发节点的内容与空位。</p>
        <div class="summary-row"><span>地图</span><strong>{{ game?.displayName || '未选择' }}</strong></div>
        <div class="summary-row"><span>玩法</span><strong>{{ preset?.displayName || '未选择' }}</strong></div>
        <div class="summary-row"><span>人数上限</span><strong>{{ preset?.maxPlayers ?? '—' }} 人</strong></div>
        <p v-if="maintenance" class="maintenance-callout" role="status">{{ maintenance }}</p>
        <button class="primary-button" type="button" :disabled="!canSubmit" @click="start">{{ busy ? '正在提交…' : '申请服务器' }} <span aria-hidden="true">↗</span></button>
        <p class="footnote">重复点击或刷新页面会恢复同一条活动申请。</p>
      </aside>
    </div>

    <div v-else class="active-layout">
      <section class="panel status-panel" :class="`phase-${state.phase}`">
        <div class="status-top"><span class="eyebrow">当前申请</span><span class="status-pill"><span class="status-dot"/>{{ state.title }}</span></div>
        <h2>{{ state.title }}</h2>
        <p class="status-description">{{ state.description }}</p>
        <div class="facts">
          <div><span>游廊地图</span><strong>{{ game?.displayName || '加载中' }}</strong></div>
          <div><span>游戏模式</span><strong>{{ preset?.displayName || '加载中' }}</strong></div>
          <div><span>服务器节点</span><strong>{{ allocation?.nodeDisplayName || '等待分配' }}</strong></div>
        </div>
        <ol class="progress" aria-label="开服进度">
          <li :class="{ active: state.phase === 'waiting' }"><span>01</span>等待资源</li>
          <li :class="{ active: ['allocating', 'creating'].includes(state.phase) }"><span>02</span>分配并启动</li>
          <li :class="{ active: ['ready', 'unavailable'].includes(state.phase) }"><span>03</span>连接服务器</li>
        </ol>
        <p v-if="allocation?.errorCode || allocation?.joinInfoErrorCode" class="diagnostic">诊断代码：{{ allocation.joinInfoErrorCode || allocation.errorCode }}</p>
        <div class="status-actions">
          <button v-if="currentRequest.state === 'running'" type="button" class="secondary-button danger" :disabled="busy" @click="stop">{{ busy ? '正在提交…' : '结束服务器' }}</button>
          <button v-if="state.phase === 'ended'" type="button" class="secondary-button" @click="newRequest">重新申请</button>
        </div>
      </section>

      <aside v-if="state.phase === 'ready' && allocation?.joinInfo" class="panel join-panel">
        <span class="eyebrow">服务器已就绪</span><h2>进入游戏</h2>
        <p>在 Dota 2 控制台输入这条命令，即可连接当前服务器。</p>
        <div class="connect-box"><span>手动连接命令</span><code>{{ allocation.joinInfo.connectCommand }}</code></div>
        <button type="button" class="primary-button" @click="copyConnect">{{ copied ? '已复制' : '复制 connect 命令' }} <span aria-hidden="true">⧉</span></button>
        <details class="help"><summary>如何启用并打开 Dota 2 控制台 <span aria-hidden="true">⌄</span></summary>
          <ol>
            <li>在 Steam 游戏库中打开 Dota 2「属性 → 启动选项」，填入 <span class="help-copy-pair"><code>-console</code><button type="button" class="inline-button" @click="copyConsoleOption">{{ copiedConsole ? '已复制' : '复制启动项' }}</button></span>，然后重启 Dota 2。</li>
            <li>可先按默认键 <kbd>&#92;</kbd> 打开控制台；如果没有反应，请在 Dota 2 的按键设置中查看或重新绑定控制台热键，以实际设置为准。</li>
            <li>点击上方“复制 connect 命令”，在控制台粘贴，按回车。等待游戏载入服务器。</li>
          </ol>
          <p>若入口链接没有反应，也可以始终使用这条 connect 命令。</p>
        </details>
        <p class="join-note">结束服务器会停止当前游戏并等待完整回收。</p>
      </aside>
      <aside v-else-if="state.phase === 'unavailable'" class="panel side-panel"><span class="eyebrow">连接状态</span><h2>暂时无法给出连接地址</h2><p>服务器仍在运行。当前端口映射信息不足或已变化，页面不会显示未经确认的命令。你可以稍后刷新或结束服务器。</p></aside>
      <aside v-else-if="state.phase === 'ended'" class="panel side-panel"><span class="eyebrow">本局已结束</span><h2>服务器已回收</h2><p>点击左侧“重新申请”即可回到地图和玩法选择。刷新页面仍能看到本局的结束状态。</p></aside>
      <aside v-else class="panel side-panel"><span class="eyebrow">申请已保存</span><h2>可以稍后回来</h2><p>关闭浏览器或刷新页面后，这台服务器的状态仍会保留在当前匿名会话中。</p></aside>
    </div>
  </main>
  <footer><span>Dota 2 Arcade Platform</span><span>玩家专服 · P1</span></footer>
</template>

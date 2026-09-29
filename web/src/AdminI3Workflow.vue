<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'

interface Game { id: string; displayName: string; currentContentVersionId: string }
interface Preset { id: string; arcadeGameId: string; displayName: string; templateRevisionId: string; validationContract: string; acceptingNewRequests: boolean }
interface Node { id: string; displayName: string; enabled: boolean; acceptingNewRequests: boolean; draining: boolean; connectivity: string; compatibility: string; reconcileCompleted: number; capabilities: string[] }
interface Binding { nodeId: string; arcadeGameId: string; reportedState: string; reportedContentVersionId: string; reportedContentSha256: string; acceptingNewAllocations: boolean; maintenanceEpoch: number; contentFactRevision: number }
interface TemplateBinding { nodeId: string; templateRevisionId: string; bindingKey: string; expectedFingerprintSha256: string; bindingGeneration: number; factState: string; factFingerprintSha256: string; templateFactRevision: number }
interface Run { id: string; nodeId: string; gameId: string; presetId: string; contentVersionId: string; templateRevisionId: string; state: string; humanResult: string; effective: boolean; formal: boolean; invalidReason?: string }
interface Release { id: string; gameId: string; oldContentVersionId: string; newContentVersionId: string; rollbackOfReleaseId?: string; publishedAt: string }
interface Overview { games: Game[]; presets: Preset[]; nodes: Node[]; bindings: Binding[]; templateBindings: TemplateBinding[]; contentVersions: { id: string; arcadeGameId: string }[]; templateRevisions: { id: string; arcadeGameId: string }[]; validationRuns: Run[]; releases: Release[]; inventories: { nodeId: string; state: string; unaccountedCount: number; current: boolean; receivedAt: string }[] }
interface Detail { id: string; state: string; humanResult: string; allocationId: string; allocationState: string; createJobId: string; createJobState: string; stopJobId?: string; stopJobState: string; instanceId?: string; connectCommand?: string; effective: boolean; formal: boolean; invalidReason?: string }
interface Plan { selected: boolean; template: string; accepting: boolean; runId: string }
interface ReleaseDetail { id: string; oldContentVersionId: string; newContentVersionId: string; presets: { presetId: string; oldTemplateRevisionId: string }[] }

const props = defineProps<{ overview: Overview; busy: boolean }>()
const emit = defineEmits<{ refresh: [] }>()
const gameId = ref('')
const nodeId = ref('')
const presetId = ref('')
const candidateContent = ref('')
const candidateTemplate = ref('')
const bindingKey = ref('')
const fingerprint = ref('')
const maintenanceReason = ref('content or template validation')
const status = ref<{ maintenanceEpoch: number; closed: boolean; targetOccupied: number; targetUnresolvedJobs: number; nodeOccupied: number } | null>(null)
const detail = ref<Detail | null>(null)
const selectedRunId = ref('')
const rollbackOf = ref('')
const error = ref('')
const notice = ref('')
const working = ref(false)
const requestIds = new Map<string, string>()
const plan = reactive<Record<string, Plan>>({})

const game = computed(() => props.overview.games.find(x => x.id === gameId.value))
const node = computed(() => props.overview.nodes.find(x => x.id === nodeId.value))
const presets = computed(() => props.overview.presets.filter(x => x.arcadeGameId === gameId.value))
const upgraded = computed(() => presets.value.some(x => x.validationContract === 'v1_0_2') || props.overview.releases.some(x => x.gameId === gameId.value))
const binding = computed(() => props.overview.bindings.find(x => x.nodeId === nodeId.value && x.arcadeGameId === gameId.value))
const inventory = computed(() => props.overview.inventories.find(x => x.nodeId === nodeId.value))
const templateBinding = computed(() => props.overview.templateBindings.find(x => x.nodeId === nodeId.value && x.templateRevisionId === candidateTemplate.value))
const gameReleases = computed(() => props.overview.releases.filter(x => x.gameId === gameId.value))
const needsAll = computed(() => !upgraded.value || candidateContent.value !== game.value?.currentContentVersionId)
const planned = computed(() => presets.value.filter(x => plan[x.id]?.selected))
const canPublish = computed(() => !!game.value && !!candidateContent.value && planned.value.length > 0 &&
  (!needsAll.value || planned.value.length === presets.value.length) &&
  (planned.value.some(x => plan[x.id]?.accepting && !!plan[x.id]?.runId) ||
    (needsAll.value && !rollbackOf.value && planned.value.every(x => !plan[x.id]?.accepting))))

function requestId(key: string): string { const existing = requestIds.get(key); if (existing) return existing; const next = crypto.randomUUID(); requestIds.set(key, next); return next }
const hints: Record<string, string> = {
  SCOPED_RESOURCES_ACTIVE: '这张游戏在目标节点仍有实例或未完成任务；等待完整回收。',
  CAPACITY_FULL: '节点没有验证名额；等待容量释放。',
  CONTENT_FACT_MISMATCH: '节点内容版本、SHA 或维护代次与候选不一致。',
  TEMPLATE_FACT_MISMATCH: '模板指纹、绑定代次或 Controller 模板事实不一致。',
  INVENTORY_UNKNOWN: '等待 Controller 完成新鲜、完整的实例清单。',
  UNACCOUNTED_INSTANCE: '节点存在未入账实例，先核对并处理。',
  VALIDATION_INCOMPLETE: '缺少当前有效的逐玩法 PASS；检查维护代次和机器事实。',
  CAS_CONFLICT: '页面状态已过期或请求 ID 被用于不同内容；刷新后重试。',
}
async function call<T>(path: string, body?: unknown): Promise<T> {
  const response = await fetch(`/api/v1/admin${path}`, { method: body === undefined ? 'GET' : 'POST', credentials: 'same-origin',
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) })
  const data = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(`${hints[data.code] || '请求失败，请刷新后重试。'} (${data.code || response.status})`)
  return data as T
}
async function action<T>(path: string, body: unknown, message: string): Promise<T | undefined> {
  working.value = true; error.value = ''; notice.value = ''
  try { const result = await call<T>(path, body); notice.value = message; emit('refresh'); await loadStatus(); return result }
  catch (e) { error.value = e instanceof Error ? e.message : String(e); return undefined }
  finally { working.value = false }
}
async function loadStatus(): Promise<void> {
  if (!nodeId.value || !gameId.value) { status.value = null; return }
  try { status.value = await call(`/maintenance/status?nodeId=${encodeURIComponent(nodeId.value)}&gameId=${encodeURIComponent(gameId.value)}`) }
  catch { status.value = null }
}
async function loadRun(id: string): Promise<void> {
  selectedRunId.value = id; detail.value = null; error.value = ''
  if (!id) return
  try { detail.value = await call<Detail>(`/validation/${encodeURIComponent(id)}`) }
  catch (e) { error.value = e instanceof Error ? e.message : String(e) }
}
watch([gameId, nodeId], () => { void loadStatus(); detail.value = null })
watch(gameId, () => {
  candidateContent.value = game.value?.currentContentVersionId || ''
  presetId.value = presets.value[0]?.id || ''
  rollbackOf.value = ''
  for (const key of Object.keys(plan)) delete plan[key]
  for (const p of presets.value) plan[p.id] = { selected: true, template: p.templateRevisionId, accepting: false, runId: '' }
})
watch(presetId, () => { candidateTemplate.value = presets.value.find(x => x.id === presetId.value)?.templateRevisionId || '' })
watch(templateBinding, value => { bindingKey.value = value?.bindingKey || ''; fingerprint.value = value?.expectedFingerprintSha256 || '' }, { immediate: true })

async function beginMaintenance(): Promise<void> {
  if (!binding.value || !status.value) return
  const body = { nodeId: nodeId.value, gameId: gameId.value, expectedMaintenanceEpoch: status.value.maintenanceEpoch,
    reason: maintenanceReason.value }
  await action('/maintenance/begin', { ...body, requestId: requestId(`maintenance:${JSON.stringify(body)}`) }, '目标游戏的新分配已关闭，维护代次已前进。')
}
async function saveTemplateBinding(): Promise<void> {
  if (!templateBinding.value && !bindingKey.value) return
  await action('/actions', { action: 'template_binding.upsert', targetId: nodeId.value, templateRevisionId: candidateTemplate.value,
    bindingKey: bindingKey.value, expectedTemplateFingerprintSha256: fingerprint.value.toLowerCase(),
    expectedOldBindingGeneration: templateBinding.value?.bindingGeneration || 0 }, '模板绑定已保存；等待 Controller 模板事实。')
}
async function startValidation(): Promise<void> {
  if (!status.value) return
  const body = { nodeId: nodeId.value, gameId: gameId.value, presetId: presetId.value,
    contentVersionId: candidateContent.value, templateRevisionId: candidateTemplate.value,
    expectedMaintenanceEpoch: status.value.maintenanceEpoch, expectedTemplateFingerprint: fingerprint.value.toLowerCase() }
  const run = await action<{ id: string }>('/validation/start', { ...body, requestId: requestId(`validation:${JSON.stringify(body)}`) }, '验证房已预留；等待 Ready 和 JoinInfo。')
  if (run?.id) await loadRun(run.id)
}
async function confirmHuman(result: 'pass' | 'fail'): Promise<void> {
  if (!detail.value) return
  if (!window.confirm(result === 'pass' ? '确认 Owner 已亲自进入这个 Ready 实例并验证玩法？确认后会排入正式 stop，完整回收后才可能 PASS。' : '确认本次真人验证失败并排入正式 stop？')) return
  await action(`/validation/${detail.value.id}/confirm`, { result, confirmed: true, expectedRunState: detail.value.state }, '真人结果已记录；等待 stop 和完整回收。')
  await loadRun(detail.value.id)
}
async function stopValidation(): Promise<void> {
  if (!detail.value || !window.confirm('确认停止这次验证的可信实例？')) return
  await action(`/validation/${detail.value.id}/stop`, { expectedRunState: detail.value.state }, '正式 stop 已排入；等待完整回收。')
  await loadRun(detail.value.id)
}
async function publish(): Promise<void> {
  if (!game.value || !canPublish.value) return
  const body = { gameId: game.value.id, expectedOldContentVersionId: game.value.currentContentVersionId,
    newContentVersionId: candidateContent.value, rollbackOfReleaseId: rollbackOf.value,
    presets: planned.value.map(p => ({ presetId: p.id, expectedOldTemplateRevisionId: p.templateRevisionId,
      newTemplateRevisionId: plan[p.id].template, expectedOldAccepting: p.acceptingNewRequests,
      newAccepting: plan[p.id].accepting, validationRunId: plan[p.id].accepting ? plan[p.id].runId : '' })) }
  await action('/release/publish', { ...body, requestId: requestId(`release:${JSON.stringify(body)}`) }, '正式组合和逐玩法发布记录已原子提交。')
}
async function chooseRollback(id: string): Promise<void> {
  try {
    const release = await call<ReleaseDetail>(`/release/${encodeURIComponent(id)}`)
    rollbackOf.value = id; candidateContent.value = release.oldContentVersionId
    for (const p of presets.value) {
      const target = release.presets.find(x => x.presetId === p.id)
      plan[p.id] = { selected: true, template: target?.oldTemplateRevisionId || p.templateRevisionId, accepting: false, runId: '' }
    }
    notice.value = '已载入回退目标；先恢复节点旧 bytes，并在本次维护代次重新取得逐玩法 PASS。'
  } catch (e) { error.value = e instanceof Error ? e.message : String(e) }
}
</script>

<template>
  <section class="panel admin-card admin-i3">
    <span class="eyebrow">v1.0.2 正式流程</span><h2>逐玩法验证与发布</h2>
    <p>选择游戏和节点后，按 scoped maintenance → 本地准备 → Controller 事实 → 逐玩法真人验证 → 完整回收 → 发布 → 开放执行。刷新页面不会成为证明；每次提交都由服务端重新检查。</p>
    <p v-if="error" class="admin-task-warning" role="alert">{{ error }}</p><p v-if="notice" class="admin-task-done" role="status">{{ notice }}</p>
    <div class="admin-task-form">
      <label>游戏<select v-model="gameId"><option value="">请选择</option><option v-for="g in overview.games" :key="g.id" :value="g.id">{{ g.displayName }}</option></select></label>
      <label>节点<select v-model="nodeId"><option value="">请选择</option><option v-for="n in overview.nodes" :key="n.id" :value="n.id">{{ n.displayName }}</option></select></label>
    </div>
    <template v-if="game && node">
      <p>当前正式 ContentVersion：<code>{{ game.currentContentVersionId || '未发布' }}</code> · 合同：{{ upgraded ? 'v1_0_2' : '首次进入 v1_0_2 待发布' }}</p>
      <p>已完成 reconcile 批次 {{ node.reconcileCompleted }}；当前 v1.0.2 机器证明单独由能力、内容/模板事实和 inventory 决定。</p>
      <div class="admin-task-form"><label>候选 ContentVersion<select v-model="candidateContent"><option v-for="v in overview.contentVersions.filter(x => x.arcadeGameId === gameId)" :key="v.id" :value="v.id">{{ v.id }}</option></select></label><label>维护原因<input v-model.trim="maintenanceReason" maxlength="128" /></label></div>
      <article class="admin-task-node">
        <h3>1. Node × Game scoped maintenance</h3>
        <p>Binding {{ binding?.acceptingNewAllocations ? '开放' : '关闭' }} · epoch {{ status?.maintenanceEpoch ?? binding?.maintenanceEpoch ?? '未知' }} · 本游戏占用 {{ status?.targetOccupied ?? '未知' }} · 未决任务 {{ status?.targetUnresolvedJobs ?? '未知' }}。其它游戏继续运行；已有实例自然结束。</p>
        <button class="secondary-button" type="button" :disabled="busy || working || !binding || !maintenanceReason" @click="beginMaintenance">开始新维护代次并关闭此图分配</button>
      </article>
      <article class="admin-task-node">
        <h3>2. 本地准备与机器事实</h3><p>运维在节点本地手工准备 Content Tool / 模板；网页只读取 Controller 事实。可请求 Controller 核对。</p>
        <p>内容：{{ binding?.reportedState || 'unknown' }} · {{ binding?.reportedContentVersionId || '未上报' }} · SHA {{ binding?.reportedContentSha256?.slice(0, 12) || '无' }}… · fact revision {{ binding?.contentFactRevision ?? 0 }}</p>
        <p>Inventory：{{ inventory?.state || 'unknown' }} · 当前扫描 {{ inventory?.current ? '是' : '否' }} · 未入账 {{ inventory?.unaccountedCount ?? '未知' }}。</p>
        <p>Controller 能力：{{ node.capabilities?.join(', ') || '无' }}。</p>
        <button class="secondary-button" type="button" :disabled="busy || working" @click="action('/actions',{action:'node.reconcile',targetId:nodeId},'已请求新一轮 reconcile；仍需独立检查机器证明。')">请求 Controller 核对</button>
      </article>
      <article class="admin-task-node">
        <h3>3. 逐玩法 ValidationRun</h3>
        <div class="admin-task-form"><label>玩法<select v-model="presetId"><option v-for="p in presets" :key="p.id" :value="p.id">{{ p.displayName }} · {{ p.validationContract }}</option></select></label><label>候选模板<select v-model="candidateTemplate"><option v-for="t in overview.templateRevisions.filter(x => x.arcadeGameId === gameId)" :key="t.id" :value="t.id">{{ t.id }}</option></select></label></div>
        <div class="admin-task-form"><label>Node 模板绑定键<input v-model.trim="bindingKey" maxlength="128" /></label><label>期望 manifest SHA256<input v-model.trim="fingerprint" maxlength="64" /></label></div>
        <p>Binding generation {{ templateBinding?.bindingGeneration ?? 0 }} · Controller fact {{ templateBinding?.factState || 'unknown' }} · fact revision {{ templateBinding?.templateFactRevision ?? 0 }}。</p>
        <button class="secondary-button" type="button" :disabled="busy || working || !candidateTemplate || !bindingKey || !/^[0-9a-fA-F]{64}$/.test(fingerprint)" @click="saveTemplateBinding">保存期望模板身份</button>
        <button class="secondary-button" type="button" :disabled="busy || working || !status?.closed || !!status.targetOccupied || !!status.targetUnresolvedJobs || !candidateContent || !candidateTemplate" @click="startValidation">启动验证房</button>
        <div v-for="p in presets" :key="p.id" class="admin-task-preset"><strong>{{ p.displayName }}</strong><span>正式模板 {{ p.templateRevisionId }} · {{ p.acceptingNewRequests ? '开放' : '暂停' }}</span><span v-for="run in overview.validationRuns.filter(x => x.gameId === gameId && x.presetId === p.id)" :key="run.id"><button class="admin-text-button" type="button" @click="loadRun(run.id)">Run {{ run.id.slice(0, 8) }} · {{ run.state }} · {{ run.effective ? '当前证明有效' : run.invalidReason || '无有效证明' }}</button></span></div>
        <label>查找历史 Run ID<input v-model.trim="selectedRunId" /><button class="secondary-button" type="button" @click="loadRun(selectedRunId)">读取详情</button></label>
        <div v-if="detail" class="admin-task-next"><strong>Run {{ detail.id }} · {{ detail.state }}</strong>
          <p>Allocation {{ detail.allocationId }} / {{ detail.allocationState }} · create Job {{ detail.createJobId }} / {{ detail.createJobState }} · stop Job {{ detail.stopJobId || '无' }} / {{ detail.stopJobState || '无' }} · instance {{ detail.instanceId || '未知' }}</p>
          <p>Ready + JoinInfo：{{ detail.connectCommand || '尚不可进入' }}。真人确认 {{ detail.humanResult }}；最终 PASS：{{ detail.state === 'passed' ? '是' : '否' }}。当前证明 {{ detail.effective ? '有效' : detail.invalidReason || '无效' }}。</p>
          <div class="admin-actions"><button class="secondary-button" type="button" :disabled="busy || working || detail.state !== 'awaiting_human' || !detail.connectCommand" @click="confirmHuman('pass')">确认真人通过并正式停止</button><button class="secondary-button" type="button" :disabled="busy || working || !detail.instanceId || detail.state === 'passed' || detail.state === 'failed'" @click="confirmHuman('fail')">真人失败</button><button class="secondary-button" type="button" :disabled="busy || working || !detail.instanceId || detail.state === 'passed' || detail.state === 'failed' || !!detail.stopJobId" @click="stopValidation">中止并正式停止</button><button class="admin-text-button" type="button" @click="loadRun(detail.id)">刷新 Run</button></div>
        </div>
      </article>
      <article class="admin-task-node">
        <h3>4. 原子发布与开放</h3><p>{{ needsAll ? '首次升级或内容指针变化：必须列出全部现存玩法。' : '模板单独更新：可只列受影响玩法。' }}每个开放玩法必须选择自身当前有效 PASS；未证明玩法暂停。</p>
        <div v-for="p in presets" :key="p.id" class="admin-task-preset"><strong>{{ p.displayName }}</strong><label><input v-model="plan[p.id].selected" type="checkbox" :disabled="needsAll" />列入本次发布</label><template v-if="plan[p.id]?.selected"><label>新模板<select v-model="plan[p.id].template"><option v-for="t in overview.templateRevisions.filter(x => x.arcadeGameId === gameId)" :key="t.id" :value="t.id">{{ t.id }}</option></select></label><label><input v-model="plan[p.id].accepting" type="checkbox" />发布后开放</label><label>当前 PASS<select v-model="plan[p.id].runId" :disabled="!plan[p.id].accepting"><option value="">无</option><option v-for="run in overview.validationRuns.filter(x => x.presetId === p.id && x.contentVersionId === candidateContent && x.templateRevisionId === plan[p.id].template && x.effective)" :key="run.id" :value="run.id">{{ run.id.slice(0, 8) }} · {{ run.nodeId.slice(0, 8) }}</option></select></label></template></div>
        <button class="primary-button" type="button" :disabled="busy || working || !canPublish" @click="publish">发布正式组合（服务端 CAS）</button>
        <p>发布后按当前证明分别打开 Node binding、玩法申请；已有 Allocation 保持原快照。</p>
        <div class="admin-actions"><button class="secondary-button" type="button" :disabled="busy || working || binding?.acceptingNewAllocations" @click="action('/actions',{action:'binding.update',targetId:nodeId,gameId,accepting:true},'节点绑定已重新开放。')">开放此节点此图</button><button v-for="p in presets.filter(x => !x.acceptingNewRequests)" :key="p.id" class="secondary-button" type="button" :disabled="busy || working" @click="action('/actions',{action:'preset.update',targetId:p.id,accepting:true},'玩法已重新开放。')">开放 {{ p.displayName }}</button></div>
      </article>
      <article class="admin-task-node"><h3>发布历史与回退</h3><p>回退先在节点本地恢复旧 bytes，等 Controller 确认，再在本次 epoch 获取新 PASS；这里会创建新的 release，历史不会改写。</p><div v-for="release in gameReleases" :key="release.id" class="admin-task-preset"><span>{{ release.id.slice(0, 8) }} · {{ release.oldContentVersionId || '无' }} → {{ release.newContentVersionId }} · {{ release.publishedAt }} <template v-if="release.rollbackOfReleaseId">· 回退 {{ release.rollbackOfReleaseId.slice(0, 8) }}</template></span><button class="admin-text-button" type="button" @click="chooseRollback(release.id)">准备回退此发布</button></div><label>查找历史 Release ID<input v-model.trim="rollbackOf" /><button class="secondary-button" type="button" @click="chooseRollback(rollbackOf)">读取回退目标</button></label></article>
    </template>
  </section>
</template>

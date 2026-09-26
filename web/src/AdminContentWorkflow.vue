<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { canPublishContent, deriveNodeContentProgress, type WorkflowMode, type WorkflowOverview } from './adminContentWorkflow'

interface ContentAction { action: string; targetId?: string; gameId?: string; workshopId?: string; displayName?: string; contentVersionId?: string; contentSha256?: string; templateRevisionId?: string; description?: string; maxPlayers?: number; bindingKey?: string; accepting?: boolean; enabled?: boolean; draining?: boolean; confirmed?: boolean; message?: string }
const props = defineProps<{ overview: WorkflowOverview; busy: boolean }>()
const emit = defineEmits<{ action: [value: ContentAction, confirmation?: string]; refresh: [] }>()

const mode = ref<WorkflowMode | ''>('')
const workshopId = ref('')
const gameName = ref('')
const selectedGameId = ref('')
const versionId = ref('')
const sha256 = ref('')
const selectedNodeIds = ref<string[]>([])
const templateDraft = reactive({ id: '', description: '' })
const presetDraft = reactive({ name: '', maxPlayers: 10, templateId: '' })
const bindingKeys = reactive<Record<string, string>>({})
const smokeChecks = reactive<Record<string, boolean>>({})

const game = computed(() => mode.value === 'new'
  ? props.overview.games.find(item => item.workshopId === workshopId.value)
  : props.overview.games.find(item => item.id === selectedGameId.value))
const targetVersionId = computed(() => mode.value === 'manage' ? game.value?.currentContentVersionId || '' : versionId.value)
const version = computed(() => {
  const found = props.overview.contentVersions.find(item => item.id === targetVersionId.value && item.arcadeGameId === game.value?.id)
  return found && (mode.value === 'manage' || !sha256.value || found.contentSha256 === sha256.value.toLowerCase()) ? found : undefined
})
const conflictingVersion = computed(() => props.overview.contentVersions.find(item => item.id === versionId.value && (item.arcadeGameId !== game.value?.id || !!sha256.value && item.contentSha256 !== sha256.value.toLowerCase())))
const presets = computed(() => props.overview.presets.filter(item => item.arcadeGameId === game.value?.id))
const versions = computed(() => props.overview.contentVersions.filter(item => item.arcadeGameId === game.value?.id))
const revisions = computed(() => props.overview.templateRevisions.filter(item => item.arcadeGameId === game.value?.id))
const requiredRevisionIds = computed(() => [...new Set(presets.value.map(item => item.templateRevisionId))])
const nodeCards = computed(() => game.value && version.value ? props.overview.nodes.filter(node => selectedNodeIds.value.includes(node.id)).map(node => ({
  node, progress: deriveNodeContentProgress(props.overview, node, game.value!, version.value!, mode.value || 'manage'),
})) : [])
const nodesPrepared = computed(() => nodeCards.value.length > 0 && nodeCards.value.every(card => card.progress.complete))
const publishReady = computed(() => !!game.value && !!version.value && (mode.value !== 'new' || presets.value.length > 0) && canPublishContent(props.overview, game.value, version.value, mode.value || 'manage', selectedNodeIds.value))
const allSmokeChecked = (nodeId: string): boolean => presets.value.length > 0 && presets.value.every(preset => smokeChecks[`${nodeId}:${targetVersionId.value}:${preset.id}`])
const next = computed(() => {
  if (!mode.value) return '选择“接入新地图”或“更新现有地图 VPK”开始。'
  if (!game.value) return mode.value === 'new' ? '填写 Workshop ID 和显示名称，先登记地图。' : '先选择要更新的地图。'
  if (!version.value) return mode.value === 'new' ? '登记首个不可变 VPK 版本和 SHA256。' : mode.value === 'manage' ? '这张地图尚未发布版本；请从“接入新地图”继续准备。' : '登记这次更新的新 VPK 版本和 SHA256。'
  if (mode.value === 'new' && presets.value.length === 0) return '添加至少一个玩家可选玩法，并选择或登记其启动模板。'
  if (selectedNodeIds.value.length === 0) return '选择准备接入或更新的节点。'
  if (publishReady.value) return '至少一台节点已完成内容版本验收；现在可以发布为新开服版本。'
  const incomplete = nodeCards.value.find(card => !card.progress.complete)
  if (incomplete) return `${incomplete.node.displayName}：${incomplete.progress.next}`
  return '选中节点已使用当前发布版本并允许新分配；最后从普通玩家页面完成开服、进服、结束及完整回收。'
})

function beginNew(): void {
  mode.value = 'new'; workshopId.value = ''; gameName.value = ''; versionId.value = ''; sha256.value = ''; selectedNodeIds.value = []
}
function beginUpdate(id = ''): void {
  mode.value = 'update'; selectedGameId.value = id; versionId.value = ''; sha256.value = ''
  selectedNodeIds.value = props.overview.bindings.filter(item => item.arcadeGameId === id).map(item => item.nodeId)
}
function inspectMap(id: string): void {
  mode.value = 'manage'; selectedGameId.value = id
  selectedNodeIds.value = props.overview.nodes.map(item => item.id)
}
function validate(nodeId: string): void {
  if (!game.value || !version.value) return
  emit('action', { action: 'content.validate', targetId: nodeId, gameId: game.value.id, contentVersionId: version.value.id, confirmed: true },
    '我确认此节点在 Drain 中，已用正式模板完成真实 Dota 内容测试；临时实例已 stop 并达到 reclaimed/stopped/cleanup=complete，Controller 已重新核对且没有遗留实例。此操作只记录内容版本验收，不会切换节点文件。')
}
function publish(): void {
  if (!game.value || !version.value) return
  emit('action', { action: 'content.publish', targetId: game.value.id, contentVersionId: version.value.id, confirmed: true },
    '确认至少一台已恢复、兼容的节点上报相同版本和 SHA256，并已完成内容版本验收？发布只改变以后新开服务器的版本，不切换节点 VPK，也不修改已有服务器。')
}
function editMessage(kind: 'game.update' | 'preset.update', id: string, current: string): void {
  const value = window.prompt(kind === 'game.update' ? '地图维护提示' : '玩法维护提示', current)
  if (value !== null) emit('action', { action: kind, targetId: id, message: value })
}
</script>

<template>
  <section class="panel admin-card admin-task-home">
    <span class="eyebrow">选择要完成的事</span><h2>内容管理</h2>
    <div class="admin-task-choices">
      <button type="button" :aria-pressed="mode === 'new'" @click="beginNew"><strong>接入新地图</strong><span>从 Workshop ID、VPK 和玩法开始</span></button>
      <button type="button" :aria-pressed="mode === 'update'" @click="beginUpdate()"><strong>更新现有地图 VPK</strong><span>保留已有地图、玩法与启动模板</span></button>
    </div>
    <p class="admin-task-hint">这里记录平台状态并显示节点上报；VPK 和正式模板仍由运维在节点本地准备。</p>
  </section>

  <section v-if="mode" class="panel admin-card admin-task-workspace">
    <div class="admin-card-heading"><div><span class="eyebrow">{{ mode === 'new' ? '新地图接入' : mode === 'update' ? 'VPK 更新' : '现有地图' }}</span><h2>{{ mode === 'new' ? '接入新地图' : mode === 'update' ? '更新现有地图 VPK' : '地图当前状态' }}</h2></div><button type="button" class="secondary-button" @click="emit('refresh')">刷新状态</button></div>
    <div class="admin-task-next" role="status"><strong>{{ nodesPrepared ? mode === 'manage' ? '当前节点状态' : '节点准备完成 · 待玩家验收' : '下一步' }}</strong><p>{{ next }}</p></div>

    <div v-if="mode === 'new'" class="admin-task-step">
      <span class="admin-task-step-number">01</span><div><h3>地图基本信息</h3><p>玩家看到的是显示名称；Workshop ID 用来识别这一张地图。</p>
        <form v-if="!game" class="admin-task-form" @submit.prevent="emit('action',{action:'game.create',workshopId,displayName:gameName})"><label>Workshop ID<input v-model.trim="workshopId" required pattern="[0-9]+" maxlength="32" /></label><label>玩家显示名称<input v-model.trim="gameName" required maxlength="128" /></label><button class="secondary-button" type="submit" :disabled="busy">登记地图</button></form>
        <p v-else class="admin-task-done">已登记：{{ game.displayName }} · Workshop {{ game.workshopId }}</p>
      </div>
    </div>
    <div v-else class="admin-task-step"><span class="admin-task-step-number">01</span><div><h3>选择地图</h3><label>要操作的地图<select v-model="selectedGameId" @change="versionId = ''; sha256 = ''; selectedNodeIds = overview.bindings.filter(item => item.arcadeGameId === selectedGameId).map(item => item.nodeId)"><option value="">请选择</option><option v-for="item in overview.games" :key="item.id" :value="item.id">{{ item.displayName }} · {{ item.workshopId }}</option></select></label><p v-if="mode === 'update'">普通 VPK 更新不需要重建地图、玩法、启动模板或节点模板映射；只有启动参数、cfg 或 Ready 规则变化时才另建模板修订。</p></div></div>

    <div v-if="game" class="admin-task-step"><span class="admin-task-step-number">02</span><div><h3>{{ mode === 'new' ? '首个内容版本' : mode === 'update' ? '这次更新的内容版本' : '当前发布版本' }}</h3><p>当前发布版本：<code>{{ game.currentContentVersionId || '尚未发布' }}</code>。登记只写平台记录，不上传或切换 VPK。</p>
      <template v-if="mode !== 'manage'"><form v-if="!version" class="admin-task-form" @submit.prevent="emit('action',{action:'content.create',targetId:game.id,contentVersionId:versionId,contentSha256:sha256.toLowerCase()})"><label>新 ContentVersion ID<input v-model.trim="versionId" required maxlength="128" /></label><label>隔离副本 VPK 的 SHA256<input v-model.trim="sha256" required pattern="[0-9a-fA-F]{64}" maxlength="64" /></label><button class="secondary-button" type="submit" :disabled="busy || !!conflictingVersion">登记 VPK 版本</button></form><p v-if="conflictingVersion" class="admin-task-warning">该版本 ID 已有不同的地图或 SHA256。版本不可覆盖，请换一个新 ID。</p></template>
      <p v-if="version" class="admin-task-done">已登记：<code>{{ version.id }}</code> · SHA256 {{ version.contentSha256.slice(0, 12) }}…</p>
    </div></div>

    <section v-if="game && mode === 'manage'" class="admin-task-manage"><h3>日常开放状态</h3><p>地图申请、每个玩法申请和各节点的新分配各自独立；停止新申请不会停止已有服务器。</p>
      <div class="admin-task-manage-row"><div><strong>地图 · {{ game.displayName }}</strong><small>{{ game.enabled ? game.acceptingNewRequests ? '已启用 · 可申请' : '已启用 · 暂停申请' : '已停用' }}{{ game.maintenanceMessage ? ' · 提示：' + game.maintenanceMessage : '' }}</small></div><div class="admin-actions"><button type="button" class="secondary-button" :disabled="busy || !game.enabled && !game.acceptingNewRequests" @click="emit('action',{action:'game.update',targetId:game.id,accepting:!game.acceptingNewRequests})">{{ game.acceptingNewRequests ? '暂停地图申请' : '恢复地图申请' }}</button><button type="button" class="admin-text-button" :disabled="busy" @click="editMessage('game.update',game.id,game.maintenanceMessage || '')">编辑维护提示</button><button type="button" class="admin-text-button" :disabled="busy" @click="emit('action',{action:'game.update',targetId:game.id,enabled:!game.enabled},game.enabled ? '确认停用这张地图？已有服务器不受影响。' : '')">{{ game.enabled ? '停用地图' : '启用地图' }}</button></div></div>
      <div v-for="preset in presets" :key="preset.id" class="admin-task-manage-row"><div><strong>玩法 · {{ preset.displayName }}</strong><small>最多 {{ preset.maxPlayers }} 人 · 启动模板 {{ preset.templateRevisionId }} · {{ preset.enabled ? preset.acceptingNewRequests ? '可申请' : '暂停申请' : '已停用' }}{{ preset.maintenanceMessage ? ' · 提示：' + preset.maintenanceMessage : '' }}</small></div><div class="admin-actions"><button type="button" class="secondary-button" :disabled="busy || !preset.enabled && !preset.acceptingNewRequests" @click="emit('action',{action:'preset.update',targetId:preset.id,accepting:!preset.acceptingNewRequests})">{{ preset.acceptingNewRequests ? '暂停玩法申请' : '恢复玩法申请' }}</button><button type="button" class="admin-text-button" :disabled="busy" @click="editMessage('preset.update',preset.id,preset.maintenanceMessage || '')">编辑提示</button><button type="button" class="admin-text-button" :disabled="busy" @click="emit('action',{action:'preset.update',targetId:preset.id,enabled:!preset.enabled},preset.enabled ? '确认停用这个玩法？已有服务器不受影响。' : '')">{{ preset.enabled ? '停用玩法' : '启用玩法' }}</button></div></div>
      <details class="admin-content-task"><summary>查看当前与保留的 VPK 版本</summary><div v-for="item in versions" :key="item.id" class="admin-task-version"><strong>{{ item.id }}</strong><span>{{ item.id === game.currentContentVersionId ? '当前发布版本' : '保留版本' }}</span></div></details>
    </section>

    <div v-if="game && mode === 'new'" class="admin-task-step"><span class="admin-task-step-number">03</span><div><h3>添加玩家玩法</h3><p>玩家看到的是玩法名称和人数；启动模板修订是内部配置。多个玩法可复用同一启动语义。</p>
      <div v-for="preset in presets" :key="preset.id" class="admin-task-preset"><strong>{{ preset.displayName }}</strong><span>最多 {{ preset.maxPlayers }} 人 · 模板 {{ preset.templateRevisionId }}</span></div>
      <details class="admin-content-task"><summary>需要新的启动模板修订？</summary><p>只登记平台修订；正式模板文件及绑定键要先在各节点本地准备。</p><form class="admin-task-form" @submit.prevent="emit('action',{action:'template.create',targetId:game.id,templateRevisionId:templateDraft.id,description:templateDraft.description})"><label>模板修订 ID<input v-model.trim="templateDraft.id" required maxlength="128" /></label><label>启动语义说明<input v-model.trim="templateDraft.description" maxlength="1000" /></label><button class="secondary-button" type="submit" :disabled="busy">登记模板修订</button></form></details>
      <form class="admin-task-form" @submit.prevent="emit('action',{action:'preset.create',targetId:game.id,displayName:presetDraft.name,maxPlayers:presetDraft.maxPlayers,templateRevisionId:presetDraft.templateId})"><label>玩家玩法名称<input v-model.trim="presetDraft.name" required maxlength="128" /></label><label>最大玩家数<input v-model.number="presetDraft.maxPlayers" type="number" required min="1" step="1" /></label><label>启动模板修订<select v-model="presetDraft.templateId" required><option value="">请选择</option><option v-for="revision in revisions" :key="revision.id" :value="revision.id">{{ revision.id }}</option></select></label><button class="secondary-button" type="submit" :disabled="busy">添加玩法</button></form>
    </div></div>

    <div v-if="game && version" class="admin-task-step"><span class="admin-task-step-number">{{ mode === 'new' ? '04' : '03' }}</span><div><h3>选择准备的节点</h3><p>可以先完成一台并发布，另一台以后再准备；平台发布不会替另一台切换 VPK。</p><div class="admin-task-node-choices"><label v-for="node in overview.nodes" :key="node.id" class="admin-check"><input v-model="selectedNodeIds" type="checkbox" :value="node.id" /> {{ node.displayName }} · {{ node.compatibility === 'compatible' ? '兼容' : '待核对' }}</label></div></div></div>

    <div v-if="nodeCards.length" class="admin-task-step"><span class="admin-task-step-number">{{ mode === 'new' ? '05' : '04' }}</span><div><h3>逐台准备与验收</h3><p>状态来自 Controller 上报和平台已有记录；下方本地测试勾选仅供当前页面提醒，刷新后不作为持久验收凭据。</p>
      <article v-for="{ node, progress } in nodeCards" :key="node.id" class="admin-task-node">
        <div class="admin-entity-head"><div><h3>{{ node.displayName }}</h3><small>已上报版本 {{ progress.binding?.reportedContentVersionId || '暂无' }} · 平台占用 {{ node.occupied }}</small><small v-if="progress.validation">内容版本验收时间 {{ new Date(progress.validation.verifiedAt).toLocaleString('zh-CN') }}</small></div><span class="admin-badge" :class="{ 'admin-badge-warm': !progress.complete }">{{ progress.complete ? '节点已准备' : '尚未完成' }}</span></div>
        <ul class="admin-task-facts"><li>{{ progress.binding ? '✓' : '○' }} Controller 已上报此地图</li><li>{{ progress.reportedVersion ? '✓' : '○' }} 版本为 {{ version?.id }}</li><li>{{ progress.digestMatches ? '✓' : '○' }} SHA256 匹配</li><li>{{ progress.confirmed ? '✓' : '○' }} 节点内容已确认</li><li v-if="mode === 'new'">{{ progress.mappedTemplates === progress.requiredTemplates && progress.requiredTemplates > 0 ? '✓' : '○' }} 玩法模板映射 {{ progress.mappedTemplates }}/{{ progress.requiredTemplates }}</li><li>{{ progress.validation ? '✓' : '○' }} 内容版本验收</li><li>{{ progress.published ? '✓' : '○' }} 平台已发布此版本</li><li>{{ progress.admissionOpen ? '✓' : '○' }} 此图新分配已开启</li></ul>
        <div class="admin-task-next"><strong>{{ progress.complete ? '节点状态' : '下一步' }}</strong><p>{{ progress.next }}</p></div>
        <p v-if="!progress.binding" class="admin-task-warning">在此节点外部配置 Controller 对 Workshop {{ game?.workshopId }} 的内容绑定，并用 Content Tool 准备 VPK。Web 无法上传或切换节点文件。</p>
        <p v-else-if="!progress.reportedVersion" class="admin-task-hint">节点外部：先确认旧实例完整回收，再执行 Content Tool prepare → switch → status；本页不会远程运行命令。</p>
        <div class="admin-actions"><button v-if="progress.binding && progress.admissionOpen" type="button" class="secondary-button" :disabled="busy" @click="emit('action',{action:'binding.update',targetId:node.id,gameId:game?.id,accepting:false})">暂停此图新分配</button><button type="button" class="secondary-button" :disabled="busy" @click="emit('action',{action:'node.reconcile',targetId:node.id})">请求 Controller 核对</button><button v-if="progress.confirmed && !progress.validation && !node.draining" type="button" class="secondary-button" :disabled="busy || node.occupied > 0" @click="emit('action',{action:'node.update',targetId:node.id,draining:true})">进入节点维护</button><button v-else-if="progress.validation && node.draining" type="button" class="secondary-button" :disabled="busy" @click="emit('action',{action:'node.update',targetId:node.id,draining:false})">结束节点维护</button></div>
        <div v-if="mode === 'new' && progress.confirmed && progress.mappedTemplates < progress.requiredTemplates" class="admin-task-mapping"><p>先在此节点本地准备正式模板文件和 Controller 绑定键，再保存下列逻辑映射。</p><div v-for="revisionId in requiredRevisionIds.filter(id => !overview.templateBindings.some(item => item.nodeId === node.id && item.templateRevisionId === id))" :key="revisionId" class="admin-task-map-row"><label>{{ revisionId }} 的 Controller 绑定键<input v-model.trim="bindingKeys[`${node.id}:${revisionId}`]" maxlength="128" /></label><button type="button" class="secondary-button" :disabled="busy || !bindingKeys[`${node.id}:${revisionId}`]" @click="emit('action',{action:'template_binding.upsert',targetId:node.id,templateRevisionId:revisionId,bindingKey:bindingKeys[`${node.id}:${revisionId}`]})">保存映射</button></div></div>
        <div v-if="progress.confirmed && !progress.validation" class="admin-task-validation"><h4>{{ mode === 'new' ? '逐玩法真人 smoke' : '本地内容测试' }}</h4><p>仅在 Node Drain 且 d2core/Platform 均确认无占用时，本地用正式模板创建临时实例；真人进服后显式 stop，确认 reclaimed/stopped/cleanup=complete，再让 Controller resync。</p><div v-if="mode === 'new'" class="admin-task-smoke"><label v-for="preset in presets" :key="preset.id" class="admin-check"><input v-model="smokeChecks[`${node.id}:${version?.id}:${preset.id}`]" type="checkbox" /> {{ preset.displayName }} · {{ smokeChecks[`${node.id}:${version?.id}:${preset.id}`] ? '本页已勾选' : '待测试' }}</label></div><button type="button" class="secondary-button" :disabled="busy || !progress.canValidate || (mode === 'new' && !allSmokeChecked(node.id)) || (mode === 'new' && progress.mappedTemplates < progress.requiredTemplates)" @click="validate(node.id)">确认内容版本验收</button><small v-if="!progress.canValidate">需先达到：版本/SHA256 已确认、节点兼容且在线、Drain、占用为零；平台提交时仍会核对新鲜心跳。</small></div>
        <button v-if="progress.published && progress.validation && !progress.admissionOpen" type="button" class="secondary-button" :disabled="busy || !progress.confirmed || node.draining" @click="emit('action',{action:'binding.update',targetId:node.id,gameId:game?.id,accepting:true})">打开此图新分配</button>
      </article>
    </div></div>

    <div v-if="game && version && nodeCards.length" class="admin-task-step"><span class="admin-task-step-number">{{ mode === 'new' ? '06' : '05' }}</span><div><h3>发布并开放</h3><p>节点切换 VPK 改的是节点磁盘；平台发布改的是以后新 Allocation 使用的版本。已有服务器不会改版。</p><p class="admin-task-done">当前发布版本：<code>{{ game.currentContentVersionId || '尚未发布' }}</code></p><button v-if="game.currentContentVersionId !== version.id" type="button" class="primary-button" :disabled="busy || !publishReady" @click="publish">发布为新开服版本</button><p v-else class="admin-task-done">此版本已发布。每台节点仍须单独开放此图新分配。</p>
      <div v-if="mode === 'new' && game.currentContentVersionId === version.id && nodeCards.some(card => card.progress.admissionOpen)" class="admin-task-open"><div class="admin-actions"><button v-if="!game.enabled" type="button" class="secondary-button" :disabled="busy" @click="emit('action',{action:'game.update',targetId:game.id,enabled:true})">启用地图</button><button v-if="!game.acceptingNewRequests" type="button" class="secondary-button" :disabled="busy || !game.enabled" @click="emit('action',{action:'game.update',targetId:game.id,accepting:true})">开放地图申请</button></div><div v-for="preset in presets" :key="preset.id" class="admin-task-preset"><strong>{{ preset.displayName }}</strong><div class="admin-actions"><button v-if="!preset.enabled" type="button" class="secondary-button" :disabled="busy" @click="emit('action',{action:'preset.update',targetId:preset.id,enabled:true},'确认这个玩法已在准备开放的节点使用正式模板完成真人进服测试，临时实例已完整回收？')">启用玩法</button><button v-if="!preset.acceptingNewRequests" type="button" class="secondary-button" :disabled="busy || !preset.enabled" @click="emit('action',{action:'preset.update',targetId:preset.id,accepting:true})">开放玩法申请</button></div></div></div><p class="admin-task-hint">最后到普通玩家页面完成 request → Ready/JoinInfo → 实际进服 → 结束 → 完整回收。本页不会把未发生的玩家测试标记成完成。</p>
    </div></div>
    <details class="admin-content-task admin-task-details"><summary>查看节点本地详细操作</summary><p>先在节点隔离复制 VPK，并用 Content Tool prepare、switch、status；配置 Controller 的内容元数据和正式模板绑定键。平台只显示逻辑键，不保存 DOTA_ROOT、CONTENT_ROOT、模板或 d2core 私有 data-dir 的绝对路径。</p><p>节点维护测试要使用固定 d2core v0.1.1 的正式模板。每个临时实例都须真人进入、显式 stop、查 operation/status/list 并确认完整回收，然后请求 Controller 核对。后台不会远程执行这些命令。</p></details>
  </section>

  <section class="panel admin-card admin-task-existing"><span class="eyebrow">日常查看</span><h2>现有地图管理</h2><p>地图总开关、玩法开关、节点对该图的新分配是三层独立控制。</p><div v-for="item in overview.games" :key="item.id" class="admin-task-existing-row"><div><strong>{{ item.displayName }}</strong><small>Workshop {{ item.workshopId }} · 当前发布 {{ item.currentContentVersionId || '未发布' }} · {{ item.enabled ? item.acceptingNewRequests ? '可申请' : '暂停申请' : '已停用' }}</small></div><div class="admin-actions"><button type="button" class="secondary-button" @click="inspectMap(item.id)">查看状态</button><button type="button" class="admin-text-button" @click="beginUpdate(item.id)">更新 VPK</button></div></div><p v-if="overview.games.length === 0" class="admin-empty">还没有地图，从上方“接入新地图”开始。</p></section>
</template>

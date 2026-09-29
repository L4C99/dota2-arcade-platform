// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { createApp, nextTick, type App } from 'vue'
import AdminI3Workflow from './AdminI3Workflow.vue'

let app: App | undefined
let root: HTMLDivElement | undefined
const data = () => ({
  games: [{ id: 'g', displayName: 'Test game', currentContentVersionId: 'v2' }],
  presets: [
    { id: 'p1', arcadeGameId: 'g', displayName: 'P1', templateRevisionId: 't2', validationContract: 'v1_0_2', acceptingNewRequests: true },
    { id: 'p2', arcadeGameId: 'g', displayName: 'P2', templateRevisionId: 't1', validationContract: 'v1_0_2', acceptingNewRequests: false },
  ],
  nodes: [{ id: 'n', displayName: 'Node A', enabled: true, acceptingNewRequests: true, draining: false,
    connectivity: 'online', compatibility: 'compatible', reconcileCompleted: 5, capabilities: ['contentValidationV102'] }],
  bindings: [{ nodeId: 'n', arcadeGameId: 'g', reportedState: 'confirmed', reportedContentVersionId: 'v2',
    reportedContentSha256: 'a'.repeat(64), acceptingNewAllocations: false, maintenanceEpoch: 3, contentFactRevision: 8 }],
  templateBindings: [{ nodeId: 'n', templateRevisionId: 't2', bindingKey: 'binding', expectedFingerprintSha256: 'f'.repeat(64),
    bindingGeneration: 2, factState: 'confirmed', factFingerprintSha256: 'f'.repeat(64), templateFactRevision: 6 }],
  contentVersions: [{ id: 'v2', arcadeGameId: 'g' }],
  templateRevisions: [{ id: 't1', arcadeGameId: 'g' }, { id: 't2', arcadeGameId: 'g' }],
  validationRuns: [{ id: 'r1', nodeId: 'n', gameId: 'g', presetId: 'p1', contentVersionId: 'v2',
    templateRevisionId: 't2', state: 'passed', humanResult: 'pass', effective: true, formal: true }],
  releases: [{ id: 'release-1', gameId: 'g', oldContentVersionId: 'v1', newContentVersionId: 'v2', publishedAt: '2026-09-29T00:00:00Z' }],
  inventories: [{ nodeId: 'n', state: 'confirmed', unaccountedCount: 0, current: true, receivedAt: '2026-09-29T00:00:00Z' }],
})
async function settle() { for (let i = 0; i < 12; i++) { await Promise.resolve(); await nextTick() } }
async function mount() {
  root = document.createElement('div'); document.body.append(root)
  app = createApp(AdminI3Workflow, { overview: data(), busy: false }); app.mount(root)
  const selectors = root.querySelectorAll('select')
  ;(selectors[0] as HTMLSelectElement).value = 'g'; selectors[0].dispatchEvent(new Event('change', { bubbles: true }))
  await settle()
  ;(root.querySelectorAll('select')[1] as HTMLSelectElement).value = 'n'
  root.querySelectorAll('select')[1].dispatchEvent(new Event('change', { bubbles: true }))
  await settle()
}
afterEach(() => { app?.unmount(); root?.remove(); vi.unstubAllGlobals() })

it('shows per-Preset PASS and pause, with reconcile separate from machine proof', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ maintenanceEpoch: 3, closed: true, targetOccupied: 0, targetUnresolvedJobs: 0, nodeOccupied: 0 }), { status: 200 })))
  await mount()
  expect(root!.textContent).toContain('P1')
  expect(root!.textContent).toContain('当前证明有效')
  expect(root!.textContent).toContain('P2')
  expect(root!.textContent).toContain('暂停')
  expect(root!.textContent).toContain('已完成 reconcile 批次 5')
  expect(root!.textContent).toContain('机器证明')
  expect(root!.textContent).not.toContain('整台节点占用为 0')
})

it('keeps human pass separate from final PASS and uses scoped maintenance', async () => {
  const requests: { path: string; body?: Record<string, unknown> }[] = []
  vi.stubGlobal('crypto', { randomUUID: () => 'request-1' })
  vi.stubGlobal('fetch', vi.fn(async (url: string, options?: RequestInit) => {
    requests.push({ path: url, body: options?.body ? JSON.parse(String(options.body)) : undefined })
    if (url.includes('/validation/r1')) return new Response(JSON.stringify({ id: 'r1', state: 'stop_pending', humanResult: 'pass',
      allocationId: 'a', allocationState: 'stopping', createJobId: 'c', createJobState: 'succeeded', stopJobId: 's', stopJobState: 'pending', instanceId: 'i', effective: false }), { status: 200 })
    return new Response(JSON.stringify({ maintenanceEpoch: 4, closed: true, targetOccupied: 0, targetUnresolvedJobs: 0, nodeOccupied: 0 }), { status: 200 })
  }))
  await mount()
  const begin = [...root!.querySelectorAll('button')].find(x => x.textContent?.includes('开始新维护代次'))!
  begin.click(); await settle()
  const post = requests.find(x => x.path.endsWith('/maintenance/begin'))!
  expect(post.body).toMatchObject({ nodeId: 'n', gameId: 'g', expectedMaintenanceEpoch: 4, requestId: 'request-1' })
  expect(requests.some(x => x.body?.action === 'node.update')).toBe(false)
  const runLookup = [...root!.querySelectorAll('button')].find(x => x.textContent?.includes('Run r1'))!
  runLookup.click(); await settle()
  expect(root!.textContent).toContain('真人确认 pass；最终 PASS：否')
})

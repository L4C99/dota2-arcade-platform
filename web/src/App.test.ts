// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { createApp, nextTick, type App as VueApp } from 'vue'
import App from './App.vue'
import { api } from './api'
vi.mock('./api', async (original) => {
  const module = await original<typeof import('./api')>()
  return { ...module, api: Object.fromEntries(Object.keys(module.api).map(k => [k, vi.fn()])) }
})
let app: VueApp
let root: HTMLDivElement
const request = { id: 'old', arcadeGameId: 'g', gamePresetId: 'p', state: 'unavailable', requestedAt: '2026-09-27T00:00:00Z', updatedAt: '', nodeSelectionMode: 'auto' as const }
async function mount() { root = document.createElement('div'); document.body.append(root); app = createApp(App); app.mount(root); for(let i=0;i<20;i++) { await Promise.resolve(); await nextTick() } }
beforeEach(() => {
  vi.resetAllMocks(); localStorage.clear()
  vi.mocked(api.me).mockResolvedValue({ userId: 'owner', displayName: '保留的玩家' })
  vi.mocked(api.catalog).mockResolvedValue({ globalAcceptingNewRequests:true, globalMaintenanceMessage:'', siteAnnouncement:'', games:[{id:'g', displayName:'地图', workshopId:'123', enabled:true, acceptingNewRequests:true, maintenanceMessage:''}], presets:[{id:'p', arcadeGameId:'g',displayName:'玩法',enabled:true,acceptingNewRequests:true,maintenanceMessage:'',maxPlayers:8}] })
  vi.mocked(api.party).mockResolvedValue(null)
  vi.mocked(api.current).mockResolvedValue(null)
  vi.mocked(api.getRequest).mockResolvedValue(request)
  vi.mocked(api.allocation).mockResolvedValue(null)
  vi.mocked(api.nextGameIntent).mockResolvedValue(null)
  vi.mocked(api.nodes).mockResolvedValue([{id:'node',displayName:'节点',status:'unavailable',connectivity:'offline',reason:'unreachable',availableSlots:0,selectable:false}])
})
afterEach(() => { app?.unmount(); root?.remove() })
it('lets unavailable history return to selection without submitting or replacing identity', async () => {
  localStorage.setItem('arcade.lastRequestId','old')
  await mount()
  const button = [...root.querySelectorAll('button')].find(b => b.textContent === '重新申请')!
  expect(button).toBeTruthy(); button.click(); await nextTick()
  expect(root.textContent).toContain('全部地图')
  expect(root.textContent).toContain('保留的玩家')
  expect(localStorage.getItem('arcade.lastRequestId')).toBeNull()
  expect(api.createRequest).not.toHaveBeenCalled(); expect(api.session).not.toHaveBeenCalled()
})
it('keeps current request priority and valid join controls during offline heartbeat', async () => {
  localStorage.setItem('arcade.lastRequestId','old')
  vi.mocked(api.current).mockResolvedValue({...request,id:'new',state:'running'})
  vi.mocked(api.allocation).mockResolvedValue({id:'a',serverRequestId:'new',attemptSequence:1,nodeId:'node',nodeDisplayName:'节点',contentVersionId:'v',state:'running',assignedAt:'',readyAt:'time',joinInfoAvailableAt:'time',joinInfo:{connectCommand:'connect 203.0.113.1:28000',connectHost:'203.0.113.1',publicPort:28000}})
  await mount()
  expect(api.getRequest).not.toHaveBeenCalled()
  expect(root.textContent).toContain('节点心跳暂不可达')
  expect(root.textContent).toContain('connect 203.0.113.1:28000')
  expect(root.querySelector('.join-panel')).not.toBeNull()
})

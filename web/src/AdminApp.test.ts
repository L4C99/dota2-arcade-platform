// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { createApp, nextTick, type App } from 'vue'
import AdminApp from './AdminApp.vue'
let app: App
let root: HTMLDivElement
const node = { id:'n',displayName:'测试节点',os:'windows',connectivity:'online',controllerVersion:'v',d2coreVersion:'v',d2coreCommit:'c',compatibility:'compatible',recentErrorCode:'',enabled:true,acceptingNewRequests:true,draining:false,priority:1,hard:4,desired:2,occupied:0,reconcileRequested:0,reconcileCompleted:0 }
const overview = () => ({ settings:{acceptingNewRequests:true,maintenanceMessage:'',siteAnnouncement:''},counts:{waiting:0,creating:0,running:0,stopping:0,quarantined:0,failedUnreclaimed:0},games:[],presets:[],contentVersions:[],templateRevisions:[],templateBindings:[],contentValidations:[],nodes:[{...node}],bindings:[],entries:[],requests:[],parties:[],allocations:[],jobs:[],audit:[] })
async function settle() { for(let i=0;i<20;i++){await Promise.resolve();await nextTick()} }
async function mount() {root=document.createElement('div');document.body.append(root);app=createApp(AdminApp);app.mount(root);await settle()}
function click(label:string) {const b=[...root.querySelectorAll('button')].find(b=>b.textContent===label);expect(b).toBeTruthy();b!.click()}
afterEach(()=>{app?.unmount();root?.remove();vi.unstubAllGlobals()})
it('retries a failed overview without login and sends 401 back to login',async()=>{
 let status=503
 vi.stubGlobal('fetch',vi.fn(async(url:string)=>new Response(JSON.stringify(url.endsWith('/overview')?overview():{}),{status:url.endsWith('/overview')?status:200})))
 await mount();expect(root.textContent).toContain('管理数据暂不可用');expect(root.textContent).not.toContain('登录管理后台')
 status=200;click('重试加载');await settle();expect(root.textContent).toContain('管理后台')
 status=401;click('刷新状态');await settle();expect(root.textContent).toContain('登录管理后台')
})
it('preserves dirty node edits on unrelated refresh and requires resync on conflict',async()=>{
 let data=overview()
 vi.stubGlobal('fetch',vi.fn(async(url:string)=>new Response(JSON.stringify(url.endsWith('/overview')?data:{}),{status:200})))
 await mount();click('节点');await settle()
 const input=root.querySelector<HTMLInputElement>('input[type=number]')!
 input.value='9';input.dispatchEvent(new Event('input',{bubbles:true}));await settle()
 click('刷新状态');await settle();expect(input.value).toBe('9')
 data={...data,nodes:[{...node,priority:3}]};click('刷新状态');await settle()
 expect(input.value).toBe('9');expect(root.textContent).toContain('服务器上的调度设置已变化')
 expect([...root.querySelectorAll('button')].find(b=>b.textContent==='保存调度设置')!.disabled).toBe(true)
 click('放弃草稿并同步');await settle();expect(input.value).toBe('3')
})
it('retains authenticated UI when logout fails',async()=>{
 vi.stubGlobal('fetch',vi.fn(async(url:string)=>new Response(JSON.stringify(url.endsWith('/overview')?overview():{}),{status:url.endsWith('/logout')?503:200})))
 await mount();click('退出');await settle();expect(root.textContent).toContain('退出失败');expect(root.textContent).not.toContain('登录管理后台')
})
it('shows the manual content publish control only for legacy games',async()=>{
 const game=(id:string)=>({id,displayName:id,workshopId:'123456',currentContentVersionId:'v1',maintenanceMessage:'',enabled:true,acceptingNewRequests:true})
 const preset=(id:string,gameId:string,contract:string)=>({id,arcadeGameId:gameId,displayName:id,templateRevisionId:'t1',maintenanceMessage:'',enabled:true,acceptingNewRequests:true,maxPlayers:10,validationContract:contract})
 const data={...overview(),games:[game('upgraded'),game('legacy')],presets:[preset('p1','upgraded','v1_0_2'),preset('p2','legacy','legacy_v1')],releases:[],inventories:[]}
 vi.stubGlobal('fetch',vi.fn(async(url:string)=>new Response(JSON.stringify(url.endsWith('/overview')?data:{}),{status:200})))
 await mount();click('内容与维护');await settle()
 expect(root.textContent).not.toContain('旧合同手动发布')
 const selector=[...root.querySelectorAll('select')].find(s=>s.closest('.admin-catalog-create'))!
 selector.value='legacy';selector.dispatchEvent(new Event('change',{bubbles:true}));await settle()
 expect(root.textContent).toContain('旧合同手动发布')
})
it('shows legacy content validation only for a pure legacy game on the Node tab',async()=>{
 const game=(id:string)=>({id,displayName:id,workshopId:'123456',currentContentVersionId:'v1',maintenanceMessage:'',enabled:true,acceptingNewRequests:true})
 const preset=(id:string,gameId:string,contract:string)=>({id,arcadeGameId:gameId,displayName:id,templateRevisionId:'t1',maintenanceMessage:'',enabled:true,acceptingNewRequests:true,maxPlayers:10,validationContract:contract})
 const binding=(gameId:string)=>({nodeId:'n',arcadeGameId:gameId,reportedContentVersionId:'v1',reportedContentSha256:'a'.repeat(64),reportedState:'confirmed',acceptingNewAllocations:false})
 const data={...overview(),games:[game('upgraded'),game('released'),game('legacy')],presets:[preset('p1','upgraded','v1_0_2'),preset('p2','released','legacy_v1'),preset('p3','legacy','legacy_v1')],releases:[{id:'r',gameId:'released'}],bindings:[binding('upgraded'),binding('released'),binding('legacy')],contentVersions:[{id:'v1',arcadeGameId:'upgraded',contentSha256:'a'.repeat(64)},{id:'v1',arcadeGameId:'released',contentSha256:'a'.repeat(64)},{id:'v1',arcadeGameId:'legacy',contentSha256:'a'.repeat(64)}],inventories:[]}
 vi.stubGlobal('fetch',vi.fn(async(url:string)=>new Response(JSON.stringify(url.endsWith('/overview')?data:{}),{status:200})))
 await mount();click('节点');await settle()
 const cards=[...root.querySelectorAll<HTMLElement>('.admin-map-binding')]
 expect(cards.find(x=>x.textContent?.includes('upgraded'))?.querySelector('.admin-map-record')).toBeNull()
 expect(cards.find(x=>x.textContent?.includes('released'))?.querySelector('.admin-map-record')).toBeNull()
 expect(cards.find(x=>x.textContent?.includes('legacy'))?.querySelector('.admin-map-record')).not.toBeNull()
})

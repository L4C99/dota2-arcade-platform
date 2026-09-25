export interface ArcadeGame {
  id: string
  workshopId: string
  displayName: string
  enabled: boolean
  acceptingNewRequests: boolean
  maintenanceMessage: string
}

export interface GamePreset {
  id: string
  arcadeGameId: string
  displayName: string
  enabled: boolean
  acceptingNewRequests: boolean
  maintenanceMessage: string
  maxPlayers: number
}

export interface Catalog {
  globalAcceptingNewRequests: boolean
  globalMaintenanceMessage: string
  games: ArcadeGame[]
  presets: GamePreset[]
}

export interface ServerRequest {
  id: string
  arcadeGameId: string
  gamePresetId: string
  state: string
  requestedAt: string
  updatedAt: string
  nodeSelectionMode: 'auto' | 'manual'
  manualNodeId?: string
}

export interface NodeChoice {
  id: string
  displayName: string
  status: 'available' | 'full' | 'maintenance' | 'unavailable'
  connectivity: 'online' | 'stale' | 'offline'
  reason: string
  availableSlots: number
  selectable: boolean
}

export interface PartyMember {
  userId: string
  displayName: string
  role: 'leader' | 'member'
  joinedAt: string
}

export interface Party {
  id: string
  leaderUserId: string
  currentRole: 'leader' | 'member'
  maxSize: number
  members: PartyMember[]
  createdAt: string
}

export interface PartyInvite {
  partyId: string
  token: string
}

export interface JoinInfo {
  connectCommand: string
  connectHost: string
  publicPort: number
  steamUri?: string
  steamChinaUri?: string
}

export interface Allocation {
  id: string
  serverRequestId: string
  attemptSequence: number
  nodeId: string
  nodeDisplayName: string
  contentVersionId: string
  state: string
  assignedAt: string
  createStartedAt?: string
  readyAt?: string
  joinInfoAvailableAt?: string
  reclaimedAt?: string
  errorCode?: string
  joinInfo?: JoinInfo
  joinInfoErrorCode?: string
}

export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string) {
    super(message)
  }
}

async function request<T>(path: string, method = 'GET', body?: object): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    method,
    credentials: 'same-origin',
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!response.ok) {
    const contentType = response.headers.get('Content-Type') || ''
    const error = contentType.includes('application/json') ? await response.json() : { message: await response.text() }
    throw new ApiError(response.status, error.code || '', error.message || `请求失败 (${response.status})`)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export const api = {
  me: () => request<{ userId: string; displayName: string }>('/me'),
  session: () => request<{ userId: string; displayName: string }>('/session', 'POST'),
  catalog: () => request<Catalog>('/catalog'),
  nodes: (arcadeGameId: string, gamePresetId: string) =>
    request<NodeChoice[]>(`/nodes?arcadeGameId=${encodeURIComponent(arcadeGameId)}&gamePresetId=${encodeURIComponent(gamePresetId)}`),
  party: () => request<Party | null>('/party'),
  createParty: () => request<Party>('/party', 'POST'),
  currentInvite: () => request<PartyInvite>('/party/invite'),
  resetInvite: () => request<PartyInvite>('/party/invite/reset', 'POST'),
  joinParty: (token: string) => request<Party>('/party/join', 'POST', { token }),
  leaveParty: () => request<void>('/party/leave', 'POST'),
  removeMember: (id: string) => request<void>(`/party/members/${encodeURIComponent(id)}/remove`, 'POST'),
  disbandParty: () => request<void>('/party/disband', 'POST'),
  current: () => request<ServerRequest | null>('/server-requests/current'),
  getRequest: (id: string) => request<ServerRequest>(`/server-requests/${encodeURIComponent(id)}`),
  createRequest: (arcadeGameId: string, gamePresetId: string, nodeSelectionMode: 'auto' | 'manual' = 'auto', manualNodeId = '') =>
    request<ServerRequest>('/server-requests', 'POST', { arcadeGameId, gamePresetId, nodeSelectionMode, manualNodeId }),
  allocation: (id: string) => request<Allocation | null>(`/server-requests/${encodeURIComponent(id)}/allocation`),
  stop: (id: string) => request<ServerRequest>(`/server-requests/${encodeURIComponent(id)}/stop`, 'POST'),
  abandon: (id: string) => request<ServerRequest>(`/server-requests/${encodeURIComponent(id)}/abandon`, 'POST', { confirm: true }),
  cancel: (id: string) => request<ServerRequest>(`/server-requests/${encodeURIComponent(id)}/cancel`, 'POST'),
}

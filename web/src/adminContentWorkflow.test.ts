import { describe, expect, it } from 'vitest'
import { canPublishContent, deriveNodeContentProgress, type WorkflowOverview } from './adminContentWorkflow'

function fixture(): WorkflowOverview {
  return {
    games: [{ id: 'game', displayName: '地图', workshopId: '2307479570', currentContentVersionId: 'old', enabled: true, acceptingNewRequests: true }],
    contentVersions: [{ id: 'new', arcadeGameId: 'game', contentSha256: 'a'.repeat(64) }],
    presets: [{ id: 'custom', arcadeGameId: 'game', displayName: '自定义', templateRevisionId: 'custom-300', maxPlayers: 10, enabled: true, acceptingNewRequests: true }],
    nodes: [
      { id: 'linux', displayName: 'Linux', connectivity: 'online', compatibility: 'compatible', enabled: true, acceptingNewRequests: true, draining: false, occupied: 0 },
      { id: 'windows', displayName: 'Windows', connectivity: 'online', compatibility: 'compatible', enabled: true, acceptingNewRequests: true, draining: false, occupied: 0 },
    ],
    bindings: [], contentValidations: [], templateBindings: [], templateRevisions: [{ id: 'custom-300', arcadeGameId: 'game', description: 'custom' }],
  }
}

function binding(data: WorkflowOverview, nodeId: string, version = 'new', digest = 'a'.repeat(64), accepting = false): void {
  data.bindings.push({ nodeId, arcadeGameId: 'game', reportedContentVersionId: version, reportedContentSha256: digest, reportedState: 'confirmed', acceptingNewAllocations: accepting })
}
function validated(data: WorkflowOverview, nodeId: string): void {
  data.contentValidations.push({ nodeId, arcadeGameId: 'game', contentVersionId: 'new', verifiedAt: '2026-09-27T00:00:00Z' })
}
function progress(data: WorkflowOverview, nodeId = 'linux', mode: 'new' | 'update' = 'update') {
  return deriveNodeContentProgress(data, data.nodes.find(node => node.id === nodeId)!, data.games[0], data.contentVersions[0], mode)
}

describe('Admin content workflow derived from existing facts', () => {
  it('starts a new map with missing Controller content binding', () => {
    expect(progress(fixture(), 'linux', 'new').next).toContain('Controller 内容绑定')
  })
  it('asks to pause an old open binding before an update', () => {
    const data = fixture(); binding(data, 'linux', 'old', 'a'.repeat(64), true)
    expect(progress(data).next).toContain('暂停此节点')
    expect(progress(data).reportedVersion).toBe(false)
  })
  it('does not say a reported old version is ready after admission closes', () => {
    const data = fixture(); binding(data, 'linux', 'old')
    expect(progress(data).next).toContain('Content Tool')
    expect(progress(data).complete).toBe(false)
  })
  it('rejects a matching version with a different VPK digest', () => {
    const data = fixture(); binding(data, 'linux', 'new', 'b'.repeat(64))
    expect(progress(data).digestMatches).toBe(false)
    expect(progress(data).canValidate).toBe(false)
    expect(progress(data).next).toContain('SHA256 不匹配')
  })
  it('requires Drain and zero occupancy for content validation', () => {
    const data = fixture(); binding(data, 'linux')
    expect(progress(data).canValidate).toBe(false)
    data.nodes[0].draining = true; data.nodes[0].occupied = 1
    expect(progress(data).canValidate).toBe(false)
    expect(progress(data).next).toContain('等待完整回收')
    data.nodes[0].occupied = 0
    expect(progress(data).canValidate).toBe(true)
  })
  it('requires all new-map template mappings before calling a node ready to publish', () => {
    const data = fixture(); binding(data, 'linux'); validated(data, 'linux')
    expect(progress(data, 'linux', 'new').mappedTemplates).toBe(0)
    expect(progress(data, 'linux', 'new').readyToPublish).toBe(false)
    data.templateBindings.push({ nodeId: 'linux', templateRevisionId: 'custom-300', bindingKey: 'custom-local' })
    expect(progress(data, 'linux', 'new').readyToPublish).toBe(true)
  })
  it('points to publication after validated node resumes', () => {
    const data = fixture(); binding(data, 'linux'); validated(data, 'linux')
    expect(progress(data).next).toContain('发布为新开服版本')
    expect(canPublishContent(data, data.games[0], data.contentVersions[0], 'update', ['linux'])).toBe(true)
  })
  it('keeps validation and publication distinct while drained', () => {
    const data = fixture(); binding(data, 'linux'); validated(data, 'linux'); data.nodes[0].draining = true
    expect(progress(data).readyToPublish).toBe(false)
    expect(progress(data).next).toContain('结束节点维护')
  })
  it('does not treat a disabled node as an eligible publication source', () => {
    const data = fixture(); binding(data, 'linux'); validated(data, 'linux'); data.nodes[0].enabled = false
    expect(progress(data).readyToPublish).toBe(false)
  })
  it('shows publication without silently opening Node admission', () => {
    const data = fixture(); binding(data, 'linux'); validated(data, 'linux'); data.games[0].currentContentVersionId = 'new'
    expect(progress(data).published).toBe(true)
    expect(progress(data).admissionOpen).toBe(false)
    expect(progress(data).next).toContain('打开此节点')
  })
  it('reports two Node readiness states independently', () => {
    const data = fixture(); binding(data, 'linux', 'new', 'a'.repeat(64), true); validated(data, 'linux'); binding(data, 'windows', 'old')
    data.games[0].currentContentVersionId = 'new'
    expect(progress(data, 'linux').complete).toBe(true)
    expect(progress(data, 'windows').complete).toBe(false)
    expect(progress(data, 'windows').next).toContain('Content Tool')
  })
  it('does not use Entry or A2S facts for content readiness', () => {
    const data = fixture(); binding(data, 'linux', 'new', 'a'.repeat(64), true); validated(data, 'linux'); data.games[0].currentContentVersionId = 'new'
    expect(progress(data).complete).toBe(true)
  })
})

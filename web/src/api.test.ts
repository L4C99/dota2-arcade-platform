import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './api'

afterEach(() => vi.unstubAllGlobals())

describe('player API observation time', () => {
  it('uses the response server clock for a confirmed pending state', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ id: 'request', state: 'creating' }), {
      status: 200,
      headers: { 'Content-Type': 'application/json', Date: 'Sat, 26 Sep 2026 10:00:20 GMT' },
    })))
    let observed = 0
    const request = await api.current((serverMs) => { observed = serverMs })
    expect(request?.state).toBe('creating')
    expect(observed).toBe(Date.parse('2026-09-26T10:00:20Z'))
  })
})

import { describe, expect, it } from 'vitest'
import { inviteTokenFromHash, partyInviteLink, tokenFromInviteInput } from './partyInvite'

describe('Party invite link', () => {
  it('keeps the credential out of the HTTP request target and query', () => {
    const token = 'A'.repeat(43)
    const url = new URL(partyInviteLink('https://platform.example.org', token))
    expect(url.pathname).toBe('/')
    expect(url.search).toBe('')
    expect(url.hash).toBe(`#invite=${token}`)
    expect(inviteTokenFromHash(url.hash)).toBe(token)
    expect(tokenFromInviteInput(url.href)).toBe(token)
  })
})

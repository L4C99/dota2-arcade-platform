export function inviteTokenFromHash(hash: string): string {
  return new URLSearchParams(hash.replace(/^#/, '')).get('invite') || ''
}

export function partyInviteLink(origin: string, token: string): string {
  return `${origin}/#invite=${encodeURIComponent(token)}`
}

export function tokenFromInviteInput(value: string): string {
  try { return inviteTokenFromHash(new URL(value).hash) || value.trim() }
  catch { return value.trim() }
}

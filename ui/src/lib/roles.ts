import { useQuery } from '@tanstack/react-query'
import { api } from './api'

export type Role = 'owner' | 'member' | 'guest'

// What each person sees in the menus. The server decides what they may
// do; this only hides what would answer "only the owner".
const ownerOnly = ['/skills', '/connections', '/people', '/rules', '/settings']
const forGuests = ['/', '/chat', '/memory', '/inbox', '/help']

export function canOpen(path: string, role: Role): boolean {
  if (role === 'owner') return true
  if (role === 'guest') return forGuests.some((p) => path === p || path.startsWith(p + '/'))
  return !ownerOnly.some((p) => path === p || path.startsWith(p + '/'))
}

export function useRole(): Role {
  const state = useQuery({ queryKey: ['state'], queryFn: api.state })
  return (state.data?.role as Role | undefined) ?? 'owner'
}

// Route names shown in the sidebar, in order. The menu stays empty until the
// first profile exists so the WelcomeCard is the only call to action;
// Settings remains reachable through the header gear in that state.
export const SIDEBAR_KEYS = ['search', 'papers', 'analysis', 'settings'] as const

export type SidebarKey = typeof SIDEBAR_KEYS[number]

export function sidebarKeys(profiles: unknown[] | null | undefined): SidebarKey[] {
  return (profiles?.length ?? 0) > 0 ? [...SIDEBAR_KEYS] : []
}

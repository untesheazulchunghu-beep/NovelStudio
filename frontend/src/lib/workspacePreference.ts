export function readActiveWorkspacePreference() {
  try {
    return typeof window === 'undefined' ? null : window.localStorage.getItem('novelstudio.activeWorkspace')
  } catch {
    return null
  }
}

export function rememberActiveWorkspacePreference(workspaceId: string) {
  try {
    window.localStorage.setItem('novelstudio.activeWorkspace', workspaceId)
  } catch {
    // Remembering the selection is optional; the workspace itself is stored by the backend.
  }
}

export type EditorFontFamily = 'serif' | 'sans' | 'mono'
export type ThemePreference = 'dark' | 'light'

export interface EditorPreferences {
  fontSize: number
  fontFamily: EditorFontFamily
  focusMode: boolean
  theme: ThemePreference
}

export const defaultEditorPreferences: EditorPreferences = {
  fontSize: 14,
  fontFamily: 'serif',
  focusMode: false,
  theme: 'dark',
}

const storageKey = 'novelstudio.editorPreferences'
const fontSizes = [13, 14, 15, 16, 18, 20]

export function readEditorPreferences(): EditorPreferences {
  if (typeof window === 'undefined') return defaultEditorPreferences
  try {
    const raw = window.localStorage.getItem(storageKey)
    if (!raw) return defaultEditorPreferences
    const parsed = JSON.parse(raw) as Partial<EditorPreferences>
    const fontSize = typeof parsed.fontSize === 'number' && fontSizes.includes(parsed.fontSize) ? parsed.fontSize : defaultEditorPreferences.fontSize
    const fontFamily = parsed.fontFamily === 'serif' || parsed.fontFamily === 'sans' || parsed.fontFamily === 'mono'
      ? parsed.fontFamily
      : defaultEditorPreferences.fontFamily
    const focusMode = typeof parsed.focusMode === 'boolean' ? parsed.focusMode : defaultEditorPreferences.focusMode
    const theme = parsed.theme === 'light' || parsed.theme === 'dark' ? parsed.theme : defaultEditorPreferences.theme
    return { fontSize, fontFamily, focusMode, theme }
  } catch {
    return defaultEditorPreferences
  }
}

export function saveEditorPreferences(preferences: EditorPreferences) {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(storageKey, JSON.stringify(preferences))
  } catch {
    // Preferences are optional; private browsing or a full storage quota
    // should not prevent the editor from loading.
  }
}

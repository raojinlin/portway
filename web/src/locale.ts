import english from './locales/en.json'

export type Language = 'zh' | 'en'
export type LanguagePreference = 'system' | Language
export type MessageKey = keyof typeof english
export type Translate = (key: MessageKey, values?: Record<string, string | number>) => string
export const LANGUAGE_STORAGE_KEY = 'portway-language'

export function resolveLanguage(preference: LanguagePreference, languages: readonly string[]): Language {
  if (preference !== 'system') return preference
  for (const language of languages) {
    if (/^zh(?:-|_|$)/i.test(language)) return 'zh'
    if (/^en(?:-|_|$)/i.test(language)) return 'en'
  }
  return 'en'
}

export function loadLanguagePreference(): LanguagePreference {
  try {
    const saved = window.localStorage.getItem(LANGUAGE_STORAGE_KEY)
    if (saved === 'zh' || saved === 'en') return saved
  } catch { /* Storage restrictions must not prevent startup. */ }
  return 'system'
}

export function translator(language: Language): Translate {
  return (key, values = {}) => (language === 'en' ? english[key] : key)
    .replace(/\{(\w+)\}/g, (placeholder, name: string) => String(values[name] ?? placeholder))
}

import IntlMessageFormat, { type MessageValues } from 'intl-messageformat'
import { computed, ref } from 'vue'
import { localeManifest, messageCatalogs } from './generated'

export type LocaleCode = keyof typeof messageCatalogs
export type LocalePreference = 'system' | LocaleCode
export type MessageKey = keyof (typeof messageCatalogs)['en']

const storageKey = 'agentdock.shared-ui.locale'
const sourceLocale = localeManifest.sourceLocale as LocaleCode
const localeByAlias = new Map<string, LocaleCode>()

for (const locale of localeManifest.locales) {
  localeByAlias.set(locale.code.toLowerCase(), locale.code as LocaleCode)
  for (const alias of locale.aliases) {
    localeByAlias.set(alias.toLowerCase(), locale.code as LocaleCode)
  }
}

function canonicalLocale(raw: string | null | undefined): LocaleCode | null {
  if (!raw) return null
  const normalized = raw.trim().toLowerCase()
  const exact = localeByAlias.get(normalized)
  if (exact) return exact

  const language = normalized.split('-')[0]
  if (language === 'zh') {
    if (
      normalized.includes('hant') ||
      normalized.includes('-tw') ||
      normalized.includes('-hk') ||
      normalized.includes('-mo')
    ) {
      return 'zh-Hant'
    }
    return 'zh-Hans'
  }
  return localeByAlias.get(language) ?? null
}

function systemLocale(): LocaleCode {
  if (typeof navigator === 'undefined') return sourceLocale
  const candidates = navigator.languages?.length ? navigator.languages : [navigator.language]
  for (const candidate of candidates) {
    const locale = canonicalLocale(candidate)
    if (locale) return locale
  }
  return sourceLocale
}

function savedPreference(): LocalePreference {
  if (typeof localStorage === 'undefined') return 'system'
  try {
    const raw = localStorage.getItem(storageKey)
    if (!raw || raw === 'system') return 'system'
    return canonicalLocale(raw) ?? 'system'
  } catch {
    return 'system'
  }
}

export const currentPreference = ref<LocalePreference>(savedPreference())
export const currentLocale = ref<LocaleCode>(
  currentPreference.value === 'system' ? systemLocale() : currentPreference.value,
)

export const localeOptions = computed(() =>
  localeManifest.locales.map((locale) => ({
    code: locale.code as LocaleCode,
    nativeName: locale.nativeName,
    direction: locale.direction,
    status: locale.status,
  })),
)

function messageFor(locale: LocaleCode, key: MessageKey): string {
  const catalog = messageCatalogs[locale] as Partial<Record<MessageKey, string>>
  const direct = catalog[key]
  if (direct) return direct

  const descriptor = localeManifest.locales.find((item) => item.code === locale)
  if (descriptor?.fallback) {
    const fallback = canonicalLocale(descriptor.fallback)
    if (fallback && fallback !== locale) return messageFor(fallback, key)
  }

  return messageCatalogs[sourceLocale][key] ?? key
}

function applyLocale(locale: LocaleCode) {
  currentLocale.value = locale
  if (typeof document !== 'undefined') {
    document.documentElement.lang = locale
    const descriptor = localeManifest.locales.find((item) => item.code === locale)
    document.documentElement.dir = descriptor?.direction ?? 'ltr'
  }
}

export function setLocale(preference: LocalePreference) {
  currentPreference.value = preference
  applyLocale(preference === 'system' ? systemLocale() : preference)

  if (typeof localStorage !== 'undefined') {
    try {
      if (preference === 'system') {
        localStorage.removeItem(storageKey)
      } else {
        localStorage.setItem(storageKey, preference)
      }
    } catch {
      // Storage is a preference optimization; locale switching must still work without it.
    }
  }
}

export function t(key: MessageKey, values?: MessageValues): string {
  const locale = currentLocale.value
  const formatter = new IntlMessageFormat(messageFor(locale, key), locale)
  const result = formatter.format(values)
  return Array.isArray(result) ? result.join('') : String(result)
}

export function useI18n() {
  return {
    currentPreference,
    currentLocale,
    localeOptions,
    setLocale,
    t,
  }
}

applyLocale(currentLocale.value)

if (typeof window !== 'undefined') {
  window.addEventListener('languagechange', () => {
    if (currentPreference.value === 'system') {
      applyLocale(systemLocale())
    }
  })
}

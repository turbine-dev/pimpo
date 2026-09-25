import { useQuery } from '@tanstack/react-query'
import { createContext, Fragment, useContext, useEffect, useMemo, type ReactNode } from 'react'
import { api } from './api'
import { de } from './locales/de'
import { en } from './locales/en'
import { es } from './locales/es'
import { fr } from './locales/fr'
import { it } from './locales/it'
import { ja } from './locales/ja'
import { ko } from './locales/ko'
import { pt, type Key } from './locales/pt'
import { ru } from './locales/ru'
import { zh } from './locales/zh'

export type Locale = 'pt' | 'en' | 'es' | 'fr' | 'de' | 'it' | 'ja' | 'zh' | 'ko' | 'ru'
export type Vars = Record<string, string | number>

// Plural keys are written as `name_one` and `name_other` (and `name_few`,
// `name_many` where the language has them) and called as `name` with a
// numeric `count`.
type PluralBase<K> = K extends `${infer B}_one` ? B : never
export type TKey = Key | PluralBase<Key>

// Dict is a full translation: every key of the Portuguese source, plus any
// extra plural forms the language needs.
export type Dict = Record<Key, string> & Record<string, string>

const dicts: Record<Locale, Dict> = { pt, en, es, fr, de, it, ja, zh, ko, ru }

// The languages offered, each named in itself.
export const LANGUAGES: { tag: string; locale: Locale; name: string }[] = [
  { tag: 'pt-BR', locale: 'pt', name: 'Português' },
  { tag: 'en-US', locale: 'en', name: 'English' },
  { tag: 'es-ES', locale: 'es', name: 'Español' },
  { tag: 'fr-FR', locale: 'fr', name: 'Français' },
  { tag: 'de-DE', locale: 'de', name: 'Deutsch' },
  { tag: 'it-IT', locale: 'it', name: 'Italiano' },
  { tag: 'ja-JP', locale: 'ja', name: '日本語' },
  { tag: 'zh-CN', locale: 'zh', name: '简体中文' },
  { tag: 'ko-KR', locale: 'ko', name: '한국어' },
  { tag: 'ru-RU', locale: 'ru', name: 'Русский' },
]

export function localeFrom(tag?: string): Locale {
  const lang = (tag ?? '').toLowerCase().split(/[-_]/)[0]
  return LANGUAGES.find((l) => l.locale === lang)?.locale ?? 'en'
}

export function localeTag(l: Locale = current): string {
  return LANGUAGES.find((x) => x.locale === l)?.tag ?? 'en-US'
}

const rules = new Map<Locale, Intl.PluralRules>()
function pluralOf(locale: Locale, n: number): string {
  let r = rules.get(locale)
  if (!r) rules.set(locale, (r = new Intl.PluralRules(localeTag(locale))))
  return r.select(n)
}

export function translate(locale: Locale, key: TKey, vars?: Vars): string {
  const d = dicts[locale]
  let s: string | undefined
  if (typeof vars?.count === 'number') {
    const cat = pluralOf(locale, vars.count)
    for (const dict of [d, en as Dict]) {
      s = dict[`${key}_${cat}`] ?? dict[`${key}_${vars.count === 1 ? 'one' : 'other'}`]
      if (s) break
    }
  }
  s ??= d[key] ?? en[key as Key] ?? pt[key as Key] ?? key
  return vars ? s.replace(/\{(\w+)\}/g, (m, k: string) => (k in vars ? String(vars[k]) : m)) : s
}

export function hasKey(key: string): key is Key {
  return key in pt
}

// fill places React nodes where a translated string has {name} slots,
// for links and code inside a sentence.
export function fill(s: string, nodes: Record<string, ReactNode>): ReactNode {
  return s.split(/\{(\w+)\}/).map((part, i) => <Fragment key={i}>{i % 2 ? (nodes[part] ?? `{${part}}`) : part}</Fragment>)
}

// The locale for code outside components (formatting, class components).
// LocaleProvider keeps it in step with the context; without one it is pt.
let current: Locale = 'pt'

export const currentLocale = () => current

export function tr(key: TKey, vars?: Vars) {
  return translate(current, key, vars)
}

export type T = ((key: TKey, vars?: Vars) => string) & { locale: Locale }

const Ctx = createContext<Locale>('pt')

// LocaleProvider follows the saved setting, then the browser. A fixed
// `locale` skips both, for tests and previews.
export function LocaleProvider({ locale: fixed, children }: { locale?: Locale; children: ReactNode }) {
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings, enabled: !fixed })
  const locale = fixed ?? (settings.data?.locale ? localeFrom(settings.data.locale) : localeFrom(typeof navigator === 'undefined' ? undefined : navigator.language))
  current = locale
  useEffect(() => {
    document.documentElement.lang = localeTag(locale)
  }, [locale])
  return <Ctx.Provider value={locale}>{children}</Ctx.Provider>
}

export function useT(): T {
  const locale = useContext(Ctx)
  return useMemo(() => Object.assign((key: TKey, vars?: Vars) => translate(locale, key, vars), { locale }), [locale])
}

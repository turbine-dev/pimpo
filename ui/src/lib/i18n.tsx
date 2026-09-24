import { useQuery } from '@tanstack/react-query'
import { createContext, Fragment, useContext, useEffect, useMemo, type ReactNode } from 'react'
import { api } from './api'
import { en } from './locales/en'
import { pt, type Key } from './locales/pt'

export type Locale = 'pt' | 'en'
export type Vars = Record<string, string | number>

// Plural keys are written as `name_one` and `name_other` and called as `name`
// with a numeric `count`.
type PluralBase<K> = K extends `${infer B}_one` ? B : never
export type TKey = Key | PluralBase<Key>

const dicts: Record<Locale, Record<Key, string>> = { pt, en }

export function localeFrom(tag?: string): Locale {
  return tag?.toLowerCase().startsWith('pt') ? 'pt' : 'en'
}

export function localeTag(l: Locale = current): string {
  return l === 'pt' ? 'pt-BR' : 'en-US'
}

export function translate(locale: Locale, key: TKey, vars?: Vars): string {
  const d = dicts[locale]
  let s: string | undefined
  if (typeof vars?.count === 'number') s = d[`${key}_${vars.count === 1 ? 'one' : 'other'}` as Key]
  s ??= d[key as Key] ?? key
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

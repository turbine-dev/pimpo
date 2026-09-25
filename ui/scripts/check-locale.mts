// Checks a translation against the Portuguese source: the same keys, the
// same {slots}, and every plural form the language needs.
// Usage: node scripts/check-locale.mts es
import { pt } from '../src/lib/locales/pt.ts'

const code = process.argv[2]
const mod = await import(`../src/lib/locales/${code}.ts`)
const dict: Record<string, string> = mod[code]
const tags: Record<string, string> = { en: 'en-US', es: 'es-ES', fr: 'fr-FR', de: 'de-DE', it: 'it-IT', ja: 'ja-JP', zh: 'zh-CN', ko: 'ko-KR', ru: 'ru-RU' }
const slots = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort().join(',')
const problems: string[] = []
const source = pt as Record<string, string>
for (const k of Object.keys(source)) {
  if (!(k in dict)) problems.push(`missing ${k}`)
  else if (typeof dict[k] !== 'string' || !dict[k].trim()) problems.push(`empty ${k}`)
  else if (slots(dict[k]) !== slots(source[k])) problems.push(`slots differ in ${k}: ${slots(dict[k])} vs ${slots(source[k])}`)
}
const bases = Object.keys(source).filter((k) => k.endsWith('_one')).map((k) => k.slice(0, -4))
const cats = new Set(new Intl.PluralRules(tags[code]).resolvedOptions().pluralCategories)
for (const k of Object.keys(dict)) {
  if (k in source) continue
  const m = k.match(/^(.*)_(zero|two|few|many)$/)
  if (!m || !bases.includes(m[1]) || !cats.has(m[2] as Intl.LDMLPluralRule)) problems.push(`unknown key ${k}`)
  else if (slots(dict[k]) !== slots(source[m[1] + '_other'])) problems.push(`slots differ in ${k}`)
}
for (const b of bases) for (const c of cats) if (c !== 'one' && c !== 'other' && !(`${b}_${c}` in dict)) problems.push(`missing plural ${b}_${c}`)
if (problems.length) {
  console.log(problems.join('\n'))
  process.exit(1)
}
console.log(`${code}: ${Object.keys(dict).length} keys, all good`)

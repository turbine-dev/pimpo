import type { Org } from './api'

// below is every member under id, for not letting someone report to
// their own report.
export function below(org: Org, id: string): Set<string> {
  const out = new Set<string>()
  const walk = (boss: string) => {
    for (const m of org.members) {
      if (m.reports_to === boss && !out.has(m.id)) {
        out.add(m.id)
        walk(m.id)
      }
    }
  }
  walk(id)
  return out
}

export function deptColor(org: Org, id?: string): string | undefined {
  const d = org.departments.find((x) => x.id === id)
  return d ? `var(--color-${d.color || 'line-strong'})` : undefined
}

export function slug(name: string, fallback: string) {
  return name.toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '').replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 40) || fallback
}

// unique gives a new thing an id no other thing of its kind has.
export function unique(base: string, taken: string[]) {
  let id = base
  for (let i = 2; taken.includes(id); i++) id = `${base.slice(0, 36)}-${i}`
  return id
}

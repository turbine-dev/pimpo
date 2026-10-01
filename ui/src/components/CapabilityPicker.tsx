import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import { capabilityLabel } from './RoutineCard'
import { RiskBadge } from './ui'

// CapabilityPicker lists every capability by family, to choose some.
export function CapabilityPicker({ value, onChange }: { value: string[]; onChange: (v: string[]) => void }) {
  const caps = useQuery({ queryKey: ['capabilities'], queryFn: api.capabilities })
  const groups = new Map<string, NonNullable<typeof caps.data>>()
  for (const c of caps.data ?? []) {
    const g = c.name.split('.')[0]
    groups.set(g, [...(groups.get(g) ?? []), c])
  }
  const toggle = (name: string) => onChange(value.includes(name) ? value.filter((x) => x !== name) : [...value, name])
  return (
    <div className="max-h-72 space-y-3 overflow-y-auto rounded-xl border border-line p-3">
      {[...groups.entries()].map(([g, list]) => (
        <div key={g}>
          <div className="mb-1 text-[11.5px] font-medium uppercase tracking-wide text-ink-3">{g}</div>
          {list.map((c) => (
            <label key={c.name} className="flex items-center gap-2 py-0.5 text-[13px]">
              <input type="checkbox" className="size-4 accent-[var(--color-accent)]" checked={value.includes(c.name)} onChange={() => toggle(c.name)} />
              <span className="flex-1">{capabilityLabel(c.name)} <code className="text-[11.5px] text-ink-3">{c.name}</code></span>
              <RiskBadge risk={c.risk} />
            </label>
          ))}
        </div>
      ))}
    </div>
  )
}

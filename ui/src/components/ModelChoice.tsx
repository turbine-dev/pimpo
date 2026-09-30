import { useT } from '../lib/i18n'
import { label, useModelOptions } from './ModelSetup'

// ModelChoice is the owner choosing which of the house's models someone
// (a person or an assistant) may use: all of them, or some. null is all.
export function ModelChoice({ value, onChange, legend }: { value: string[] | null; onChange: (v: string[] | null) => void; legend: string }) {
  const t = useT()
  const { house } = useModelOptions()
  // A model chosen before and since removed from the house still shows,
  // so it can be unticked.
  const list = [...house, ...(value ?? []).filter((m) => !house.includes(m))]
  const toggle = (m: string) => {
    const now = value ?? []
    onChange(now.includes(m) ? now.filter((x) => x !== m) : [...now, m])
  }
  return (
    <fieldset className="space-y-1.5">
      <legend className="mb-1 text-[13px] font-medium text-ink-2">{legend}</legend>
      <label className="flex items-center gap-2 text-[13px]">
        <input type="checkbox" className="size-4 accent-[var(--color-accent)]" checked={value === null} onChange={(e) => onChange(e.target.checked ? null : [])} /> {t('lim.allModels')}
      </label>
      {value !== null && (
        <div className="space-y-0.5 rounded-xl border border-line p-3">
          {list.length === 0 && <p className="text-[12.5px] text-ink-3">{t('lim.noHouseModels')}</p>}
          {list.map((m) => (
            <label key={m} className="flex items-center gap-2 py-0.5 text-[13px]">
              <input type="checkbox" className="size-4 accent-[var(--color-accent)]" checked={value.includes(m)} onChange={() => toggle(m)} />
              <span className="flex-1 truncate">{label(m)}</span>
            </label>
          ))}
        </div>
      )}
      {value !== null && value.length === 0 && <p className="text-[12.5px] text-ink-3">{t('lim.none')}</p>}
    </fieldset>
  )
}

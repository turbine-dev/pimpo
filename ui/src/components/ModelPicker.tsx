import * as Menu from '@radix-ui/react-dropdown-menu'
import { Check, ChevronDown, Sparkles } from 'lucide-react'
import { cn } from '../lib/cn'
import { useT } from '../lib/i18n'
import { label, useModelOptions } from './ModelSetup'

// ModelPicker chooses which model answers a conversation: automatic, where
// Pimpo weighs each request, or one fixed model.
export function ModelPicker({ value, onChange, className }: { value: string; onChange: (model: string) => void; className?: string }) {
  const t = useT()
  const { options, auto } = useModelOptions()
  const current = !value || value === 'auto' ? 'auto' : value
  const all = options.includes(current) || current === 'auto' ? options : [current, ...options]
  const item = 'flex cursor-default items-start gap-2 rounded-lg px-2.5 py-2 text-[13px] outline-none data-[highlighted]:bg-sunken'
  return (
    <Menu.Root>
      <Menu.Trigger aria-label={t('mp.label')}
        className={cn('inline-flex h-7 max-w-[220px] items-center gap-1 rounded-full px-2.5 text-[12px] text-ink-2 transition hover:bg-sunken hover:text-ink data-[state=open]:bg-sunken', className)}>
        {current === 'auto' && <Sparkles size={12} className="shrink-0" />}
        <span className="truncate">{current === 'auto' ? t('mp.auto') : label(current)}</span>
        <ChevronDown size={12} className="shrink-0 opacity-60" />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content side="top" align="start" sideOffset={6}
          className="z-50 max-h-[60vh] w-72 overflow-y-auto rounded-xl border border-line bg-surface p-1.5 shadow-[var(--shadow-pop)]">
          <Menu.RadioGroup value={current} onValueChange={onChange}>
            <Menu.RadioItem value="auto" className={item}>
              <Sparkles size={14} className="mt-0.5 shrink-0" />
              <span className="min-w-0 flex-1">
                <span className="block font-medium">{t('mp.auto')}</span>
                <span className="block text-[11.5px] text-ink-3">
                  {auto && (auto.light || auto.strong)
                    ? t('mp.autoText', { light: label(auto.light || auto.base), strong: label(auto.strong || auto.base) })
                    : t('mp.autoPlain')}
                </span>
              </span>
              <Menu.ItemIndicator><Check size={14} className="mt-0.5" /></Menu.ItemIndicator>
            </Menu.RadioItem>
            {all.length > 0 && <Menu.Separator className="my-1 h-px bg-line" />}
            {all.map((o) => (
              <Menu.RadioItem key={o} value={o} className={item}>
                <span className="min-w-0 flex-1 truncate">{label(o)}</span>
                <Menu.ItemIndicator><Check size={14} /></Menu.ItemIndicator>
              </Menu.RadioItem>
            ))}
          </Menu.RadioGroup>
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  )
}

// useRoutedText says under a reply which model answered and why.
export function useRoutedText() {
  const t = useT()
  return (r?: { model: string; tier?: string; by: string }) => {
    if (!r?.model) return ''
    const name = label(r.model)
    if (r.by === 'fixed') return t('mp.byFixed', { model: name })
    if (r.by === 'default' || !r.tier) return name
    return t(r.tier === 'simple' ? 'mp.bySimple' : r.tier === 'hard' ? 'mp.byHard' : 'mp.byNormal', { model: name })
  }
}

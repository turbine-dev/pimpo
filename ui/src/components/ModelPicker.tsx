import * as Menu from '@radix-ui/react-dropdown-menu'
import { AlertTriangle, Brain, Check, ChevronDown, Sparkles } from 'lucide-react'
import { EFFORTS, type Routed } from '../lib/api'
import { cn } from '../lib/cn'
import { useT, type TKey } from '../lib/i18n'
import { label, RetiredHint, useModelOptions, useRetired } from './ModelSetup'

const levelKey = (e: string) => `effort.${e}` as TKey

// ModelPicker chooses which model answers a conversation and how hard it
// thinks: automatic, where Pimpo weighs each request, or fixed.
// allowed narrows the list further, to what the conversation's assistant
// may use.
export function ModelPicker({ value, effort = 'auto', onChange, onEffort, className, allowed }: {
  value: string; effort?: string; onChange: (model: string) => void; onEffort?: (effort: string) => void; className?: string; allowed?: string[]
}) {
  const t = useT()
  const { options: mine, auto } = useModelOptions()
  const options = allowed?.length ? mine.filter((m) => allowed.includes(m)) : mine
  const retired = useRetired()
  const current = !value || value === 'auto' ? 'auto' : value
  const level = !effort || effort === 'auto' ? 'auto' : effort
  const all = options.includes(current) || current === 'auto' ? options : [current, ...options]
  const item = 'flex cursor-default items-start gap-2 rounded-lg px-2.5 py-2 text-[13px] outline-none data-[highlighted]:bg-sunken'
  const heading = 'px-2.5 pb-1 pt-2 text-[11px] font-medium uppercase tracking-wide text-ink-3'
  const bothAuto = current === 'auto' && level === 'auto'
  return (
    <Menu.Root>
      <Menu.Trigger aria-label={t('mp.label')}
        className={cn('inline-flex h-7 max-w-[260px] items-center gap-1 rounded-full px-2.5 text-[12px] text-ink-2 transition hover:bg-sunken hover:text-ink data-[state=open]:bg-sunken', className)}>
        {bothAuto && <Sparkles size={12} className="shrink-0" />}
        {retired.has(current) && <AlertTriangle size={12} className="shrink-0 text-danger" aria-label={t('ms.retired')} />}
        <span className="truncate">
          {current === 'auto' ? t('mp.auto') : label(current)}
          {level !== 'auto' && <span className="text-ink-3"> · {t(levelKey(level))}</span>}
        </span>
        <ChevronDown size={12} className="shrink-0 opacity-60" />
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Content side="top" align="start" sideOffset={6}
          className="z-50 max-h-[70vh] w-72 overflow-y-auto rounded-xl border border-line bg-surface p-1.5 shadow-[var(--shadow-pop)]">
          <Menu.Label className={heading}>{t('mp.model')}</Menu.Label>
          <RetiredHint model={current} className="px-2.5 pb-1" />
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
            {all.map((o) => (
              <Menu.RadioItem key={o} value={o} className={item}>
                <span className="min-w-0 flex-1 truncate">{label(o)}</span>
                {retired.has(o) && <span className="rounded-full bg-danger-soft px-1.5 text-[10.5px] font-medium text-danger">{t('ms.retired')}</span>}
                <Menu.ItemIndicator><Check size={14} /></Menu.ItemIndicator>
              </Menu.RadioItem>
            ))}
          </Menu.RadioGroup>
          {onEffort && (
            <>
              <Menu.Separator className="my-1 h-px bg-line" />
              <Menu.Label className={heading}>{t('mp.effort')}</Menu.Label>
              <Menu.RadioGroup value={level} onValueChange={onEffort}>
                <Menu.RadioItem value="auto" className={item}>
                  <Brain size={14} className="mt-0.5 shrink-0" />
                  <span className="min-w-0 flex-1">
                    <span className="block font-medium">{t('mp.auto')}</span>
                    <span className="block text-[11.5px] text-ink-3">{t('mp.effortAuto')}</span>
                  </span>
                  <Menu.ItemIndicator><Check size={14} className="mt-0.5" /></Menu.ItemIndicator>
                </Menu.RadioItem>
                {EFFORTS.map((e) => (
                  <Menu.RadioItem key={e} value={e} className={item}>
                    <span className="min-w-0 flex-1">
                      <span className="block first-letter:uppercase">{t(levelKey(e))}</span>
                      <span className="block text-[11.5px] text-ink-3">{t(`mp.effort.${e}` as TKey)}</span>
                    </span>
                    <Menu.ItemIndicator><Check size={14} className="mt-0.5" /></Menu.ItemIndicator>
                  </Menu.RadioItem>
                ))}
              </Menu.RadioGroup>
            </>
          )}
        </Menu.Content>
      </Menu.Portal>
    </Menu.Root>
  )
}

// useRoutedText says under a reply which model answered, how hard it
// thought, and why.
export function useRoutedText() {
  const t = useT()
  return (r?: Routed) => {
    if (!r?.model) return ''
    const name = label(r.model)
    const model = r.by === 'fixed' ? t('mp.byFixed', { model: name })
      : r.by === 'allowed' ? t('mp.byAllowed', { model: name })
      : r.by === 'default' || !r.tier ? name
      : t(r.tier === 'simple' ? 'mp.bySimple' : r.tier === 'hard' ? 'mp.byHard' : 'mp.byNormal', { model: name })
    return r.effort ? `${model} · ${t('mp.thinks', { level: t(levelKey(r.effort)) })}` : model
  }
}

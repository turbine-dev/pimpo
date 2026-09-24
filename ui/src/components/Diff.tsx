import { cn } from '../lib/cn'

type Line = { kind: 'same' | 'add' | 'del'; text: string }

// diffLines is a longest-common-subsequence line diff, fine for routines
// that are tens of lines long.
export function diffLines(a: string, b: string): Line[] {
  const x = a.split('\n')
  const y = b.split('\n')
  const dp = Array.from({ length: x.length + 1 }, () => new Array<number>(y.length + 1).fill(0))
  for (let i = x.length - 1; i >= 0; i--) for (let j = y.length - 1; j >= 0; j--) dp[i][j] = x[i] === y[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1])
  const out: Line[] = []
  let i = 0
  let j = 0
  while (i < x.length && j < y.length) {
    if (x[i] === y[j]) {
      out.push({ kind: 'same', text: x[i] })
      i++
      j++
    } else if (dp[i + 1][j] >= dp[i][j + 1]) out.push({ kind: 'del', text: x[i++] })
    else out.push({ kind: 'add', text: y[j++] })
  }
  while (i < x.length) out.push({ kind: 'del', text: x[i++] })
  while (j < y.length) out.push({ kind: 'add', text: y[j++] })
  return out
}

export function Diff({ before, after }: { before: string; after: string }) {
  return (
    <pre className="overflow-x-auto rounded-xl border border-line bg-sunken py-2 font-mono text-[12px] leading-[1.65] [font-variant-ligatures:none]">
      {diffLines(before, after).map((l, i) => (
        <div key={i} className={cn('flex px-3', l.kind === 'add' && 'bg-read-soft', l.kind === 'del' && 'bg-danger-soft')}>
          <span className={cn('w-4 shrink-0 select-none', l.kind === 'add' ? 'text-read' : l.kind === 'del' ? 'text-danger' : 'text-ink-3/50')}>{l.kind === 'add' ? '+' : l.kind === 'del' ? '−' : ' '}</span>
          <code className="whitespace-pre">{l.text}</code>
        </div>
      ))}
    </pre>
  )
}

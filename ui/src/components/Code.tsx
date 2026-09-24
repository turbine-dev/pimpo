import { Fragment } from 'react'

// A tiny highlighter: capability calls stand out, since they are the only
// way a routine reaches the world.
const token = /(\/\/.*$)|("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|`(?:[^`\\]|\\.)*`)|\b(async|await|function|const|let|var|if|else|for|of|in|return|new|throw|try|catch|continue|break|true|false|null)\b|\b((?:calendar|gmail|http|telegram|judge)\.[A-Za-z]+)\b|\b((?:dates|money)\.[A-Za-z]+|now|log)\b/gm

export function Code({ code }: { code: string }) {
  const lines = code.replace(/\t/g, '  ').split('\n')
  return (
    <pre className="overflow-x-auto rounded-xl border border-line bg-sunken py-3 font-mono text-[12.5px] leading-[1.7] [font-variant-ligatures:none]">
      {lines.map((line, i) => (
        <div key={i} className="flex">
          <span className="w-10 shrink-0 select-none pr-3 text-right text-ink-3/60">{i + 1}</span>
          <code className="whitespace-pre pr-4">{highlight(line)}</code>
        </div>
      ))}
    </pre>
  )
}

function highlight(line: string) {
  const out = []
  let last = 0
  for (const m of line.matchAll(token)) {
    if (m.index! > last) out.push(line.slice(last, m.index))
    const [text, comment, str, kw, cap, lib] = m
    const cls = comment ? 'text-ink-3 italic' : str ? 'text-read' : kw ? 'text-accent' : cap ? 'rounded bg-change-soft px-0.5 text-change' : lib ? 'text-explore' : ''
    out.push(
      <span key={m.index} className={cls}>
        {text}
      </span>,
    )
    last = m.index! + text.length
  }
  out.push(line.slice(last))
  return out.map((p, i) => <Fragment key={i}>{p}</Fragment>)
}

import { describe, expect, it } from 'vitest'
import { afterWake, step, type Levels } from '../lib/conversation'

describe('wake word', () => {
  it('hears its name the ways Whisper writes it', () => {
    expect(afterWake('Pimpo, como está o tempo amanhã?')).toBe('como está o tempo amanhã?')
    expect(afterWake('Ei Pimpo! Liga pra casa')).toBe('Liga pra casa')
    expect(afterWake('Hey pimpão what is on today')).toBe('what is on today')
    expect(afterWake('Olá Pimpo')).toBe('')
    expect(afterWake('pimpo')).toBe('')
  })
  it('ignores talk that does not start with it', () => {
    expect(afterWake('eu falei com o Pimpo ontem')).toBeNull()
    expect(afterWake('pimpolho')).toBeNull()
    expect(afterWake('')).toBeNull()
  })
})

describe('voice activity', () => {
  const run = (levels: number[], strict = false) => {
    let l: Levels = { floor: 0.01, speaking: false, loud: 0, quiet: 0 }
    const events: string[] = []
    for (const r of levels) {
      const s = step(l, r, strict)
      l = s.l
      if (s.started) events.push('start')
      if (s.ended) events.push('end')
    }
    return events
  }
  it('starts on sustained speech and ends after a pause', () => {
    expect(run([...Array(20).fill(0.005), ...Array(10).fill(0.2), ...Array(20).fill(0.005)])).toEqual(['start', 'end'])
  })
  it('does not start on a click', () => {
    expect(run([...Array(20).fill(0.005), 0.3, 0.005, 0.3, ...Array(20).fill(0.005)])).toEqual([])
  })
  it('needs louder speech to interrupt while speaking', () => {
    const quietVoice = [...Array(20).fill(0.005), ...Array(10).fill(0.03)]
    expect(run(quietVoice)).toEqual(['start'])
    expect(run(quietVoice, true)).toEqual([])
  })
})

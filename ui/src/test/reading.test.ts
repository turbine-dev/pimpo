import { sentences } from '../lib/voice'

describe('reading aloud a piece at a time', () => {
  it('starts with a short piece and keeps sentences whole', () => {
    const text = 'Bom dia! Hoje há três histórias. A primeira fala de um modelo aberto que venceu um teste de programação, e a comunidade discute a memória que ele usa. A segunda é sobre o SQLite. A terceira, sobre o Postgres 19, que saiu ontem com novidades.'
    const parts = sentences(text)
    expect(parts[0]).toBe('Bom dia! Hoje há três histórias.')
    expect(parts.join(' ')).toBe(text)
    expect(parts.every((p) => p.length <= 320)).toBe(true)
    expect(parts.length).toBeGreaterThan(1)
  })
  it('cuts a very long sentence at spaces', () => {
    const long = 'palavra '.repeat(200).trim()
    const parts = sentences(long)
    expect(parts.length).toBeGreaterThan(1)
    expect(parts.join(' ').replace(/\s+/g, ' ')).toBe(long)
  })
})

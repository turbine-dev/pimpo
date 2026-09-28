import { useEffect, useReducer, useRef, useState } from 'react'
import { localeTag } from './i18n'

type Recognition = {
  lang: string
  interimResults: boolean
  continuous: boolean
  start: () => void
  stop: () => void
  onresult: ((e: { results: ArrayLike<ArrayLike<{ transcript: string }> & { isFinal: boolean }> }) => void) | null
  onend: (() => void) | null
  onerror: ((e: { error: string }) => void) | null
}

function recognizer(): (new () => Recognition) | undefined {
  const w = window as unknown as { SpeechRecognition?: new () => Recognition; webkitSpeechRecognition?: new () => Recognition }
  return w.SpeechRecognition ?? w.webkitSpeechRecognition
}

const canRecord = () => typeof MediaRecorder !== 'undefined' && !!navigator.mediaDevices?.getUserMedia

// useDictation turns speech into text: with the browser's own recognition
// when there is one, else by recording and asking Pimpo to transcribe it
// on this machine.
export function useDictation(onText: (text: string, final: boolean) => void) {
  const [listening, setListening] = useState(false)
  const [error, setError] = useState('')
  const stopRef = useRef<() => void>(() => {})
  const supported = !!recognizer() || canRecord()
  useEffect(() => () => stopRef.current(), [])

  const start = async () => {
    setError('')
    const R = recognizer()
    if (R) {
      const r = new R()
      r.lang = localeTag()
      r.interimResults = true
      r.continuous = false
      r.onresult = (e) => {
        const res = Array.from(e.results)
        onText(res.map((x) => x[0].transcript).join(''), res.every((x) => x.isFinal))
      }
      r.onerror = (e) => { if (e.error !== 'no-speech' && e.error !== 'aborted') setError(e.error) }
      r.onend = () => setListening(false)
      stopRef.current = () => r.stop()
      r.start()
      setListening(true)
      return
    }
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
      const rec = new MediaRecorder(stream)
      const parts: Blob[] = []
      rec.ondataavailable = (e) => parts.push(e.data)
      rec.onstop = async () => {
        stream.getTracks().forEach((t) => t.stop())
        setListening(false)
        const res = await fetch('/api/voice/transcribe', { method: 'POST', body: new Blob(parts, { type: rec.mimeType }), credentials: 'same-origin' })
        const body = await res.json().catch(() => ({}))
        if (res.ok && body.text) onText(body.text, true)
        else setError(body.error ?? res.statusText)
      }
      stopRef.current = () => rec.state === 'recording' && rec.stop()
      rec.start()
      setListening(true)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }
  return { supported, listening, error, start, stop: () => stopRef.current() }
}

export const canSpeak = () => typeof window !== 'undefined' && 'speechSynthesis' in window

// speak reads text aloud with the system's voice.
export function speak(text: string) {
  if (!canSpeak()) return
  window.speechSynthesis.cancel()
  const u = new SpeechSynthesisUtterance(text)
  u.lang = localeTag()
  window.speechSynthesis.speak(u)
}

// Reading aloud with Pimpo's voices, as it goes: the text is read a few
// sentences at a time, the first as soon as it is ready while the next are
// made, so listening starts in about a second. Pimpo keeps what it read,
// so hearing it again starts at once.

export type Reading = { key: string; phase: 'loading' | 'playing' } | null

let reading: Reading = null
let run = 0
let playing: HTMLAudioElement | null = null
let stopPlaying: (() => void) | null = null
const listeners = new Set<() => void>()

function setReading(r: Reading) {
  reading = r
  listeners.forEach((f) => f())
}

// useReading is what is being read now, for the buttons.
export function useReading(): Reading {
  const [, bump] = useReducer((x: number) => x + 1, 0)
  useEffect(() => {
    listeners.add(bump)
    return () => { listeners.delete(bump) }
  }, [])
  return reading
}

// sentences splits text into pieces to read: the first short, so it starts
// soon; the rest a few sentences each.
export function sentences(text: string, first = 140, rest = 320): string[] {
  const parts = text.replace(/\s+/g, ' ').trim().split(/(?<=[.!?…:;])\s+/)
  const out: string[] = []
  let cur = ''
  for (const p of parts) {
    const limit = out.length === 0 ? first : rest
    if (cur && (cur + ' ' + p).length > limit) {
      out.push(cur)
      cur = p
    } else {
      cur = cur ? cur + ' ' + p : p
    }
  }
  if (cur) out.push(cur)
  return out.flatMap((c) => (c.length > rest * 2 ? c.match(new RegExp(`.{1,${rest}}(\\s|$)`, 'g')) ?? [c] : [c])).map((c) => c.trim()).filter(Boolean)
}

function speakPiece(text: string): Promise<Blob> {
  return fetch('/api/speak', { method: 'POST', headers: { 'Content-Type': 'application/json' }, credentials: 'same-origin', body: JSON.stringify({ text, language: localeTag() }) })
    .then((r) => (r.ok ? r.blob() : Promise.reject(new Error(r.statusText))))
}

function play(blob: Blob): Promise<void> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(blob)
    const a = new Audio(url)
    playing = a
    const done = () => { URL.revokeObjectURL(url); stopPlaying = null; resolve() }
    stopPlaying = () => { a.pause(); done() }
    a.onended = done
    a.onerror = () => { URL.revokeObjectURL(url); reject(new Error('audio')) }
    a.play().catch(reject)
  })
}

// readAloud reads an answer with the chat's voice chosen in Settings: the
// browser's own, or one Pimpo makes (downloaded, system or cloud). When
// Pimpo cannot read it, the browser's voice does. key names what is read,
// for the buttons.
export async function readAloud(text: string, chatVoice?: string, key = text) {
  stopReading()
  if (!text) return
  if (chatVoice === 'browser') return speak(text)
  const mine = ++run
  setReading({ key, phase: 'loading' })
  const parts = sentences(text)
  const made: (Promise<Blob> | undefined)[] = []
  const get = (i: number) => (made[i] ??= speakPiece(parts[i]))
  let i = 0
  try {
    for (; i < parts.length; i++) {
      const blob = await get(i)
      if (mine !== run) return
      if (i + 1 < parts.length) get(i + 1) // the next is made while this one plays
      setReading({ key, phase: 'playing' })
      await play(blob)
      if (mine !== run) return
    }
  } catch {
    if (mine === run) speak(parts.slice(i).join(' '))
  } finally {
    if (mine === run) setReading(null)
  }
}

export function stopReading() {
  run++
  stopPlaying?.()
  playing?.pause()
  playing = null
  if (typeof window !== 'undefined') window.speechSynthesis?.cancel()
  if (reading) setReading(null)
}

const warmed = new Set<string>()

// warmReading makes the first piece of an answer ahead of time, so Listen
// starts at once; only free voices are warmed, never a paid one.
export function warmReading(text: string, engine: string) {
  if (!text || !['auto', 'local', 'system'].includes(engine)) return
  const first = sentences(text)[0]
  if (!first || warmed.has(first)) return
  warmed.add(first)
  speakPiece(first).catch(() => warmed.delete(first))
}

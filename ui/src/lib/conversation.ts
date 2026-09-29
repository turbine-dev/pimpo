import { useEffect, useRef, useState } from 'react'
import { stopReading } from './voice'

// A spoken conversation with Pimpo: it waits for its name (the wake word),
// listens until you stop talking, answers aloud and listens again; talking
// while it speaks interrupts it (barge-in). Everything heard is written on
// this Pimpo with Whisper, never by the browser's cloud recognition, and
// only while the conversation is on. Approvals are never given by voice:
// what Pimpo would do waits for a tap on the screen.

export type ConversationState = 'off' | 'standby' | 'listening' | 'thinking' | 'speaking'

const strip = (s: string) => s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()

// Whisper writes the name in a few ways.
const wake = /^\s*(?:(?:ei|oi|ok|okay|hey|hi|ola|hola|e|ai)[\s,!.]+)?(?:pimpo|pimpao|pimbo|pinpo|pimpu|pim po)\b[\s,!.?:;-]*/

// afterWake is what follows the wake word, or null when it was not said.
export function afterWake(text: string): string | null {
  const s = strip(text)
  const m = s.match(wake)
  if (!m) return null
  return text.slice(m[0].length).trim()
}

// Voice activity: speech when the level stays well above the room's
// noise for a moment, the end after a pause.
export type Levels = { floor: number; speaking: boolean; loud: number; quiet: number }

export function step(l: Levels, rms: number, strict: boolean): { l: Levels; started: boolean; ended: boolean } {
  // The floor follows quiet quickly and noise slowly, so speech does not
  // raise it before it is heard.
  const floor = l.speaking ? l.floor : Math.min(0.2, rms < l.floor ? l.floor * 0.8 + rms * 0.2 : l.floor * 0.995 + rms * 0.005)
  const threshold = Math.max(0.015, floor * 3) * (strict ? 2 : 1)
  const above = rms > threshold
  const loud = above ? l.loud + 1 : 0
  const quiet = above ? 0 : l.quiet + 1
  if (!l.speaking && loud >= 3) return { l: { floor, speaking: true, loud, quiet }, started: true, ended: false }
  if (l.speaking && quiet >= 18) return { l: { floor, speaking: false, loud, quiet }, started: false, ended: true }
  return { l: { floor, speaking: l.speaking, loud, quiet }, started: false, ended: false }
}

const frame = 50 // ms: 3 loud frames start speech, 18 quiet ones (0.9 s) end it
const followUp = 12_000

async function transcribe(audio: Blob): Promise<string> {
  const res = await fetch('/api/voice/transcribe', { method: 'POST', body: audio, credentials: 'same-origin' })
  const body = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(body.error ?? res.statusText)
  return (body.text ?? '').trim()
}

export function useConversation(onRequest: (text: string) => void) {
  const [state, setState] = useState<ConversationState>('off')
  const [useWake, setUseWake] = useState(true)
  const [error, setError] = useState('')
  const [heard, setHeard] = useState('')
  const st = useRef<ConversationState>('off')
  const wakeRef = useRef(useWake)
  const request = useRef(onRequest)
  const stopAll = useRef<() => void>(() => {})
  const idle = useRef<ReturnType<typeof setTimeout>>(undefined)
  useEffect(() => { request.current = onRequest }, [onRequest])
  useEffect(() => { wakeRef.current = useWake }, [useWake])
  useEffect(() => () => stopAll.current(), [])

  const go = (s: ConversationState) => {
    st.current = s
    setState(s)
    clearTimeout(idle.current)
    // After an answer, a follow-up needs no wake word for a while.
    if (s === 'listening' && wakeRef.current) idle.current = setTimeout(() => st.current === 'listening' && go('standby'), followUp)
  }

  const utterance = async (blob: Blob, ms: number) => {
    if (ms < 400) return
    const before = st.current
    if (before !== 'standby' && before !== 'listening') return
    let text = ''
    try {
      text = await transcribe(blob)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      return
    }
    if (!text || st.current !== before) return
    if (before === 'standby') {
      const rest = afterWake(text)
      if (rest === null) return
      if (!rest) return go('listening')
      text = rest
    }
    setHeard(text)
    go('thinking')
    request.current(text)
  }

  const start = async () => {
    setError('')
    let stream: MediaStream
    try {
      stream = await navigator.mediaDevices.getUserMedia({ audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true } })
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      return
    }
    const ctx = new AudioContext()
    const analyser = ctx.createAnalyser()
    analyser.fftSize = 1024
    ctx.createMediaStreamSource(stream).connect(analyser)
    const buf = new Float32Array(analyser.fftSize)
    let levels: Levels = { floor: 0.01, speaking: false, loud: 0, quiet: 0 }
    let rec: MediaRecorder | null = null
    let began = 0
    const timer = setInterval(() => {
      analyser.getFloatTimeDomainData(buf)
      let sum = 0
      for (const v of buf) sum += v * v
      const r = step(levels, Math.sqrt(sum / buf.length), st.current === 'speaking')
      levels = r.l
      if (r.started && st.current !== 'thinking') {
        if (st.current === 'speaking') {
          stopReading() // barge-in
          go('listening')
        }
        const parts: Blob[] = []
        rec = new MediaRecorder(stream)
        began = Date.now()
        rec.ondataavailable = (e) => parts.push(e.data)
        const mine = rec
        rec.onstop = () => utterance(new Blob(parts, { type: mine.mimeType }), Date.now() - began)
        rec.start()
      }
      if ((r.ended || Date.now() - began > 30_000) && rec?.state === 'recording') {
        rec.stop()
        rec = null
      }
    }, frame)
    stopAll.current = () => {
      clearInterval(timer)
      clearTimeout(idle.current)
      if (rec?.state === 'recording') { rec.onstop = null; rec.stop() }
      stream.getTracks().forEach((t) => t.stop())
      ctx.close()
      stopReading()
      st.current = 'off'
      setState('off')
    }
    go(wakeRef.current ? 'standby' : 'listening')
  }

  // replied reads the answer and listens again. speak resolves when the
  // reading ends or is interrupted.
  const replied = async (speak: () => Promise<void>) => {
    if (st.current === 'off') return
    go('speaking')
    await speak()
    if (st.current === 'speaking') go('listening')
  }

  const supported = typeof MediaRecorder !== 'undefined' && !!navigator.mediaDevices?.getUserMedia && typeof AudioContext !== 'undefined'
  return { state, supported, error, heard, useWake, setUseWake, start, stop: () => stopAll.current(), replied }
}

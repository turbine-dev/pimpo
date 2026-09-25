import { useEffect, useRef, useState } from 'react'
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
// when there is one, else by recording and asking Zodim to transcribe it
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

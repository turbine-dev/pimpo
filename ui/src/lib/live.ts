import { type QueryClient, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import type { Progress, VEvent } from './api'

// Every event from the server refreshes the data it may have changed.
// The socket reconnects on its own after sleep or a restart.
export function useLiveEvents(onEvent?: (e: VEvent) => void) {
  const qc = useQueryClient()
  const [connected, setConnected] = useState(false)
  useEffect(() => {
    let ws: WebSocket | undefined
    let retry: ReturnType<typeof setTimeout>
    let closed = false
    const connect = () => {
      const proto = location.protocol === 'https:' ? 'wss' : 'ws'
      ws = new WebSocket(`${proto}://${location.host}/api/ws`)
      ws.onopen = () => setConnected(true)
      ws.onclose = () => {
        setConnected(false)
        if (!closed) retry = setTimeout(connect, 2000)
      }
      ws.onmessage = (m) => {
        const e = JSON.parse(m.data) as VEvent
        onEvent?.(e)
        // The mascot and other listeners hear every event without a second socket.
        window.dispatchEvent(new CustomEvent('pimpo:event', { detail: e }))
        if (e.type === 'progress.updated') {
          onProgress(qc, e)
          return
        }
        qc.invalidateQueries({ queryKey: ['state'] })
        if (e.type.startsWith('routine') || e.type.startsWith('exploration') || e.type === 'action.done') {
          qc.invalidateQueries({ queryKey: ['routines'] })
          qc.invalidateQueries({ queryKey: ['explorations'] })
          qc.invalidateQueries({ queryKey: ['exploration'] })
          qc.invalidateQueries({ queryKey: ['routine'] })
        }
        if (e.type.startsWith('connection') || e.type === 'telegram.paired') qc.invalidateQueries({ queryKey: ['connections'] })
        if (e.type.startsWith('approval')) qc.invalidateQueries({ queryKey: ['approvals'] })
        if (e.type === 'action.done' || e.type === 'action.undone') qc.invalidateQueries({ queryKey: ['receipts'] })
        if (e.type === 'memory.changed') qc.invalidateQueries({ queryKey: ['memory'] })
        if (e.type === 'rules.changed') qc.invalidateQueries({ queryKey: ['rules'] })
        if (e.type === 'cost.recorded') qc.invalidateQueries({ queryKey: ['cost'] })
        if (e.type === 'widget.updated' || e.type.startsWith('routine.run') || e.type.startsWith('approval') || e.type === 'cost.recorded') {
          qc.invalidateQueries({ queryKey: ['dashboard-widgets'] })
          qc.invalidateQueries({ queryKey: ['widgets'] })
        }
        // What needs the person: approvals, questions, runs, jobs, and
        // explorations or suggestions becoming ready.
        if (/^(approval|question|routine|job|exploration|suggestion)/.test(e.type)) qc.invalidateQueries({ queryKey: ['needs'] })
        if (e.type === 'dashboard.changed') qc.invalidateQueries({ queryKey: ['dashboards'] })
        if (e.type.startsWith('company.')) {
          qc.invalidateQueries({ queryKey: ['companies'] })
          qc.invalidateQueries({ queryKey: ['company'] })
          qc.invalidateQueries({ queryKey: ['company-work'] })
          qc.invalidateQueries({ queryKey: ['company-tasks'] })
          qc.invalidateQueries({ queryKey: ['company-questions'] })
          qc.invalidateQueries({ queryKey: ['company-notes'] })
          qc.invalidateQueries({ queryKey: ['company-meetings'] })
          qc.invalidateQueries({ queryKey: ['company-digest'] })
        }
        qc.invalidateQueries({ queryKey: ['events'] })
      }
    }
    connect()
    return () => {
      closed = true
      clearTimeout(retry)
      ws?.close()
    }
  }, [qc, onEvent])
  return connected
}

// A progress update replaces its record in place; someone else's comes
// without data and changes nothing here.
function onProgress(qc: QueryClient, e: VEvent) {
  const p = e.data as unknown as Progress
  if (!p?.id) return
  qc.setQueryData<Progress[]>(['progress'], (old) => (old ? [p, ...old.filter((x) => x.id !== p.id)] : old))
  if (p.state !== 'running') qc.invalidateQueries({ queryKey: ['progress'] })
  if (p.kind === 'job') {
    qc.invalidateQueries({ queryKey: ['jobs'] })
    if (p.job) qc.invalidateQueries({ queryKey: ['job', p.job] })
  }
  if (p.kind === 'run' && p.state !== 'running') {
    qc.invalidateQueries({ queryKey: ['routine'] })
    qc.invalidateQueries({ queryKey: ['runs'] })
  }
}

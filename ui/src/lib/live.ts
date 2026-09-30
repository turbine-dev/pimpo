import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import type { VEvent } from './api'

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

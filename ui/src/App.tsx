import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { Route, Routes } from 'react-router-dom'
import { NewTask } from './components/NewTask'
import { Shell, nav } from './components/Shell'
import { api } from './lib/api'
import { useLiveEvents } from './lib/live'
import { Connections } from './pages/Connections'
import { Design } from './pages/Design'
import { ExplorationPage } from './pages/ExplorationPage'
import { Inbox } from './pages/Inbox'
import { RoutinePage } from './pages/RoutinePage'
import { Routines } from './pages/Routines'
import { Settings } from './pages/Settings'

const ready = new Set(['/', '/inbox', '/connections', '/settings'])

export default function App() {
  const [newTask, setNewTask] = useState(false)
  useLiveEvents()
  const state = useQuery({ queryKey: ['state'], queryFn: api.state, refetchInterval: 30_000 })
  const attention = (state.data?.awaiting ?? 0) + (state.data?.broken ?? 0)
  const items = nav.filter((n) => ready.has(n.to)).map((n) => (n.to === '/inbox' ? { ...n, badge: attention } : n))
  return (
    <Shell items={items} budget={state.data?.budget} healthy={state.data?.healthy ?? true}>
      <Routes>
        <Route path="/" element={<Routines onNew={() => setNewTask(true)} />} />
        <Route path="/routines/:id" element={<RoutinePage />} />
        <Route path="/explorations/:id" element={<ExplorationPage />} />
        <Route path="/inbox" element={<Inbox />} />
        <Route path="/connections" element={<Connections />} />
        <Route path="/settings" element={<Settings />} />
        <Route path="/design" element={<Design />} />
      </Routes>
      <NewTask open={newTask} onOpenChange={setNewTask} />
    </Shell>
  )
}

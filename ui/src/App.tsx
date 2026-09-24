import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { ErrorBoundary } from './components/ErrorBoundary'
import { NewTask } from './components/NewTask'
import { Palette } from './components/Palette'
import { Welcome } from './pages/Welcome'
import { Shell, nav } from './components/Shell'
import { api } from './lib/api'
import { useLiveEvents } from './lib/live'
import { Connections } from './pages/Connections'
import { Cost } from './pages/Cost'
import { Receipts } from './pages/Receipts'
import { Rules } from './pages/Rules'
import { Design } from './pages/Design'
import { ExplorationPage } from './pages/ExplorationPage'
import { Inbox } from './pages/Inbox'
import { RoutinePage } from './pages/RoutinePage'
import { Routines } from './pages/Routines'
import { Settings } from './pages/Settings'

const ready = new Set(['/', '/inbox', '/receipts', '/rules', '/cost', '/connections', '/settings'])

export default function App() {
  const [newTask, setNewTask] = useState(false)
  const [palette, setPalette] = useState(false)
  const location = useLocation()
  const setup = useQuery({ queryKey: ['setup'], queryFn: api.setup })
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setPalette((p) => !p)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
  useLiveEvents()
  const state = useQuery({ queryKey: ['state'], queryFn: api.state, refetchInterval: 30_000 })
  const attention = (state.data?.awaiting ?? 0) + (state.data?.broken ?? 0)
  const items = nav.filter((n) => ready.has(n.to)).map((n) => (n.to === '/inbox' ? { ...n, badge: attention } : n))
  if (location.pathname === '/welcome') {
    return (
      <div className="min-h-full px-4">
        <Welcome onFirstTask={() => setNewTask(true)} />
        <NewTask open={newTask} onOpenChange={setNewTask} />
      </div>
    )
  }
  return (
    <Shell items={items} budget={state.data?.budget} healthy={state.data?.healthy ?? true} onSearch={() => setPalette(true)}>
      <ErrorBoundary resetKey={location.pathname}>
      <Routes>
        <Route path="/" element={setup.data && !setup.data.done ? <Navigate to="/welcome" replace /> : <Routines onNew={() => setNewTask(true)} />} />
        <Route path="/routines/:id" element={<RoutinePage />} />
        <Route path="/explorations/:id" element={<ExplorationPage />} />
        <Route path="/inbox" element={<Inbox />} />
        <Route path="/receipts" element={<Receipts />} />
        <Route path="/rules" element={<Rules />} />
        <Route path="/cost" element={<Cost />} />
        <Route path="/connections" element={<Connections />} />
        <Route path="/settings" element={<Settings />} />
        <Route path="/design" element={<Design />} />
      </Routes>
      </ErrorBoundary>
      <NewTask open={newTask} onOpenChange={setNewTask} />
      <Palette open={palette} onOpenChange={setPalette} onNewTask={() => setNewTask(true)} />
    </Shell>
  )
}

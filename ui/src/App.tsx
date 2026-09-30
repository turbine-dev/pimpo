import { Skills } from './pages/Skills'
import { Phone } from './pages/Phone'
import { Jobs } from './pages/Jobs'
import { Account } from './pages/Account'
import { Dashboards } from './pages/Dashboards'
import { SignIn } from './pages/SignIn'
import { CreateAdmin } from './pages/CreateAdmin'
import { Recovery } from './pages/Recovery'
import { Credential } from './pages/Credential'

import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { ErrorBoundary } from './components/ErrorBoundary'
import { Mascot } from './components/Mascot'
import { NewTask } from './components/NewTask'
import { Palette } from './components/Palette'
import { Welcome } from './pages/Welcome'
import { Shell } from './components/Shell'
import { Home } from './pages/Home'
import { Assistants } from './pages/Assistants'
import { Chat } from './pages/Chat'
import { Help } from './pages/Help'
import { api, ApiError } from './lib/api'
import { useLiveEvents } from './lib/live'
import { Connections } from './pages/Connections'
import { Cost } from './pages/Cost'
import { Receipts } from './pages/Receipts'
import { Rules } from './pages/Rules'
import { Design } from './pages/Design'
import { ExplorationPage } from './pages/ExplorationPage'
import { Inbox } from './pages/Inbox'
import { Memory } from './pages/Memory'
import { Lessons } from './pages/Lessons'
import { RoutinePage } from './pages/RoutinePage'
import { Routines } from './pages/Routines'
import { Import } from './pages/Import'
import { People } from './pages/People'
import { Gallery } from './pages/Gallery'
import { Settings } from './pages/Settings'

export default function App() {
  const [newTask, setNewTask] = useState(false)
  const [palette, setPalette] = useState(false)
  const location = useLocation()
  const nav = useNavigate()
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
  const [creatingAdmin, setCreatingAdmin] = useState(false)
  const attention = (state.data?.awaiting ?? 0) + (state.data?.broken ?? 0) + (state.data?.approvals ?? 0)
  if (location.pathname === '/mascot') {
    // The desktop app's floating window: only the cat, on a transparent page.
    // Links go through /open, which the app turns into its main window.
    document.body.style.background = 'transparent'
    document.documentElement.style.background = 'transparent'
    document.body.style.overflow = 'hidden'
    return <Mascot standalone onOpen={(path) => { location.pathname !== path && window.location.assign('/open?path=' + encodeURIComponent(path)) }} />
  }
  if (state.error instanceof ApiError && state.error.recovery) return <Recovery />
  if (state.error instanceof ApiError && state.error.status === 401) return <SignIn />
  // The account screen stays until the person finishes it, passkey step
  // included, even once the server already knows the account.
  if (creatingAdmin || (state.data?.role === 'owner' && state.data.admin_account === false && location.pathname !== '/mascot')) {
    return <CreateAdmin onStart={() => setCreatingAdmin(true)} onDone={() => setCreatingAdmin(false)} />
  }
  if (location.pathname === '/welcome') {
    return (
      <div className="min-h-full px-4">
        <Welcome onFirstTask={() => setNewTask(true)} />
        <NewTask open={newTask} onOpenChange={setNewTask} />
      </div>
    )
  }
  return (
    <Shell attention={attention} budget={state.data?.budget} healthy={state.data?.healthy ?? true} onSearch={() => setPalette(true)}>
      <ErrorBoundary resetKey={location.pathname}>
      <Routes>
        <Route path="/" element={setup.data && !setup.data.done ? <Navigate to="/welcome" replace /> : <Home />} />
        <Route path="/routines" element={<Routines onNew={() => setNewTask(true)} />} />
        <Route path="/routines/:id" element={<RoutinePage />} />
        <Route path="/explorations/:id" element={<ExplorationPage />} />
        <Route path="/chat" element={<Chat />} />
        <Route path="/chat/:id" element={<Chat />} />
        <Route path="/assistants" element={<Assistants />} />
        <Route path="/skills" element={<Skills />} />
        <Route path="/phone" element={<Phone />} />
        <Route path="/jobs" element={<Jobs />} />
        <Route path="/account" element={<Account />} />
        <Route path="/dashboards" element={<Dashboards />} />
        <Route path="/dashboards/:id" element={<Dashboards />} />
        <Route path="/jobs/:id" element={<Jobs />} />
        <Route path="/help" element={<Help />} />
        <Route path="/inbox" element={<Inbox />} />
        <Route path="/credentials/:id" element={<Credential />} />
        <Route path="/receipts" element={<Receipts />} />
        <Route path="/rules" element={<Rules />} />
        <Route path="/cost" element={<Cost />} />
        <Route path="/memory" element={<Memory />} />
        <Route path="/lessons" element={<Lessons />} />
        <Route path="/connections" element={<Connections />} />
        <Route path="/settings" element={<Settings />} />
        <Route path="/import" element={<Import />} />
        <Route path="/people" element={<People />} />
        <Route path="/gallery" element={<Gallery />} />
        <Route path="/design" element={<Design />} />
      </Routes>
      </ErrorBoundary>
      <Mascot onOpen={(path) => nav(path)} />
      <NewTask open={newTask} onOpenChange={setNewTask} />
      <Palette open={palette} onOpenChange={setPalette} onNewTask={() => setNewTask(true)} />
    </Shell>
  )
}

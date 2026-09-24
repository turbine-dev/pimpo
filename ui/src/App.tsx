import { Route, Routes } from 'react-router-dom'
import { Shell } from './components/Shell'
import { Design } from './pages/Design'
import { Routines } from './pages/Routines'

export default function App() {
  return (
    <Shell budget={{ spent: 0, limit: 1 }}>
      <Routes>
        <Route path="/" element={<Routines routines={[]} />} />
        <Route path="/design" element={<Design />} />
      </Routes>
    </Shell>
  )
}

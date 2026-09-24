import { AlertTriangle, RotateCcw } from 'lucide-react'
import { Component, type ReactNode } from 'react'
import { Button } from './ui'

// A render error shows a way out instead of a blank screen.
export class ErrorBoundary extends Component<{ children: ReactNode; resetKey?: string }, { error?: Error }> {
  state: { error?: Error } = {}

  static getDerivedStateFromError(error: Error) {
    return { error }
  }

  componentDidUpdate(prev: { resetKey?: string }) {
    if (prev.resetKey !== this.props.resetKey && this.state.error) this.setState({ error: undefined })
  }

  render() {
    if (!this.state.error) return this.props.children
    return (
      <div className="mx-auto mt-16 max-w-md text-center">
        <div className="mx-auto mb-4 grid size-12 place-items-center rounded-2xl bg-danger-soft text-danger">
          <AlertTriangle size={22} />
        </div>
        <h2 className="text-[16px] font-semibold">Algo deu errado nesta tela</h2>
        <p className="mt-1.5 text-sm text-ink-2">Seus dados estão seguros. Recarregar costuma resolver.</p>
        <pre className="mt-4 overflow-x-auto rounded-lg bg-sunken p-3 text-left font-mono text-[11.5px] text-ink-3">{this.state.error.message}</pre>
        <Button className="mt-5" variant="primary" onClick={() => location.reload()}>
          <RotateCcw size={15} /> Recarregar
        </Button>
      </div>
    )
  }
}

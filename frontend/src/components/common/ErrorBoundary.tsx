import { Component, type ErrorInfo, type ReactNode } from 'react'

import { Button } from '@/components/ui/button'

interface Props {
  children: ReactNode
  fallback?: (error: Error, reset: () => void) => ReactNode
}

interface State {
  error: Error | null
}

// Last-resort UI safety net. Catches render-time errors so a single bad
// component can't take down the whole app. Async errors (fetches, mutations)
// don't go through this — those are handled by TanStack Query.
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(_error: Error, _info: ErrorInfo) {
    // Intentionally empty — error is captured via getDerivedStateFromError.
  }

  reset = () => this.setState({ error: null })

  render() {
    if (this.state.error) {
      if (this.props.fallback) return this.props.fallback(this.state.error, this.reset)
      return (
        <div className="flex min-h-[40vh] flex-col items-center justify-center gap-4 p-8">
          <h2 className="text-lg font-semibold text-destructive">문제가 발생했습니다</h2>
          <p className="max-w-md text-center text-sm text-muted-foreground">
            {this.state.error.message}
          </p>
          <Button onClick={this.reset} variant="outline">
            다시 시도
          </Button>
        </div>
      )
    }
    return this.props.children
  }
}

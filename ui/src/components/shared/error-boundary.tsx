import { Component } from "react"
import type { ReactNode, ErrorInfo } from "react"
import { Button } from "@/components/ui/button"
import { AlertTriangle } from "lucide-react"

interface Props {
  children: ReactNode
}

interface State {
  error: Error | null
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error) {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("ErrorBoundary caught:", error, info)
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <AlertTriangle className="h-10 w-10 text-destructive mb-4" />
          <h2 className="text-[22px] font-light tracking-[-0.22px] text-foreground mb-2">
            Something went wrong
          </h2>
          <p className="text-sm font-light text-muted-foreground mb-4 max-w-md">
            {this.state.error.message}
          </p>
          <Button
            onClick={() => this.setState({ error: null })}
            variant="outline"
            className="border-border rounded-sm font-normal"
          >
            Try Again
          </Button>
        </div>
      )
    }
    return this.props.children
  }
}

import { Badge } from "@/components/ui/badge"
import { Card, CardContent } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Loader2, CheckCircle2, XCircle, Circle } from "lucide-react"

export type IntegrationState =
  | "not_configured"
  | "configuring"
  | "testing"
  | "connected"
  | "error"

const stateLabels: Record<IntegrationState, string> = {
  not_configured: "Not Configured",
  configuring: "Configuring",
  testing: "Testing...",
  connected: "Connected",
  error: "Error",
}

const stateColors: Record<IntegrationState, string> = {
  not_configured: "bg-muted text-muted-foreground border-border",
  configuring: "bg-blue-500/10 text-blue-500 border-blue-500/30",
  testing: "bg-yellow-500/10 text-yellow-600 border-yellow-500/30",
  connected: "bg-[rgba(21,190,83,0.15)] text-[var(--stripe-success-text,#15be53)] border-[rgba(21,190,83,0.4)]",
  error: "bg-destructive/10 text-destructive border-destructive/30",
}

function StateIcon({ state }: { state: IntegrationState }) {
  switch (state) {
    case "testing":
      return <Loader2 className="h-4 w-4 animate-spin text-yellow-600" />
    case "connected":
      return <CheckCircle2 className="h-4 w-4 text-[var(--stripe-success-text,#15be53)]" />
    case "error":
      return <XCircle className="h-4 w-4 text-destructive" />
    default:
      return <Circle className="h-4 w-4 text-muted-foreground" />
  }
}

interface IntegrationCardProps {
  title: string
  description: string
  state: IntegrationState
  error?: string
  testPassed: boolean
  onTest?: () => void
  onSave?: () => void
  isTesting?: boolean
  isSaving?: boolean
  disabled?: boolean
  children?: React.ReactNode
}

export function IntegrationCard({
  title,
  description,
  state,
  error,
  testPassed,
  onTest,
  onSave,
  isTesting,
  isSaving,
  disabled,
  children,
}: IntegrationCardProps) {
  return (
    <Card className="rounded-md border-border shadow-stripe">
      <CardContent className="pt-5 pb-5 space-y-4">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <StateIcon state={state} />
            <h3 className="text-[15px] font-semibold text-foreground">{title}</h3>
          </div>
          <Badge className={`text-[10px] font-medium rounded-sm px-2 py-0.5 ${stateColors[state]}`}>
            {stateLabels[state]}
          </Badge>
        </div>

        <p className="text-xs text-muted-foreground">{description}</p>

        {children}

        {error && (
          <p className="text-xs text-destructive bg-destructive/5 border border-destructive/20 rounded-sm px-3 py-2">
            {error}
          </p>
        )}

        {onTest && onSave && (
          <div className="flex items-center gap-2 pt-1">
            <Button
              size="sm"
              variant="outline"
              onClick={onTest}
              disabled={disabled || isTesting || isSaving}
              className="border-border rounded-sm font-normal text-xs"
            >
              {isTesting ? (
                <>
                  <Loader2 className="h-3.5 w-3.5 animate-spin mr-1.5" />
                  Testing...
                </>
              ) : (
                "Test"
              )}
            </Button>
            <Button
              size="sm"
              onClick={onSave}
              disabled={!testPassed || isSaving || isTesting}
              className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal text-xs"
            >
              {isSaving ? (
                <>
                  <Loader2 className="h-3.5 w-3.5 animate-spin mr-1.5" />
                  Saving...
                </>
              ) : (
                "Save"
              )}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

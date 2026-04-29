import { useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Button } from "@/components/ui/button"
import { IntegrationCard, type IntegrationState } from "./integration-card"
import { api, getErrorMessage } from "@/lib/api"
import { Eye, EyeOff } from "lucide-react"

export function LinearCard() {
  const queryClient = useQueryClient()
  const { data: status } = useQuery({
    queryKey: ["linear-status"],
    queryFn: api.getLinearStatus,
  })

  const isConfigured = status?.configured ?? false

  const [apiKey, setApiKey] = useState("")
  const [teamKey, setTeamKey] = useState("")
  const [webhookSecret, setWebhookSecret] = useState("")
  const [showKey, setShowKey] = useState(false)
  const [showSecret, setShowSecret] = useState(false)
  const [testPassed, setTestPassed] = useState(false)
  const [testError, setTestError] = useState<string>()
  const [testWarning, setTestWarning] = useState<string>()

  const testMutation = useMutation({
    mutationFn: () => api.testLinear({ api_key: apiKey, team_key: teamKey || undefined }),
    onSuccess: (res) => {
      if (res.ok) {
        setTestPassed(true)
        setTestError(undefined)
        setTestWarning(res.team_warning)
        const orgLabel = res.organization ? ` (${res.organization})` : ""
        toast.success(`Linear API key verified${orgLabel}`)
      } else {
        setTestPassed(false)
        setTestError(res.error ?? "Test failed")
        setTestWarning(undefined)
      }
    },
    onError: (err) => {
      setTestPassed(false)
      setTestError(getErrorMessage(err))
      setTestWarning(undefined)
    },
  })

  const saveMutation = useMutation({
    mutationFn: () => api.saveLinear({
      api_key: apiKey,
      team_key: teamKey || undefined,
      webhook_secret: webhookSecret || undefined,
    }),
    onSuccess: () => {
      toast.success("Linear credentials saved")
      setApiKey("")
      setTeamKey("")
      setWebhookSecret("")
      setTestPassed(false)
      setTestWarning(undefined)
      queryClient.invalidateQueries({ queryKey: ["linear-status"] })
      queryClient.invalidateQueries({ queryKey: ["status"] })
      queryClient.invalidateQueries({ queryKey: ["onboarding-status"] })
    },
    onError: (err) => toast.error(getErrorMessage(err)),
  })

  const state: IntegrationState = saveMutation.isPending
    ? "configuring"
    : testMutation.isPending
      ? "testing"
      : testError
        ? "error"
        : isConfigured
          ? "connected"
          : "not_configured"

  const description = isConfigured && status?.organization
    ? `Push PRDs and Issues to Linear. Connected to ${status.organization.name}${status.team_key ? ` · team ${status.team_key}` : ""}.`
    : "Push PRDs and Issues to Linear. Vault owns content; Linear owns workflow state."

  return (
    <IntegrationCard
      title="Linear"
      description={description}
      state={state}
      error={testError}
      testPassed={testPassed}
      onTest={() => {
        setTestPassed(false)
        setTestError(undefined)
        setTestWarning(undefined)
        testMutation.mutate()
      }}
      onSave={() => saveMutation.mutate()}
      isTesting={testMutation.isPending}
      isSaving={saveMutation.isPending}
      disabled={!apiKey.trim()}
    >
      <div className="text-[11px] text-muted-foreground bg-muted/50 border border-border rounded-sm px-3 py-2 space-y-1">
        <p>1. Settings → Account → Security & Access → New API key</p>
        <p>2. Paste the key + your default team key (e.g. <span className="font-mono">BCN</span>)</p>
        <p>3. Click Test, then Save</p>
      </div>
      <div className="space-y-2">
        <Label className="text-[var(--stripe-label)] text-xs font-normal">API Key</Label>
        <div className="relative">
          <Input
            type={showKey ? "text" : "password"}
            value={apiKey}
            onChange={(e) => {
              setApiKey(e.target.value)
              setTestPassed(false)
              setTestError(undefined)
              setTestWarning(undefined)
            }}
            placeholder={isConfigured ? "Enter new key to reconfigure" : "lin_api_..."}
            className="border-border rounded-sm text-xs font-mono pr-10"
          />
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => setShowKey(!showKey)}
            className="absolute right-1 top-1/2 -translate-y-1/2 h-6 w-6 p-0"
          >
            {showKey ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
          </Button>
        </div>
      </div>
      <div className="space-y-2">
        <Label className="text-[var(--stripe-label)] text-xs font-normal">
          Default Team Key
        </Label>
        <Input
          value={teamKey}
          onChange={(e) => {
            setTeamKey(e.target.value)
            setTestPassed(false)
            setTestError(undefined)
            setTestWarning(undefined)
          }}
          placeholder={isConfigured && status?.team_key ? `Current: ${status.team_key}` : "BCN"}
          className="border-border rounded-sm text-xs font-mono"
        />
      </div>
      <div className="space-y-2">
        <Label className="text-[var(--stripe-label)] text-xs font-normal">
          Webhook Secret <span className="text-muted-foreground/60">(optional)</span>
        </Label>
        <div className="relative">
          <Input
            type={showSecret ? "text" : "password"}
            value={webhookSecret}
            onChange={(e) => setWebhookSecret(e.target.value)}
            placeholder={
              status?.webhook_secret_configured
                ? "Webhook secret saved — enter new value to rotate"
                : "Set after registering webhook in Linear"
            }
            className="border-border rounded-sm text-xs font-mono pr-10"
          />
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => setShowSecret(!showSecret)}
            className="absolute right-1 top-1/2 -translate-y-1/2 h-6 w-6 p-0"
          >
            {showSecret ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
          </Button>
        </div>
        {status?.webhook_secret_configured && (
          <p className="text-[11px] text-muted-foreground">
            {status.last_webhook_at
              ? `Last webhook received: ${new Date(status.last_webhook_at).toLocaleString()}`
              : "No webhooks received yet."}
          </p>
        )}
      </div>
      {testWarning && (
        <p className="text-xs text-yellow-600 bg-yellow-500/5 border border-yellow-500/20 rounded-sm px-3 py-2">
          {testWarning}
        </p>
      )}
    </IntegrationCard>
  )
}

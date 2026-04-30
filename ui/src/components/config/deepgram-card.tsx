import { useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Button } from "@/components/ui/button"
import { IntegrationCard, type IntegrationState } from "./integration-card"
import { api, getErrorMessage } from "@/lib/api"
import { Eye, EyeOff } from "lucide-react"

export function DeepgramCard() {
  const queryClient = useQueryClient()
  const { data: status } = useQuery({
    queryKey: ["status"],
    queryFn: api.getStatus,
  })

  const isConfigured = status?.integrations?.find((i) => i.id === "deepgram")?.configured ?? false

  const [apiKey, setApiKey] = useState("")
  const [showKey, setShowKey] = useState(false)
  const [testPassed, setTestPassed] = useState(false)
  const [testError, setTestError] = useState<string>()

  const testMutation = useMutation({
    mutationFn: () => api.testDeepgram({ api_key: apiKey }),
    onSuccess: (res) => {
      if (res.ok) {
        // Only enable Save when the user typed a new key — otherwise
        // we just re-verified the saved key and there's nothing to save.
        setTestPassed(apiKey.trim() !== "")
        setTestError(undefined)
        toast.success(
          apiKey.trim() === ""
            ? "Saved Deepgram key still works"
            : "Deepgram API key verified"
        )
      } else {
        setTestPassed(false)
        setTestError(res.error ?? "Test failed")
      }
    },
    onError: (err) => {
      setTestPassed(false)
      setTestError(getErrorMessage(err))
    },
  })

  const saveMutation = useMutation({
    mutationFn: () => api.saveDeepgram({ api_key: apiKey }),
    onSuccess: () => {
      toast.success("Deepgram API key saved")
      setApiKey("")
      setTestPassed(false)
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

  return (
    <IntegrationCard
      title="Deepgram"
      description="Speech-to-text for audio ingest. Required for transcribing meetings, voice notes, and audio files."
      state={state}
      error={testError}
      testPassed={testPassed}
      onTest={() => {
        setTestPassed(false)
        setTestError(undefined)
        testMutation.mutate()
      }}
      onSave={() => saveMutation.mutate()}
      isTesting={testMutation.isPending}
      isSaving={saveMutation.isPending}
      disabled={!apiKey.trim() && !isConfigured}
    >
      <div className="text-[11px] text-muted-foreground bg-muted/50 border border-border rounded-sm px-3 py-2 space-y-1">
        <p>1. Sign up at <span className="font-mono">deepgram.com</span> (free tier available)</p>
        <p>2. Create a project and generate an API key</p>
        <p>3. Paste the key below and click Test</p>
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
            }}
            placeholder={isConfigured ? "Enter new key to reconfigure" : "Paste your Deepgram API key"}
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
    </IntegrationCard>
  )
}

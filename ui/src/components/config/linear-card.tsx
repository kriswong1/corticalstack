import { useEffect, useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Button } from "@/components/ui/button"
import { IntegrationCard, type IntegrationState } from "./integration-card"
import { api, getErrorMessage } from "@/lib/api"
import { Eye, EyeOff, Link2, Unlink, ExternalLink, Copy, Check } from "lucide-react"

// LinearCard surfaces three configuration paths in priority order:
//   1. Connect (OAuth) — recommended; presented as a 3-step wizard
//      that auto-advances. Linear's OAuth-app registration is the only
//      step that has to happen outside this card.
//   2. Personal API key — fallback for headless / CI use, hidden
//      under "Advanced" until expanded.
//   3. Webhook secret — independent of auth mode; needed for inbound
//      sync once a Linear webhook is registered.
export function LinearCard() {
  const queryClient = useQueryClient()
  const { data: status } = useQuery({
    queryKey: ["linear-status"],
    queryFn: api.getLinearStatus,
  })

  const isConfigured = status?.configured ?? false
  const oauthAppConfigured = status?.oauth_app_configured ?? false
  const authMode = status?.auth_mode ?? ""
  const isOAuth = authMode === "oauth"

  // Read the redirect-back flags the OAuth callback drops into the URL
  // so we can toast on connect / surface errors. Cleared after read so
  // a refresh doesn't re-toast.
  useEffect(() => {
    const url = new URL(window.location.href)
    if (url.searchParams.has("linear_connected")) {
      toast.success("Connected to Linear")
      url.searchParams.delete("linear_connected")
      window.history.replaceState({}, "", url.toString())
      queryClient.invalidateQueries({ queryKey: ["linear-status"] })
      queryClient.invalidateQueries({ queryKey: ["onboarding-status"] })
    } else if (url.searchParams.has("linear_error")) {
      toast.error(`Linear connection failed: ${url.searchParams.get("linear_error")}`)
      url.searchParams.delete("linear_error")
      window.history.replaceState({}, "", url.toString())
    }
  }, [queryClient])

  // Wizard step state for the not-configured path. Stays self-managed
  // and resets on disconnect via the status query.
  // 1 = Register OAuth app in Linear
  // 2 = Paste client_id / client_secret
  // 3 = Connect (authorize)
  const initialStep = oauthAppConfigured ? 3 : 1
  const [wizardStep, setWizardStep] = useState<1 | 2 | 3>(initialStep)
  const [registerAck, setRegisterAck] = useState(false)
  useEffect(() => {
    setWizardStep(oauthAppConfigured ? 3 : 1)
  }, [oauthAppConfigured])

  const [clientID, setClientID] = useState("")
  const [clientSecret, setClientSecret] = useState("")
  const [showSecret, setShowSecret] = useState(false)

  // Personal API key (Advanced, fallback path)
  const [apiKey, setApiKey] = useState("")
  const [teamKey, setTeamKey] = useState("")
  const [showKey, setShowKey] = useState(false)
  const [showAdvanced, setShowAdvanced] = useState(false)
  const [testPassed, setTestPassed] = useState(false)
  const [testError, setTestError] = useState<string>()
  const [testWarning, setTestWarning] = useState<string>()

  // Webhook secret (independent of auth mode)
  const [webhookSecret, setWebhookSecret] = useState("")
  const [showWebhook, setShowWebhook] = useState(false)

  const saveOAuthAppMutation = useMutation({
    mutationFn: () => api.saveLinearOAuthApp({ client_id: clientID, client_secret: clientSecret }),
    onSuccess: () => {
      toast.success("OAuth app credentials saved")
      setClientID("")
      setClientSecret("")
      setWizardStep(3)
      queryClient.invalidateQueries({ queryKey: ["linear-status"] })
    },
    onError: (err) => toast.error(getErrorMessage(err)),
  })

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
    mutationFn: () =>
      api.saveLinear({
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

  const saveWebhookOnly = useMutation({
    mutationFn: () => api.saveLinearWebhookSecret({ webhook_secret: webhookSecret }),
    onSuccess: () => {
      toast.success("Webhook secret saved")
      setWebhookSecret("")
      queryClient.invalidateQueries({ queryKey: ["linear-status"] })
    },
    onError: (err) => toast.error(getErrorMessage(err)),
  })

  const disconnectMutation = useMutation({
    mutationFn: () => api.disconnectLinear(),
    onSuccess: () => {
      toast.success("Disconnected from Linear")
      queryClient.invalidateQueries({ queryKey: ["linear-status"] })
      queryClient.invalidateQueries({ queryKey: ["status"] })
      queryClient.invalidateQueries({ queryKey: ["onboarding-status"] })
    },
    onError: (err) => toast.error(getErrorMessage(err)),
  })

  const cardState: IntegrationState = saveMutation.isPending || saveOAuthAppMutation.isPending
    ? "configuring"
    : testMutation.isPending
      ? "testing"
      : testError
        ? "error"
        : isConfigured
          ? "connected"
          : "not_configured"

  const description = isConfigured && status?.organization
    ? `Push PRDs and Issues to Linear. Connected to ${status.organization.name}${status.team_key ? ` · team ${status.team_key}` : ""}${isOAuth ? " · OAuth" : " · API key"}.`
    : "Push PRDs and Issues to Linear. Vault owns content; Linear owns workflow state."

  const copyText = async (text: string, label: string) => {
    try {
      await navigator.clipboard.writeText(text)
      toast.success(`${label} copied`)
    } catch {
      toast.error("Could not copy — paste manually")
    }
  }

  // Smart paste: if either input field receives a paste containing
  // both a UUID-shaped client_id and a longer secret separated by
  // whitespace, auto-fill both. Lets users paste the whole "Client
  // credentials" block from Linear in one shot.
  const handleSmartPaste = (text: string): boolean => {
    const tokens = text
      .split(/\s+/)
      .map((t) => t.trim())
      .filter(Boolean)
    if (tokens.length < 2) return false
    // UUID v4 shape Linear uses for client_id
    const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
    const id = tokens.find((t) => uuid.test(t))
    if (!id) return false
    const secret = tokens.find((t) => t !== id && t.length >= 32)
    if (!secret) return false
    setClientID(id)
    setClientSecret(secret)
    toast.success("Detected both Client ID and Secret")
    return true
  }

  // ── UI ──
  return (
    <IntegrationCard
      title="Linear"
      description={description}
      state={cardState}
      error={testError}
      testPassed={true}
    >
      {/* CONNECTED STATE */}
      {isConfigured && (
        <div className="space-y-3">
          <div className="rounded-sm border border-border bg-muted/40 px-3 py-2 text-xs space-y-1">
            {status?.viewer && (
              <p>
                <span className="text-muted-foreground">User:</span>{" "}
                <span className="font-medium">{status.viewer.name}</span>{" "}
                <span className="text-muted-foreground">({status.viewer.email})</span>
              </p>
            )}
            {status?.organization && (
              <p>
                <span className="text-muted-foreground">Workspace:</span>{" "}
                <span className="font-medium">{status.organization.name}</span>
              </p>
            )}
            {status?.team_key && (
              <p>
                <span className="text-muted-foreground">Default team:</span>{" "}
                <span className="font-mono">{status.team_key}</span>
              </p>
            )}
            <p>
              <span className="text-muted-foreground">Auth mode:</span>{" "}
              <span className="font-mono">{isOAuth ? "OAuth" : "Personal API key"}</span>
            </p>
          </div>

          <WebhookSecretField
            value={webhookSecret}
            onChange={setWebhookSecret}
            show={showWebhook}
            onToggle={() => setShowWebhook(!showWebhook)}
            secretConfigured={status?.webhook_secret_configured}
            lastWebhookAt={status?.last_webhook_at}
          />

          <div className="flex items-center gap-2 pt-1">
            {webhookSecret && (
              <Button
                size="sm"
                onClick={() => saveWebhookOnly.mutate()}
                disabled={saveWebhookOnly.isPending}
                className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal text-xs"
              >
                {saveWebhookOnly.isPending ? "Saving..." : "Save webhook secret"}
              </Button>
            )}
            <Button
              size="sm"
              variant="outline"
              onClick={() => disconnectMutation.mutate()}
              disabled={disconnectMutation.isPending}
              className="border-border rounded-sm font-normal text-xs gap-1.5"
            >
              <Unlink className="h-3.5 w-3.5" />
              {disconnectMutation.isPending ? "Disconnecting..." : "Disconnect"}
            </Button>
          </div>
        </div>
      )}

      {/* NOT CONNECTED — Wizard */}
      {!isConfigured && (
        <div className="space-y-4">
          <StepStrip
            current={wizardStep}
            steps={["Register", "Paste credentials", "Connect"]}
          />

          {/* Step 1: Register OAuth app */}
          {wizardStep === 1 && (
            <div className="space-y-3">
              <div className="rounded-sm border border-border bg-muted/40 px-3 py-2.5 text-xs space-y-2">
                <p className="font-medium text-foreground">
                  Register CorticalStack as an OAuth app in Linear
                </p>
                <p className="text-muted-foreground">
                  This is the only step that happens outside CorticalStack. Takes about 30 seconds.
                </p>
                <ol className="list-decimal list-inside space-y-1 mt-2 ml-1">
                  <li>
                    Click <span className="font-medium">"Open Linear"</span> below
                  </li>
                  <li>
                    Set <span className="font-medium">Application name</span> to{" "}
                    <CopyChip text="CorticalStack" onCopy={(t) => copyText(t, "Name")} />
                  </li>
                  <li>
                    Set <span className="font-medium">Redirect URI</span> to:{" "}
                    {status?.redirect_uri && (
                      <CopyChip
                        text={status.redirect_uri}
                        onCopy={(t) => copyText(t, "Redirect URI")}
                      />
                    )}
                  </li>
                  <li>
                    Leave the default scopes (<span className="font-mono">read</span>,{" "}
                    <span className="font-mono">write</span>) and create the app
                  </li>
                  <li>
                    Linear will show a <span className="font-medium">Client ID</span> and{" "}
                    <span className="font-medium">Client Secret</span> — keep that page open for step 2
                  </li>
                </ol>
              </div>

              <div className="flex items-center gap-2">
                <a
                  href="https://linear.app/settings/api/applications/new"
                  target="_blank"
                  rel="noreferrer"
                  onClick={() => setRegisterAck(true)}
                >
                  <Button
                    size="sm"
                    className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal text-xs gap-1.5"
                  >
                    <ExternalLink className="h-3.5 w-3.5" />
                    Open Linear
                  </Button>
                </a>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => setWizardStep(2)}
                  disabled={!registerAck}
                  className="border-border rounded-sm font-normal text-xs"
                >
                  I've created the app →
                </Button>
              </div>
              {!registerAck && (
                <p className="text-[11px] text-muted-foreground">
                  Open Linear first, then come back here to continue.
                </p>
              )}

              <AdvancedToggle open={showAdvanced} onToggle={() => setShowAdvanced(!showAdvanced)} />
            </div>
          )}

          {/* Step 2: Paste client_id / client_secret */}
          {wizardStep === 2 && (
            <div className="space-y-3">
              <div className="rounded-sm border border-border bg-muted/40 px-3 py-2.5 text-xs">
                <p className="font-medium text-foreground mb-1">Paste from Linear</p>
                <p className="text-muted-foreground">
                  Tip: paste both values at once into either field — CorticalStack will detect them.
                </p>
              </div>

              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-xs font-normal">Client ID</Label>
                <Input
                  value={clientID}
                  onChange={(e) => setClientID(e.target.value)}
                  onPaste={(e) => {
                    const text = e.clipboardData.getData("text")
                    if (handleSmartPaste(text)) {
                      e.preventDefault()
                    }
                  }}
                  placeholder="lin_oauth_..."
                  className="border-border rounded-sm text-xs font-mono"
                />
              </div>
              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-xs font-normal">Client Secret</Label>
                <div className="relative">
                  <Input
                    type={showSecret ? "text" : "password"}
                    value={clientSecret}
                    onChange={(e) => setClientSecret(e.target.value)}
                    onPaste={(e) => {
                      const text = e.clipboardData.getData("text")
                      if (handleSmartPaste(text)) {
                        e.preventDefault()
                      }
                    }}
                    placeholder="(longer secret string)"
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
              </div>

              <div className="flex items-center gap-2">
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => setWizardStep(1)}
                  className="text-xs font-normal"
                >
                  ← Back
                </Button>
                <Button
                  size="sm"
                  onClick={() => saveOAuthAppMutation.mutate()}
                  disabled={
                    !clientID.trim() || !clientSecret.trim() || saveOAuthAppMutation.isPending
                  }
                  className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal text-xs"
                >
                  {saveOAuthAppMutation.isPending ? "Saving..." : "Save and continue →"}
                </Button>
              </div>

              <AdvancedToggle open={showAdvanced} onToggle={() => setShowAdvanced(!showAdvanced)} />
            </div>
          )}

          {/* Step 3: Connect */}
          {wizardStep === 3 && (
            <div className="space-y-3">
              <div className="rounded-sm border border-[rgba(21,190,83,0.4)] bg-[rgba(21,190,83,0.06)] px-3 py-2.5 text-xs">
                <p className="font-medium text-foreground flex items-center gap-1.5">
                  <Check className="h-3.5 w-3.5 text-[var(--stripe-success-text,#15be53)]" />
                  OAuth app is registered
                </p>
                <p className="text-muted-foreground mt-1">
                  Click <span className="font-medium">Connect to Linear</span> — your browser
                  will redirect to Linear, you'll approve access, and it'll send you back here.
                </p>
              </div>

              <a href="/oauth/linear/start">
                <Button
                  size="sm"
                  className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal text-xs gap-1.5"
                >
                  <Link2 className="h-3.5 w-3.5" />
                  Connect to Linear
                </Button>
              </a>

              <AdvancedToggle open={showAdvanced} onToggle={() => setShowAdvanced(!showAdvanced)} />
            </div>
          )}

          {showAdvanced && (
            <PersonalKeyForm
              apiKey={apiKey}
              setApiKey={(v) => {
                setApiKey(v)
                setTestPassed(false)
                setTestError(undefined)
                setTestWarning(undefined)
              }}
              teamKey={teamKey}
              setTeamKey={(v) => {
                setTeamKey(v)
                setTestPassed(false)
                setTestError(undefined)
                setTestWarning(undefined)
              }}
              showKey={showKey}
              toggleShowKey={() => setShowKey(!showKey)}
              testPassed={testPassed}
              testWarning={testWarning}
              onTest={() => {
                setTestPassed(false)
                setTestError(undefined)
                setTestWarning(undefined)
                testMutation.mutate()
              }}
              onSave={() => saveMutation.mutate()}
              isTesting={testMutation.isPending}
              isSaving={saveMutation.isPending}
            />
          )}
        </div>
      )}
    </IntegrationCard>
  )
}

function StepStrip({ current, steps }: { current: 1 | 2 | 3; steps: string[] }) {
  return (
    <div className="flex items-center gap-2">
      {steps.map((label, idx) => {
        const n = (idx + 1) as 1 | 2 | 3
        const done = n < current
        const active = n === current
        return (
          <div key={label} className="flex items-center gap-2 flex-1">
            <div
              className={[
                "flex h-5 w-5 items-center justify-center rounded-full text-[10px] font-medium border shrink-0",
                done
                  ? "bg-[rgba(21,190,83,0.15)] border-[rgba(21,190,83,0.4)] text-[var(--stripe-success-text,#15be53)]"
                  : active
                    ? "bg-primary border-primary text-primary-foreground"
                    : "bg-muted border-border text-muted-foreground",
              ].join(" ")}
            >
              {done ? <Check className="h-3 w-3" /> : n}
            </div>
            <span
              className={[
                "text-[11px] truncate",
                active ? "text-foreground font-medium" : "text-muted-foreground",
              ].join(" ")}
            >
              {label}
            </span>
            {idx < steps.length - 1 && (
              <div className="flex-1 h-px bg-border" />
            )}
          </div>
        )
      })}
    </div>
  )
}

function CopyChip({ text, onCopy }: { text: string; onCopy: (t: string) => void }) {
  return (
    <span className="inline-flex items-center gap-1 align-middle">
      <code className="font-mono text-[10px] bg-background border border-border rounded-sm px-1 py-0.5 break-all">
        {text}
      </code>
      <Button
        type="button"
        size="sm"
        variant="ghost"
        onClick={() => onCopy(text)}
        className="h-5 w-5 p-0 shrink-0"
      >
        <Copy className="h-3 w-3" />
      </Button>
    </span>
  )
}

function AdvancedToggle({ open, onToggle }: { open: boolean; onToggle: () => void }) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className="text-[11px] text-muted-foreground hover:text-foreground underline"
    >
      {open ? "Hide" : "Show"} advanced (personal API key)
    </button>
  )
}

function PersonalKeyForm({
  apiKey,
  setApiKey,
  teamKey,
  setTeamKey,
  showKey,
  toggleShowKey,
  testPassed,
  testWarning,
  onTest,
  onSave,
  isTesting,
  isSaving,
}: {
  apiKey: string
  setApiKey: (v: string) => void
  teamKey: string
  setTeamKey: (v: string) => void
  showKey: boolean
  toggleShowKey: () => void
  testPassed: boolean
  testWarning?: string
  onTest: () => void
  onSave: () => void
  isTesting: boolean
  isSaving: boolean
}) {
  return (
    <div className="space-y-3 border-t border-border pt-3">
      <div className="text-[11px] text-muted-foreground bg-muted/30 border border-dashed border-border rounded-sm px-3 py-2">
        Personal API key bypass — for headless / CI use. Linear → Settings → Account → Security &amp; Access → New API key.
      </div>
      <div className="space-y-2">
        <Label className="text-[var(--stripe-label)] text-xs font-normal">API Key</Label>
        <div className="relative">
          <Input
            type={showKey ? "text" : "password"}
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            placeholder="lin_api_..."
            className="border-border rounded-sm text-xs font-mono pr-10"
          />
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={toggleShowKey}
            className="absolute right-1 top-1/2 -translate-y-1/2 h-6 w-6 p-0"
          >
            {showKey ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
          </Button>
        </div>
      </div>
      <div className="space-y-2">
        <Label className="text-[var(--stripe-label)] text-xs font-normal">Default Team Key</Label>
        <Input
          value={teamKey}
          onChange={(e) => setTeamKey(e.target.value)}
          placeholder="BCN"
          className="border-border rounded-sm text-xs font-mono"
        />
      </div>
      {testWarning && (
        <p className="text-xs text-yellow-600 bg-yellow-500/5 border border-yellow-500/20 rounded-sm px-3 py-2">
          {testWarning}
        </p>
      )}
      <div className="flex items-center gap-2">
        <Button
          size="sm"
          variant="outline"
          onClick={onTest}
          disabled={!apiKey.trim() || isTesting || isSaving}
          className="border-border rounded-sm font-normal text-xs"
        >
          {isTesting ? "Testing..." : "Test"}
        </Button>
        <Button
          size="sm"
          onClick={onSave}
          disabled={!testPassed || isSaving || isTesting}
          className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal text-xs"
        >
          {isSaving ? "Saving..." : "Save"}
        </Button>
      </div>
    </div>
  )
}

function WebhookSecretField({
  value,
  onChange,
  show,
  onToggle,
  secretConfigured,
  lastWebhookAt,
}: {
  value: string
  onChange: (v: string) => void
  show: boolean
  onToggle: () => void
  secretConfigured?: boolean
  lastWebhookAt?: string
}) {
  return (
    <div className="space-y-2">
      <Label className="text-[var(--stripe-label)] text-xs font-normal">
        Webhook Secret <span className="text-muted-foreground/60">(for inbound sync)</span>
      </Label>
      <div className="relative">
        <Input
          type={show ? "text" : "password"}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={
            secretConfigured
              ? "Webhook secret saved — enter new value to rotate"
              : "Set after registering webhook in Linear"
          }
          className="border-border rounded-sm text-xs font-mono pr-10"
        />
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={onToggle}
          className="absolute right-1 top-1/2 -translate-y-1/2 h-6 w-6 p-0"
        >
          {show ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
        </Button>
      </div>
      {secretConfigured && (
        <p className="text-[11px] text-muted-foreground">
          {lastWebhookAt
            ? `Last webhook received: ${new Date(lastWebhookAt).toLocaleString()}`
            : "No webhooks received yet."}
        </p>
      )}
    </div>
  )
}

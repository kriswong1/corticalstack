import { useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { IntegrationCard, type IntegrationState } from "./integration-card"
import { api, getErrorMessage } from "@/lib/api"

export function ObsidianCard() {
  const queryClient = useQueryClient()
  const { data: status } = useQuery({
    queryKey: ["status"],
    queryFn: api.getStatus,
  })

  const savedPath = status?.vault_path ?? ""
  const [vaultPath, setVaultPath] = useState("")
  const [testPassed, setTestPassed] = useState(false)
  const [testError, setTestError] = useState<string>()
  const [initialized, setInitialized] = useState(false)

  // Seed input from status on first load
  if (savedPath && !initialized) {
    setVaultPath(savedPath)
    setInitialized(true)
  }

  const pathChanged = vaultPath !== savedPath

  const testMutation = useMutation({
    mutationFn: () => api.testObsidian({ vault_path: vaultPath }),
    onSuccess: (res) => {
      if (res.ok) {
        setTestPassed(true)
        setTestError(undefined)
        toast.success("Vault path verified")
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
    mutationFn: () => api.saveObsidian({ vault_path: vaultPath }),
    onSuccess: () => {
      toast.success("Vault path saved")
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
        : savedPath
          ? "connected"
          : "not_configured"

  return (
    <IntegrationCard
      title="Obsidian"
      description="Connect your Obsidian vault. CorticalStack writes notes and extractions here."
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
      disabled={!vaultPath.trim()}
    >
      <div className="space-y-2">
        <Label className="text-[var(--stripe-label)] text-xs font-normal">Vault Path</Label>
        <Input
          value={vaultPath}
          onChange={(e) => {
            setVaultPath(e.target.value)
            setTestPassed(false)
            setTestError(undefined)
          }}
          placeholder="/path/to/your/obsidian/vault"
          className="border-border rounded-sm text-xs font-mono"
        />
      </div>
      {pathChanged && savedPath && (
        <p className="text-[11px] text-yellow-600 bg-yellow-500/5 border border-yellow-500/20 rounded-sm px-3 py-2">
          Changing your vault location won't lose any data, but CorticalStack will need to re-index
          against the new path. Existing extractions remain intact.
        </p>
      )}
    </IntegrationCard>
  )
}

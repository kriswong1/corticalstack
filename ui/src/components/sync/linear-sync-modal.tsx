import { useState, useEffect } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"
import { api, getErrorMessage } from "@/lib/api"
import type { LinearSyncPreview, LinearSyncResult } from "@/types/api"
import { Loader2, AlertTriangle, CheckCircle2 } from "lucide-react"

interface Props {
  projectId: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

// LinearSyncModal renders the two-step preview/confirm flow:
// 1. On open, calls POST /api/projects/{id}/sync (dry-run) and shows
//    the diff Linear would receive.
// 2. User clicks Confirm → POST with ?confirm=1, surfaces the result.
//
// Closing the modal mid-flow is fine — no in-flight write happens
// during the preview step.
export function LinearSyncModal({ projectId, open, onOpenChange }: Props) {
  const queryClient = useQueryClient()
  const [preview, setPreview] = useState<LinearSyncPreview | undefined>()
  const [result, setResult] = useState<LinearSyncResult | undefined>()

  const previewMutation = useMutation({
    mutationFn: () => api.previewProjectSync(projectId),
    onSuccess: (resp) => {
      setPreview(resp.preview)
      setResult(undefined)
    },
    onError: (err) => {
      toast.error(getErrorMessage(err))
      onOpenChange(false)
    },
  })

  const confirmMutation = useMutation({
    mutationFn: () => api.confirmProjectSync(projectId),
    onSuccess: (resp) => {
      setResult(resp.result)
      const created = resp.result?.created?.length ?? 0
      const updated = resp.result?.updated?.length ?? 0
      const errors = resp.result?.errors?.length ?? 0
      if (errors > 0) {
        toast.error(`Synced with ${errors} error(s)`)
      } else {
        toast.success(`Synced: ${created} created, ${updated} updated`)
      }
      queryClient.invalidateQueries({ queryKey: ["projects"] })
      queryClient.invalidateQueries({ queryKey: ["project-content"] })
    },
    onError: (err) => toast.error(getErrorMessage(err)),
  })

  // Kick off the dry-run when the modal opens; reset state when closed.
  useEffect(() => {
    if (open) {
      setPreview(undefined)
      setResult(undefined)
      previewMutation.mutate()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, projectId])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="shadow-stripe-deep rounded-md max-w-lg">
        <DialogHeader>
          <DialogTitle className="text-[22px] font-light tracking-[-0.22px]">
            Sync to Linear
          </DialogTitle>
        </DialogHeader>

        {previewMutation.isPending && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground py-4">
            <Loader2 className="h-4 w-4 animate-spin" />
            Computing dry-run preview...
          </div>
        )}

        {preview && !result && (
          <div className="space-y-3 text-sm">
            <p className="text-muted-foreground">
              Target team:{" "}
              <Badge variant="outline" className="font-mono text-[10px]">
                {preview.team_key}
              </Badge>
            </p>
            {preview.initiative_action && (
              <Row
                label="Initiative"
                value={`${preview.initiative_action} · ${preview.initiative_name ?? ""}`}
              />
            )}
            <Row label="Project" value={preview.project_action} />
            {(preview.documents_to_create > 0 || preview.documents_to_update > 0) && (
              <Row
                label="Documents"
                value={`${preview.documents_to_create} new · ${preview.documents_to_update} updated`}
              />
            )}
            {preview.document_titles && preview.document_titles.length > 0 && (
              <ul className="ml-4 list-disc text-xs text-muted-foreground space-y-0.5">
                {preview.document_titles.map((t) => (
                  <li key={t} className="font-mono">{t}</li>
                ))}
              </ul>
            )}
            {(preview.issues_to_create > 0 || preview.issues_to_update > 0) && (
              <Row
                label="Issues"
                value={`${preview.issues_to_create} new · ${preview.issues_to_update} updated`}
              />
            )}
            {preview.warnings && preview.warnings.length > 0 && (
              <div className="rounded-sm border border-amber-500/40 bg-amber-50/10 p-2 text-xs text-amber-300 space-y-1">
                {preview.warnings.map((w, i) => (
                  <div key={i} className="flex items-start gap-1.5">
                    <AlertTriangle className="h-3.5 w-3.5 mt-px flex-shrink-0" />
                    <span>{w}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {result && (
          <div className="space-y-3 text-sm">
            <div className="flex items-center gap-2 text-[var(--stripe-success-text,#15be53)]">
              <CheckCircle2 className="h-4 w-4" /> Sync complete
            </div>
            {result.created && result.created.length > 0 && (
              <Section title="Created" items={result.created} />
            )}
            {result.updated && result.updated.length > 0 && (
              <Section title="Updated" items={result.updated} />
            )}
            {result.errors && result.errors.length > 0 && (
              <div className="rounded-sm border border-destructive/40 bg-destructive/5 p-2 text-xs text-destructive space-y-1">
                <div className="font-medium">Errors</div>
                {result.errors.map((e, i) => (
                  <div key={i}>
                    <span className="font-mono">{e.entity}</span>: {e.error}
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        <DialogFooter>
          {!result && preview && (
            <>
              <Button variant="outline" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button
                onClick={() => confirmMutation.mutate()}
                disabled={confirmMutation.isPending}
                className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
              >
                {confirmMutation.isPending ? (
                  <>
                    <Loader2 className="h-3.5 w-3.5 animate-spin mr-1.5" />
                    Syncing...
                  </>
                ) : (
                  "Confirm Sync"
                )}
              </Button>
            </>
          )}
          {result && (
            <Button onClick={() => onOpenChange(false)}>Close</Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline gap-3">
      <span className="text-xs uppercase tracking-wide text-muted-foreground/60 w-24">{label}</span>
      <span className="font-mono text-xs">{value}</span>
    </div>
  )
}

function Section({ title, items }: { title: string; items: string[] }) {
  return (
    <div>
      <div className="text-xs uppercase tracking-wide text-muted-foreground/60 mb-1">{title}</div>
      <ul className="ml-4 list-disc text-xs space-y-0.5">
        {items.map((it) => (
          <li key={it} className="font-mono">{it}</li>
        ))}
      </ul>
    </div>
  )
}

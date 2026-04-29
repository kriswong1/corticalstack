import { useState, useEffect } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"
import { api, getErrorMessage } from "@/lib/api"
import type { LinearGeneratePreview, LinearGenerateResult } from "@/types/api"
import { Loader2, AlertTriangle, CheckCircle2 } from "lucide-react"

interface Props {
  projectId: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

// LinearGenerateModal — additive-only Issue + Milestone generation
// from the linked PRD's §S4.9 / §S4.5 / Slice Plan sections.
//
// Per Q8 lock: existing Issues/Milestones (mapped in the PRD's
// .linear.json sidecar) are never modified or deleted on re-run; only
// new criterion hashes produce new Issues.
export function LinearGenerateModal({ projectId, open, onOpenChange }: Props) {
  const queryClient = useQueryClient()
  const [preview, setPreview] = useState<LinearGeneratePreview | undefined>()
  const [result, setResult] = useState<LinearGenerateResult | undefined>()

  const previewMutation = useMutation({
    mutationFn: () => api.previewGenerateIssues(projectId),
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
    mutationFn: () => api.confirmGenerateIssues(projectId),
    onSuccess: (resp) => {
      setResult(resp.result)
      const issues = resp.result?.issues_created?.length ?? 0
      const milestones = resp.result?.milestones_created?.length ?? 0
      const errors = resp.result?.errors?.length ?? 0
      if (errors > 0) {
        toast.error(`Generated with ${errors} error(s)`)
      } else {
        toast.success(`Generated: ${issues} issue(s), ${milestones} milestone(s)`)
      }
      queryClient.invalidateQueries({ queryKey: ["project-content"] })
    },
    onError: (err) => toast.error(getErrorMessage(err)),
  })

  useEffect(() => {
    if (open) {
      setPreview(undefined)
      setResult(undefined)
      previewMutation.mutate()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, projectId])

  const nothingToDo =
    preview && preview.issues_to_create === 0 && preview.milestones_to_create === 0

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="shadow-stripe-deep rounded-md max-w-lg">
        <DialogHeader>
          <DialogTitle className="text-[22px] font-light tracking-[-0.22px]">
            Generate Issues from PRD
          </DialogTitle>
        </DialogHeader>

        {previewMutation.isPending && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground py-4">
            <Loader2 className="h-4 w-4 animate-spin" />
            Parsing acceptance criteria...
          </div>
        )}

        {preview && !result && (
          <div className="space-y-3 text-sm">
            <p className="text-muted-foreground text-xs">
              Source: <span className="font-mono">{preview.prd_id}</span> · §S4.9 Acceptance Criteria + §S4.7 Slice Plan
            </p>
            <div className="space-y-1">
              <Row
                label="New Issues"
                value={`${preview.issues_to_create} (${preview.issues_already_mapped} already mapped, untouched)`}
              />
              <Row
                label="New Milestones"
                value={`${preview.milestones_to_create} (${preview.milestones_already_mapped} already mapped)`}
              />
            </div>
            {preview.new_criterion_texts && preview.new_criterion_texts.length > 0 && (
              <div>
                <div className="text-xs uppercase tracking-wide text-muted-foreground/60 mb-1">
                  New criteria
                </div>
                <ul className="ml-4 list-disc text-xs space-y-0.5 max-h-48 overflow-auto">
                  {preview.new_criterion_texts.map((t, i) => (
                    <li key={i}>{t}</li>
                  ))}
                </ul>
              </div>
            )}
            {preview.new_milestone_names && preview.new_milestone_names.length > 0 && (
              <div>
                <div className="text-xs uppercase tracking-wide text-muted-foreground/60 mb-1">
                  New milestones
                </div>
                <ul className="ml-4 list-disc text-xs space-y-0.5">
                  {preview.new_milestone_names.map((n, i) => (
                    <li key={i} className="font-mono">{n}</li>
                  ))}
                </ul>
              </div>
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
            {nothingToDo && (
              <p className="text-xs text-muted-foreground italic">
                Everything in the PRD is already mapped — nothing to generate.
              </p>
            )}
          </div>
        )}

        {result && (
          <div className="space-y-3 text-sm">
            <div className="flex items-center gap-2 text-[var(--stripe-success-text,#15be53)]">
              <CheckCircle2 className="h-4 w-4" /> Generation complete
            </div>
            {result.issues_created && result.issues_created.length > 0 && (
              <Section title={`Issues created (${result.issues_created.length})`} items={result.issues_created} />
            )}
            {result.milestones_created && result.milestones_created.length > 0 && (
              <Section title={`Milestones created (${result.milestones_created.length})`} items={result.milestones_created} />
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
                disabled={confirmMutation.isPending || nothingToDo}
                className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
              >
                {confirmMutation.isPending ? (
                  <>
                    <Loader2 className="h-3.5 w-3.5 animate-spin mr-1.5" />
                    Generating...
                  </>
                ) : (
                  "Confirm Generate"
                )}
              </Button>
            </>
          )}
          {result && <Button onClick={() => onOpenChange(false)}>Close</Button>}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline gap-3">
      <span className="text-xs uppercase tracking-wide text-muted-foreground/60 w-32">{label}</span>
      <span className="font-mono text-xs">{value}</span>
    </div>
  )
}

function Section({ title, items }: { title: string; items: string[] }) {
  return (
    <div>
      <div className="text-xs uppercase tracking-wide text-muted-foreground/60 mb-1">{title}</div>
      <ul className="ml-4 list-disc text-xs space-y-0.5 max-h-32 overflow-auto">
        {items.map((it, i) => (
          <li key={i}>{it}</li>
        ))}
      </ul>
    </div>
  )
}

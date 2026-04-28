import { useMemo, useState } from "react"
import { useNavigate, useParams } from "react-router-dom"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import Markdown from "react-markdown"
import { Breadcrumbs } from "@/components/layout/breadcrumbs"
import { ProductSubnav } from "@/components/product-subnav"
import { PageHeader } from "@/components/layout/page-header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent } from "@/components/ui/card"
import { QuestionsModal } from "@/components/questions-modal"
import { api, getErrorMessage } from "@/lib/api"
import { cn } from "@/lib/utils"
import { CheckCircle2, Loader2, RefreshCw } from "lucide-react"
import type { Answer, Question } from "@/types/api"

// Mirror of the PRD-refinement pattern from /prototypes/:id, adapted
// for the flatter PRD lifecycle (draft → review → approved → shipped).
// Lets the user review the rendered body, inspect archived versions,
// iterate via a hints prompt, and advance status — all without leaving
// the PRD-centric list context.

const PRD_ACCENT = "#48D597"

// Simple status arc the UI surfaces. Matches the draft→review→final
// flow the user expects; the backend also supports "archived" as an
// off-arc terminal state, left out of the advance sequence but still
// valid server-side.
const PRD_STATUS_ORDER = ["draft", "review", "approved", "shipped"] as const

const PRD_STATUS_LABEL: Record<string, string> = {
  draft: "Draft",
  review: "Review",
  approved: "Approved",
  shipped: "Shipped",
  archived: "Archived",
}

function nextStatus(current: string): string | null {
  const i = PRD_STATUS_ORDER.indexOf(current as (typeof PRD_STATUS_ORDER)[number])
  if (i < 0 || i >= PRD_STATUS_ORDER.length - 1) return null
  return PRD_STATUS_ORDER[i + 1]
}

function advanceLabel(current: string): string {
  const next = nextStatus(current)
  if (!next) return "No further stages"
  if (next === "review") return "Move to Review"
  if (next === "approved") return "Mark Approved"
  if (next === "shipped") return "Mark Shipped"
  return `Mark ${PRD_STATUS_LABEL[next]}`
}

// Hex → rgba() wrapper used for the status-tinted chrome on the
// refine card. Keeps the tint behavior visually consistent with the
// prototype detail page.
function withAlpha(hex: string, alpha: number): string {
  const h = hex.replace("#", "")
  const r = parseInt(h.slice(0, 2), 16)
  const g = parseInt(h.slice(2, 4), 16)
  const b = parseInt(h.slice(4, 6), 16)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

function stripFrontmatter(raw: string): string {
  const trimmed = raw.trimStart()
  if (!trimmed.startsWith("---")) return raw
  const end = trimmed.indexOf("---", 3)
  if (end < 0) return raw
  return trimmed.slice(end + 3).trimStart()
}

export function PRDDetailPage() {
  const { id = "" } = useParams<{ id: string }>()
  const queryClient = useQueryClient()
  const navigate = useNavigate()

  // One list fetch serves both the detail lookup and the list
  // invalidation contract — mutations on /prds invalidate ["prds"]
  // and this page picks up the fresh record automatically.
  const { data: allPRDs, isLoading } = useQuery({
    queryKey: ["prds"],
    queryFn: api.listPRDs,
  })
  const prd = useMemo(
    () => (allPRDs ?? []).find((p) => p.id === id) ?? null,
    [allPRDs, id],
  )

  // Live body comes from the vault file. Archived bodies come from the
  // version endpoint — selected via the switcher state below.
  const { data: liveRaw, isLoading: bodyLoading } = useQuery<string>({
    queryKey: ["vault-file", prd?.path],
    queryFn: () => api.getVaultFile(prd!.path!),
    enabled: !!prd?.path,
    staleTime: 30_000,
  })

  const { data: versions } = useQuery({
    queryKey: ["prd-versions", id],
    queryFn: () => api.listPRDVersions(id),
    enabled: !!id,
  })
  const versionsDesc = useMemo(
    () => (versions ?? []).slice().reverse(),
    [versions],
  )

  // null = live current version; otherwise the selected archived vN.
  const [selectedVersion, setSelectedVersion] = useState<number | null>(null)
  const { data: archivedBody } = useQuery<string>({
    queryKey: ["prd-version-body", id, selectedVersion],
    queryFn: () => api.getPRDVersionBody(id, selectedVersion!),
    enabled: selectedVersion !== null,
    staleTime: 30_000,
  })

  const body = (() => {
    if (selectedVersion !== null && archivedBody) {
      return stripFrontmatter(archivedBody)
    }
    return liveRaw ? stripFrontmatter(liveRaw) : null
  })()

  // Refine: primary inline hints path + Q&A fallback. Mirrors the
  // prototype refine card — Apply button = new version.
  const [hints, setHints] = useState("")
  const [qaModalOpen, setQaModalOpen] = useState(false)
  const [questions, setQuestions] = useState<Question[] | null>(null)

  const refineMutation = useMutation({
    mutationFn: (answers: Answer[]) =>
      api.refinePRD(id, {
        hints: hints || undefined,
        questions: questions ?? undefined,
        answers: answers.length > 0 ? answers : undefined,
      }),
    onSuccess: (next) => {
      toast.success(`PRD refined → v${next.version}`)
      setHints("")
      setQuestions(null)
      setQaModalOpen(false)
      // Snap back to the live version so the user sees the new draft.
      setSelectedVersion(null)
      queryClient.invalidateQueries({ queryKey: ["prds"] })
      queryClient.invalidateQueries({ queryKey: ["prd-versions", id] })
      if (next.path) {
        queryClient.invalidateQueries({ queryKey: ["vault-file", next.path] })
      }
    },
    onError: (err) => {
      setQuestions(null)
      setQaModalOpen(false)
      toast.error(`Refine failed: ${getErrorMessage(err)}`)
    },
  })

  // Q&A questions for refine — reuse the create-PRD questions endpoint
  // with the existing pitch path so Claude can ground the refinement
  // in the same context the original PRD was synthesized from.
  const refineQuestionsMutation = useMutation({
    mutationFn: () => {
      if (!prd) throw new Error("PRD not loaded")
      return api.prdQuestions({
        pitch_path: prd.source_pitch,
        project_ids: prd.projects ?? [],
      })
    },
    onSuccess: (resp) => setQuestions(resp.questions ?? []),
    onError: (err) => {
      setQuestions([])
      toast.error(`Failed to fetch refine questions: ${getErrorMessage(err)}`)
    },
  })

  const statusMutation = useMutation({
    mutationFn: (status: string) => api.setPRDStatus(id, status),
    onSuccess: (_, status) => {
      toast.success(`PRD → ${PRD_STATUS_LABEL[status] ?? status}`)
      queryClient.invalidateQueries({ queryKey: ["prds"] })
    },
    onError: (err) => {
      toast.error(`Status update failed: ${getErrorMessage(err)}`)
    },
  })

  const isRefining = refineMutation.isPending || refineQuestionsMutation.isPending
  const currentStatus = prd?.status ?? "draft"
  const next = nextStatus(currentStatus)
  const isTerminal = !next

  if (isLoading) {
    return (
      <>
        <Breadcrumbs
          items={[
            { label: "Dashboard", to: "/dashboard" },
            { label: "PRDs", to: "/prds" },
            { label: "Loading…" },
          ]}
        />
        <ProductSubnav />
        <div className="py-10 text-center text-sm text-muted-foreground">
          <Loader2 className="inline h-4 w-4 animate-spin mr-2" />
          Loading PRD...
        </div>
      </>
    )
  }

  if (!prd) {
    return (
      <>
        <Breadcrumbs
          items={[
            { label: "Dashboard", to: "/dashboard" },
            { label: "PRDs", to: "/prds" },
            { label: "Not found" },
          ]}
        />
        <ProductSubnav />
        <Card className="rounded-md border-border bg-muted/30">
          <CardContent className="py-6">
            <p className="text-sm text-muted-foreground">
              PRD not found. It may have been archived or the id is wrong.
            </p>
            <Button
              variant="outline"
              size="sm"
              className="mt-3"
              onClick={() => navigate("/prds")}
            >
              Back to PRDs
            </Button>
          </CardContent>
        </Card>
      </>
    )
  }

  return (
    <>
      <Breadcrumbs
        items={[
          { label: "Dashboard", to: "/dashboard" },
          { label: "PRDs", to: "/prds" },
          { label: prd.title },
        ]}
      />
      <ProductSubnav />
      <PageHeader title={prd.title} description={prd.path || undefined}>
        <div className="flex items-center gap-2">
          <Badge
            variant="outline"
            className="rounded-sm text-[11px] font-normal"
          >
            {PRD_STATUS_LABEL[currentStatus] ?? currentStatus}
          </Badge>
          <Badge
            variant="outline"
            className="rounded-sm text-[11px] font-normal"
          >
            v{prd.version}
          </Badge>
          {prd.open_questions_count > 0 && (
            <Badge
              variant="outline"
              className="rounded-sm text-[11px] font-normal border-amber-500/50 text-amber-600 dark:text-amber-400"
            >
              {prd.open_questions_count} open question
              {prd.open_questions_count === 1 ? "" : "s"}
            </Badge>
          )}
          {!isTerminal && (
            <Button
              onClick={() => next && statusMutation.mutate(next)}
              disabled={statusMutation.isPending}
              className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal gap-1.5"
            >
              {statusMutation.isPending ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <CheckCircle2 className="h-3.5 w-3.5" />
              )}
              {advanceLabel(currentStatus)}
            </Button>
          )}
        </div>
      </PageHeader>

      {/* Refine card — primary interaction surface. Mirrors the prototype
          refine panel: inline hints → Apply, Q&A fallback, version
          switcher. Hidden when the PRD is already shipped — finalized
          artifacts shouldn't be quietly mutated. */}
      {currentStatus !== "shipped" && currentStatus !== "archived" && (
        <Card
          className="rounded-[14px] border-border shadow-stripe mb-5 mt-5"
          style={{ borderColor: withAlpha(PRD_ACCENT, 0.25) }}
        >
          <CardContent className="py-5 px-5 space-y-3">
            <div className="flex items-center gap-3 flex-wrap">
              <div className="flex items-center gap-2">
                <RefreshCw className="h-4 w-4" style={{ color: PRD_ACCENT }} />
                <span className="text-[13px] font-semibold text-foreground">
                  Refine this PRD
                </span>
                <span
                  className="text-[10px] font-bold tracking-[0.06em] uppercase px-2 py-0.5 rounded-full tabular-nums"
                  style={{
                    background: withAlpha(PRD_ACCENT, 0.15),
                    color: PRD_ACCENT,
                    border: `1px solid ${withAlpha(PRD_ACCENT, 0.3)}`,
                  }}
                >
                  Currently v{prd.version}
                </span>
              </div>
              {(versions?.length ?? 0) > 0 && (
                <span className="text-[11px] text-muted-foreground">
                  {versions!.length} past version
                  {versions!.length === 1 ? "" : "s"} archived
                </span>
              )}
            </div>

            <div className="flex items-start gap-3 flex-wrap">
              <Label className="sr-only" htmlFor="prd-refine-hints">
                What do you want to change?
              </Label>
              <Input
                id="prd-refine-hints"
                value={hints}
                onChange={(e) => setHints(e.target.value)}
                placeholder="Describe what to change (e.g. reframe rollout around mobile, drop the analytics goal)"
                className="flex-1 min-w-[280px] h-9 border-border rounded-sm"
                disabled={isRefining}
              />
              <Button
                onClick={() => refineMutation.mutate([])}
                disabled={isRefining || !hints.trim()}
                className="gap-2 rounded-lg font-semibold text-[12px] px-4 h-9"
                style={{
                  background: PRD_ACCENT,
                  color: "white",
                  boxShadow: `0 2px 8px ${withAlpha(PRD_ACCENT, 0.35)}`,
                }}
              >
                {refineMutation.isPending ? (
                  <>
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    Refining...
                  </>
                ) : (
                  <>Create v{prd.version + 1}</>
                )}
              </Button>
            </div>

            <button
              type="button"
              onClick={() => {
                setQuestions(null)
                setQaModalOpen(true)
                refineQuestionsMutation.mutate()
              }}
              disabled={isRefining || !hints.trim()}
              className="text-[11px] text-muted-foreground underline-offset-2 hover:text-foreground hover:underline disabled:opacity-50 disabled:hover:no-underline"
            >
              Need help framing?
            </button>

            {(versions?.length ?? 0) > 0 && (
              <div className="flex items-center gap-2 flex-wrap pt-2 border-t border-border">
                <span className="text-[11px] text-muted-foreground mr-1">
                  View:
                </span>
                <button
                  type="button"
                  onClick={() => setSelectedVersion(null)}
                  className={cn(
                    "text-[11px] font-medium px-2.5 py-1 rounded-md border transition-colors",
                    selectedVersion === null
                      ? "text-foreground"
                      : "text-muted-foreground hover:text-foreground border-transparent hover:border-border",
                  )}
                  style={
                    selectedVersion === null
                      ? {
                          background: withAlpha(PRD_ACCENT, 0.12),
                          borderColor: withAlpha(PRD_ACCENT, 0.4),
                        }
                      : undefined
                  }
                >
                  v{prd.version} (current)
                </button>
                {versionsDesc.map((v) => (
                  <button
                    key={v.version}
                    type="button"
                    onClick={() => setSelectedVersion(v.version)}
                    title={v.hints ? `Refinement prompt: ${v.hints}` : undefined}
                    className={cn(
                      "text-[11px] font-medium px-2.5 py-1 rounded-md border transition-colors",
                      selectedVersion === v.version
                        ? "text-foreground"
                        : "text-muted-foreground hover:text-foreground border-transparent hover:border-border",
                    )}
                    style={
                      selectedVersion === v.version
                        ? {
                            background: withAlpha(PRD_ACCENT, 0.12),
                            borderColor: withAlpha(PRD_ACCENT, 0.4),
                          }
                        : undefined
                    }
                  >
                    v{v.version}
                  </button>
                ))}
                {selectedVersion !== null && (
                  <span className="text-[11px] text-muted-foreground italic ml-auto">
                    Viewing archived v{selectedVersion} — read only
                  </span>
                )}
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {/* Rendered body — prose styling matches the hub preview so the
          same PRD looks the same everywhere. */}
      <Card className="rounded-[14px] border-border shadow-stripe">
        <CardContent className="py-5 px-5">
          {bodyLoading ? (
            <div className="py-10 text-center text-sm text-muted-foreground">
              <Loader2 className="inline h-4 w-4 animate-spin mr-2" />
              Loading body...
            </div>
          ) : body ? (
            <div className="prose prose-sm max-w-none dark:prose-invert prose-headings:text-foreground prose-p:text-foreground/90 prose-li:text-foreground/90 prose-strong:text-foreground prose-code:text-foreground/80 prose-code:bg-muted prose-code:px-1.5 prose-code:py-0.5 prose-code:rounded">
              <Markdown>{body}</Markdown>
            </div>
          ) : (
            <p className="py-10 text-center text-sm italic text-muted-foreground">
              No content found for this PRD.
            </p>
          )}
        </CardContent>
      </Card>

      <QuestionsModal
        open={qaModalOpen}
        onOpenChange={(next) => {
          if (!next && !refineMutation.isPending) {
            setQaModalOpen(false)
            setQuestions(null)
          }
        }}
        title="Refine PRD"
        description="Answer these so Claude can tailor the refinement to your intent."
        questions={questions}
        loading={refineQuestionsMutation.isPending}
        submitting={refineMutation.isPending}
        onSubmit={(answers) => refineMutation.mutate(answers)}
        onSkip={() => refineMutation.mutate([])}
      />
    </>
  )
}

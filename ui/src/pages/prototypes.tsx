import { useMemo, useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { PageHeader } from "@/components/layout/page-header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { QuestionsModal } from "@/components/questions-modal"
import { api, getErrorMessage } from "@/lib/api"
import { Plus, ExternalLink } from "lucide-react"
import type { Answer, Question, ShapeUpThread } from "@/types/api"

const formats = [
  "interactive-html",
  "screen-flow",
  "component-spec",
  "user-journey",
]

// A thread is prototype-ready once any of its artifacts has reached the
// breadboard stage. Current stage may be breadboard or pitch — we include
// the breadboard artifact plus all earlier stages as source context.
function isPrototypeReady(t: ShapeUpThread): boolean {
  return t.artifacts.some((a) => a.stage === "breadboard")
}

// Source paths for a thread = every artifact except the raw idea (too
// unstructured) so Claude has the full shaped arc plus the breadboard.
function sourcesFromThread(t: ShapeUpThread): string[] {
  return t.artifacts
    .filter((a) => a.stage !== "raw" && a.path)
    .map((a) => a.path)
}

export function PrototypesPage() {
  const queryClient = useQueryClient()
  const [showForm, setShowForm] = useState(false)
  const [title, setTitle] = useState("")
  const [threadId, setThreadId] = useState("")
  const [format, setFormat] = useState("interactive-html")
  const [hints, setHints] = useState("")
  const [modalOpen, setModalOpen] = useState(false)
  const [questions, setQuestions] = useState<Question[] | null>(null)

  const { data: prototypes, isLoading } = useQuery({
    queryKey: ["prototypes"],
    queryFn: api.listPrototypes,
  })

  const { data: threads } = useQuery({
    queryKey: ["shapeup-threads"],
    queryFn: api.listThreads,
  })

  const readyThreads = useMemo(
    () => (threads ?? []).filter(isPrototypeReady),
    [threads],
  )

  const selectedThread = readyThreads.find((t) => t.id === threadId)
  const sourcePaths = selectedThread ? sourcesFromThread(selectedThread) : []

  const canGenerate = !!selectedThread && title.trim().length > 0

  const questionsMutation = useMutation({
    mutationFn: () =>
      api.prototypeQuestions({
        title,
        format,
        source_paths: sourcePaths,
        hints,
      }),
    onSuccess: (resp) => {
      setQuestions(resp.questions ?? [])
    },
    // Pre-flight — fall through to empty list so the user can still
    // generate, but surface the error so they know why the prompt is
    // missing.
    onError: (err) => {
      setQuestions([])
      toast.error(`Failed to fetch prototype questions: ${getErrorMessage(err)}`)
    },
  })

  const createMutation = useMutation({
    mutationFn: (answers: Answer[]) =>
      api.createPrototype({
        title,
        source_paths: sourcePaths,
        format,
        hints,
        source_thread: threadId,
        questions: questions ?? undefined,
        answers: answers.length > 0 ? answers : undefined,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["prototypes"] })
      setShowForm(false)
      setTitle("")
      setThreadId("")
      setHints("")
      setQuestions(null)
      setModalOpen(false)
      toast.success("Prototype generated")
    },
    onError: (err) => {
      setQuestions(null)
      setModalOpen(false)
      toast.error(`Prototype generation failed: ${getErrorMessage(err)}`)
    },
  })

  const startGenerate = () => {
    if (!canGenerate) return
    setQuestions(null)
    setModalOpen(true)
    questionsMutation.mutate()
  }

  const openForm = () => {
    // Toggle the form card open/closed. The title is seeded from the
    // selected thread inside the Select's `onValueChange` below, not
    // here — opening the form with no thread selected keeps the title
    // input empty until the user picks one.
    setShowForm((prev) => !prev)
  }

  const noReadyThreads = (threads ?? []).length > 0 && readyThreads.length === 0
  const noThreadsAtAll = (threads ?? []).length === 0

  return (
    <>
      <PageHeader
        title="Prototypes"
        description="Generated from product threads that have reached a breadboard"
      >
        <Button
          onClick={openForm}
          disabled={readyThreads.length === 0}
          title={
            readyThreads.length === 0
              ? "Advance a thread to the breadboard stage first"
              : undefined
          }
          className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <Plus className="h-4 w-4" /> New Prototype
        </Button>
      </PageHeader>

      {(noReadyThreads || noThreadsAtAll) && (
        <Card className="mb-6 rounded-md border-border bg-muted/30">
          <CardContent className="py-4">
            <p className="text-sm text-muted-foreground">
              {noThreadsAtAll
                ? "No product threads yet — create an idea in Product and advance it to the breadboard stage before generating a prototype."
                : "No threads have reached the breadboard stage yet. Advance one in Product → breadboard to enable prototype generation."}
            </p>
          </CardContent>
        </Card>
      )}

      {showForm && readyThreads.length > 0 && (
        <Card className="mb-6 rounded-md border-border shadow-stripe">
          <CardContent className="pt-6">
            <form
              className="space-y-4"
              onSubmit={(e) => {
                e.preventDefault()
                startGenerate()
              }}
            >
              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">
                  Source thread (must have a breadboard)
                </Label>
                <Select
                  value={threadId}
                  onValueChange={(id) => {
                    setThreadId(id)
                    const t = readyThreads.find((x) => x.id === id)
                    if (t && !title.trim()) {
                      setTitle(t.title)
                    }
                  }}
                >
                  <SelectTrigger className="border-border rounded-sm">
                    <SelectValue placeholder="Pick a thread..." />
                  </SelectTrigger>
                  <SelectContent>
                    {readyThreads.map((t) => (
                      <SelectItem key={t.id} value={t.id}>
                        {t.title} · {t.current_stage}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {selectedThread && (
                  <p className="text-[11px] text-muted-foreground font-mono">
                    {sourcePaths.length} source file
                    {sourcePaths.length === 1 ? "" : "s"} from this thread
                  </p>
                )}
              </div>

              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">
                    Title
                  </Label>
                  <Input
                    value={title}
                    onChange={(e) => setTitle(e.target.value)}
                    className="border-border rounded-sm"
                  />
                </div>
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">
                    Format
                  </Label>
                  <Select value={format} onValueChange={setFormat}>
                    <SelectTrigger className="border-border rounded-sm">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {formats.map((f) => (
                        <SelectItem key={f} value={f}>
                          {f}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>

              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">
                  Hints
                </Label>
                <Input
                  value={hints}
                  onChange={(e) => setHints(e.target.value)}
                  className="border-border rounded-sm"
                />
              </div>

              <Button
                type="submit"
                disabled={
                  createMutation.isPending ||
                  questionsMutation.isPending ||
                  !canGenerate
                }
                className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
              >
                {createMutation.isPending ? "Generating..." : "Generate Prototype"}
              </Button>
            </form>
          </CardContent>
        </Card>
      )}

      {isLoading ? (
        <p className="text-muted-foreground">Loading...</p>
      ) : (
        <div className="rounded-md border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">
                  Title
                </TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">
                  Format
                </TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">
                  Status
                </TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">
                  Created
                </TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal w-20"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {prototypes?.map((p) => (
                <TableRow key={p.id}>
                  <TableCell className="text-sm font-light">{p.title}</TableCell>
                  <TableCell>
                    <Badge
                      variant="outline"
                      className="text-[10px] font-normal rounded-sm px-1.5"
                    >
                      {p.format}
                    </Badge>
                  </TableCell>
                  <TableCell>
                    <Badge className="text-[10px] font-light rounded-sm px-1.5 py-px bg-secondary text-secondary-foreground">
                      {p.status}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground font-mono">
                    {new Date(p.created).toLocaleDateString()}
                  </TableCell>
                  <TableCell>
                    {p.has_html && (
                      <a
                        href={api.prototypeHTMLUrl(p.id)}
                        target="_blank"
                        rel="noreferrer"
                        className="inline-flex items-center gap-1 rounded-sm border border-border px-1.5 py-0.5 text-[10px] font-normal text-primary hover:border-primary/60"
                      >
                        <ExternalLink className="h-3 w-3" /> View
                      </a>
                    )}
                  </TableCell>
                </TableRow>
              ))}
              {prototypes?.length === 0 && (
                <TableRow>
                  <TableCell
                    colSpan={5}
                    className="text-center text-sm text-muted-foreground py-8"
                  >
                    No prototypes yet.
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      )}

      <QuestionsModal
        open={modalOpen}
        onOpenChange={(next) => {
          if (!next && !createMutation.isPending) {
            setModalOpen(false)
            setQuestions(null)
          }
        }}
        title={`Generate ${format} prototype`}
        description="Answer these so Claude can tailor the output."
        questions={questions}
        loading={questionsMutation.isPending}
        submitting={createMutation.isPending}
        onSubmit={(answers) => createMutation.mutate(answers)}
        onSkip={() => createMutation.mutate([])}
      />
    </>
  )
}

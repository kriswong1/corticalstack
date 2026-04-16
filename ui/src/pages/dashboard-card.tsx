import { useMemo, useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { Link, useParams } from "react-router-dom"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { PageHeader } from "@/components/layout/page-header"
import { SkeletonPage } from "@/components/shared/skeleton-card"
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table"
import { api, getErrorMessage } from "@/lib/api"
import { toast } from "sonner"
import type { CardDetail, ItemUsageAggregate, Project, ShapeUpThread } from "@/types/api"
import { ArrowLeft, Loader2, Plus, Upload, X } from "lucide-react"

// Stage color palettes per card type (same as dashboard pipeline cards)
const stageColors: Record<string, Record<string, string>> = {
  product: {
    idea:       "oklch(0.55 0.04 250)",
    frame:      "oklch(0.60 0.14 200)",
    shape:      "oklch(0.55 0.22 275)",
    breadboard: "oklch(0.60 0.20 325)",
    pitch:      "oklch(0.62 0.19 150)",
  },
  meeting: {
    transcript: "oklch(0.65 0.15 230)",
    note:       "oklch(0.62 0.19 150)",
  },
  document: {
    input: "oklch(0.55 0.04 250)",
    note:  "oklch(0.62 0.19 150)",
  },
  prototype: {
    breadboard:  "oklch(0.60 0.20 325)",
    in_progress: "oklch(0.70 0.16 85)",
    final:       "oklch(0.62 0.19 150)",
  },
}

const fallbackColor = "oklch(0.60 0.10 250)"

function stageColorFor(cardType: string, stage: string): string {
  return stageColors[cardType]?.[stage] ?? fallbackColor
}

function formatCost(usd: number): string {
  return `$${usd.toFixed(4)}`
}

function formatTokens(n: number): string {
  return n.toLocaleString()
}

function formatDate(iso?: string): string {
  if (!iso) return "-"
  return new Date(iso).toLocaleDateString()
}

const cardTitles: Record<string, string> = {
  product: "Product",
  meeting: "Meetings",
  document: "Documents",
  prototype: "Prototypes",
}

const protoFormats = [
  "screen-flow",
  "component-spec",
  "user-journey",
  "interactive-html",
]

function isPrototypeReady(t: ShapeUpThread): boolean {
  return t.artifacts.some((a) => a.stage === "breadboard")
}

function sourcesFromThread(t: ShapeUpThread): string[] {
  return t.artifacts
    .filter((a) => a.stage !== "raw" && a.path)
    .map((a) => a.path)
}

export function DashboardCardPage() {
  const { type } = useParams<{ type: string }>()
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [showNewIdea, setShowNewIdea] = useState(false)
  const [ideaTitle, setIdeaTitle] = useState("")
  const [ideaContent, setIdeaContent] = useState("")
  const [ideaProject, setIdeaProject] = useState("")
  const [newProjectName, setNewProjectName] = useState("")

  // Meeting form state
  const [showNewTranscript, setShowNewTranscript] = useState(false)
  const [transcriptFile, setTranscriptFile] = useState<File | null>(null)

  // Document form state
  const [showNewDocument, setShowNewDocument] = useState(false)
  const [docTitle, setDocTitle] = useState("")
  const [docContent, setDocContent] = useState("")

  // Prototype form state
  const [showNewPrototype, setShowNewPrototype] = useState(false)
  const [protoThreadId, setProtoThreadId] = useState("")
  const [protoFormat, setProtoFormat] = useState("interactive-html")
  const [protoHints, setProtoHints] = useState("")

  const isProduct = type === "product"
  const isMeeting = type === "meeting"
  const isDocument = type === "document"
  const isPrototype = type === "prototype"

  const { data: projects } = useQuery<Project[]>({
    queryKey: ["projects"],
    queryFn: api.listProjects,
    enabled: isProduct,
  })

  const { data: threads } = useQuery<ShapeUpThread[]>({
    queryKey: ["shapeup-threads"],
    queryFn: api.listThreads,
    enabled: isPrototype,
  })

  const readyThreads = useMemo(
    () => (threads ?? []).filter(isPrototypeReady),
    [threads],
  )

  const selectedProtoThread = readyThreads.find((t) => t.id === protoThreadId)

  const createIdeaMutation = useMutation({
    mutationFn: async () => {
      let projectIds: string[] = []
      if (ideaProject === "__new__" && newProjectName.trim()) {
        const created = await api.createProject({ name: newProjectName.trim() })
        projectIds = [created.id]
      } else if (ideaProject && ideaProject !== "__new__") {
        projectIds = [ideaProject]
      }
      return api.createIdea({
        title: ideaTitle,
        content: ideaContent,
        project_ids: projectIds.length > 0 ? projectIds : undefined,
      })
    },
    onSuccess: () => {
      toast.success("Idea created")
      queryClient.invalidateQueries({ queryKey: ["card-detail", "product"] })
      queryClient.invalidateQueries({ queryKey: ["shapeup-threads"] })
      queryClient.invalidateQueries({ queryKey: ["projects"] })
      queryClient.invalidateQueries({ queryKey: ["dashboard"] })
      setShowNewIdea(false)
      setIdeaTitle("")
      setIdeaContent("")
      setIdeaProject("")
      setNewProjectName("")
    },
    onError: (err) => toast.error(`Create idea failed: ${getErrorMessage(err)}`),
  })

  // --- Meeting: ingest transcript file ---
  const ingestTranscriptMutation = useMutation({
    mutationFn: async () => {
      if (!transcriptFile) throw new Error("No file selected")
      const formData = new FormData()
      formData.append("file", transcriptFile)
      return api.ingestFile(formData)
    },
    onSuccess: (data) => {
      toast.success(`Transcript ingesting — job ${data.job_id}`)
      queryClient.invalidateQueries({ queryKey: ["card-detail", "meeting"] })
      queryClient.invalidateQueries({ queryKey: ["dashboard"] })
      setShowNewTranscript(false)
      setTranscriptFile(null)
    },
    onError: (err) => toast.error(`Ingest failed: ${getErrorMessage(err)}`),
  })

  // --- Document: create new document ---
  const createDocumentMutation = useMutation({
    mutationFn: () =>
      api.createDocument({ title: docTitle.trim(), content: docContent }),
    onSuccess: () => {
      toast.success("Document created")
      queryClient.invalidateQueries({ queryKey: ["card-detail", "document"] })
      queryClient.invalidateQueries({ queryKey: ["dashboard"] })
      setShowNewDocument(false)
      setDocTitle("")
      setDocContent("")
    },
    onError: (err) => toast.error(`Create document failed: ${getErrorMessage(err)}`),
  })

  // --- Prototype: create from thread ---
  const createPrototypeMutation = useMutation({
    mutationFn: () => {
      if (!selectedProtoThread) throw new Error("No thread selected")
      const sourcePaths = sourcesFromThread(selectedProtoThread)
      return api.createPrototype({
        title: selectedProtoThread.title,
        source_paths: sourcePaths,
        format: protoFormat,
        hints: protoHints || undefined,
        source_thread: selectedProtoThread.id,
      })
    },
    onSuccess: () => {
      toast.success("Prototype generated")
      queryClient.invalidateQueries({ queryKey: ["card-detail", "prototype"] })
      queryClient.invalidateQueries({ queryKey: ["dashboard"] })
      queryClient.invalidateQueries({ queryKey: ["prototypes"] })
      setShowNewPrototype(false)
      setProtoThreadId("")
      setProtoHints("")
    },
    onError: (err) => toast.error(`Prototype generation failed: ${getErrorMessage(err)}`),
  })

  const { data: detail, isLoading, error } = useQuery<CardDetail>({
    queryKey: ["card-detail", type],
    queryFn: () => api.getCardDetail(type!),
    staleTime: 60_000,
    refetchInterval: 60_000,
    enabled: !!type,
  })

  const sortedSelectedKey = useMemo(
    () => [...selected].sort().join(","),
    [selected],
  )

  const { data: usageData } = useQuery<ItemUsageAggregate | undefined>({
    queryKey: ["item-usage", type, sortedSelectedKey],
    queryFn: () =>
      selected.size > 0
        ? api.getItemUsage(type!, [...selected])
        : Promise.resolve(detail?.aggregate),
    enabled: !!detail,
    staleTime: 10_000,
  })

  const usage = usageData ?? detail?.aggregate

  if (isLoading) return <SkeletonPage />

  if (error || !detail) {
    return (
      <>
        <BackLink />
        <PageHeader
          title={cardTitles[type ?? ""] ?? type ?? "Card"}
          description="Pipeline detail"
        />
        <Card className="rounded-md border-destructive/40 bg-destructive/5">
          <CardContent className="py-6">
            <p className="text-sm text-destructive">
              Could not load card detail. The backend may still be starting
              up — refresh in a moment.
            </p>
          </CardContent>
        </Card>
      </>
    )
  }

  const items = detail.items ?? []
  const allIds = items.map((i) => i.id)
  const allSelected = items.length > 0 && selected.size === items.length

  function toggleItem(id: string) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function toggleAll() {
    if (allSelected) {
      setSelected(new Set())
    } else {
      setSelected(new Set(allIds))
    }
  }

  return (
    <>
      <BackLink />

      <PageHeader
        title={detail.label || cardTitles[type ?? ""] || (type ?? "Card")}
        description="Pipeline detail"
      >
        {isProduct && (
          <Button
            onClick={() => setShowNewIdea(!showNewIdea)}
            className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal gap-1.5"
          >
            {showNewIdea ? <X className="h-4 w-4" /> : <Plus className="h-4 w-4" />}
            {showNewIdea ? "Cancel" : "New Idea"}
          </Button>
        )}
        {isMeeting && (
          <Button
            onClick={() => setShowNewTranscript(!showNewTranscript)}
            className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal gap-1.5"
          >
            {showNewTranscript ? <X className="h-4 w-4" /> : <Upload className="h-4 w-4" />}
            {showNewTranscript ? "Cancel" : "New Transcript"}
          </Button>
        )}
        {isDocument && (
          <Button
            onClick={() => setShowNewDocument(!showNewDocument)}
            className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal gap-1.5"
          >
            {showNewDocument ? <X className="h-4 w-4" /> : <Plus className="h-4 w-4" />}
            {showNewDocument ? "Cancel" : "New Document"}
          </Button>
        )}
        {isPrototype && (
          <Button
            onClick={() => setShowNewPrototype(!showNewPrototype)}
            disabled={readyThreads.length === 0}
            title={readyThreads.length === 0 ? "Advance a thread to breadboard first" : undefined}
            className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal gap-1.5 disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {showNewPrototype ? <X className="h-4 w-4" /> : <Plus className="h-4 w-4" />}
            {showNewPrototype ? "Cancel" : "New Prototype"}
          </Button>
        )}
      </PageHeader>

      {/* New Idea form (Product only) */}
      {isProduct && showNewIdea && (
        <Card className="mb-5 rounded-[14px] border-border shadow-stripe">
          <CardContent className="pt-6">
            <form
              className="space-y-4"
              onSubmit={(e) => {
                e.preventDefault()
                if (!ideaTitle.trim() || !ideaContent.trim()) return
                createIdeaMutation.mutate()
              }}
            >
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">
                    Title
                  </Label>
                  <Input
                    value={ideaTitle}
                    onChange={(e) => setIdeaTitle(e.target.value)}
                    placeholder="Short, descriptive title for the idea"
                    className="border-border rounded-sm"
                  />
                </div>
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">
                    Project
                  </Label>
                  <Select value={ideaProject} onValueChange={setIdeaProject}>
                    <SelectTrigger className="border-border rounded-sm">
                      <SelectValue placeholder="Select a project (optional)" />
                    </SelectTrigger>
                    <SelectContent>
                      {(projects ?? []).map((p) => (
                        <SelectItem key={p.id} value={p.id}>
                          {p.name}
                        </SelectItem>
                      ))}
                      <SelectItem value="__new__">+ New project...</SelectItem>
                    </SelectContent>
                  </Select>
                  {ideaProject === "__new__" && (
                    <Input
                      value={newProjectName}
                      onChange={(e) => setNewProjectName(e.target.value)}
                      placeholder="New project name"
                      className="border-border rounded-sm mt-2"
                    />
                  )}
                </div>
              </div>
              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">
                  Description
                </Label>
                <Textarea
                  value={ideaContent}
                  onChange={(e) => setIdeaContent(e.target.value)}
                  rows={4}
                  placeholder="Describe the idea — problem, who it affects, rough direction..."
                  className="border-border rounded-sm"
                />
              </div>
              <Button
                type="submit"
                disabled={createIdeaMutation.isPending || !ideaTitle.trim() || !ideaContent.trim()}
                className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
              >
                {createIdeaMutation.isPending ? "Creating..." : "Create Idea"}
              </Button>
            </form>
          </CardContent>
        </Card>
      )}

      {/* New Transcript form (Meeting only) */}
      {isMeeting && showNewTranscript && (
        <Card className="mb-5 rounded-[14px] border-border shadow-stripe">
          <CardContent className="pt-6">
            <form
              className="space-y-4"
              onSubmit={(e) => {
                e.preventDefault()
                if (!transcriptFile) return
                ingestTranscriptMutation.mutate()
              }}
            >
              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">
                  Transcript / audio file
                </Label>
                <Input
                  type="file"
                  accept="audio/*,.vtt,.srt,.txt"
                  onChange={(e) => setTranscriptFile(e.target.files?.[0] ?? null)}
                  className="border-border rounded-sm"
                />
                {transcriptFile && (
                  <p className="text-[11px] text-muted-foreground font-mono">
                    {transcriptFile.name} ({(transcriptFile.size / 1024).toFixed(1)} KB)
                  </p>
                )}
              </div>
              <Button
                type="submit"
                disabled={ingestTranscriptMutation.isPending || !transcriptFile}
                className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
              >
                {ingestTranscriptMutation.isPending ? "Uploading..." : "Upload Transcript"}
              </Button>
            </form>
          </CardContent>
        </Card>
      )}

      {/* New Document form (Document only) */}
      {isDocument && showNewDocument && (
        <Card className="mb-5 rounded-[14px] border-border shadow-stripe">
          <CardContent className="pt-6">
            <form
              className="space-y-4"
              onSubmit={(e) => {
                e.preventDefault()
                if (!docTitle.trim() || !docContent.trim()) return
                createDocumentMutation.mutate()
              }}
            >
              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">
                  Title
                </Label>
                <Input
                  value={docTitle}
                  onChange={(e) => setDocTitle(e.target.value)}
                  placeholder="Document title"
                  className="border-border rounded-sm"
                />
              </div>
              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">
                  Content
                </Label>
                <Textarea
                  value={docContent}
                  onChange={(e) => setDocContent(e.target.value)}
                  rows={6}
                  placeholder="Paste or write the document content..."
                  className="border-border rounded-sm"
                />
              </div>
              <Button
                type="submit"
                disabled={createDocumentMutation.isPending || !docTitle.trim() || !docContent.trim()}
                className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
              >
                {createDocumentMutation.isPending ? "Creating..." : "Create Document"}
              </Button>
            </form>
          </CardContent>
        </Card>
      )}

      {/* New Prototype form (Prototype only) */}
      {isPrototype && showNewPrototype && readyThreads.length > 0 && (
        <Card className="mb-5 rounded-[14px] border-border shadow-stripe">
          <CardContent className="pt-6">
            <form
              className="space-y-4"
              onSubmit={(e) => {
                e.preventDefault()
                if (!selectedProtoThread) return
                createPrototypeMutation.mutate()
              }}
            >
              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">
                  Source thread (must have a breadboard)
                </Label>
                <Select value={protoThreadId} onValueChange={setProtoThreadId}>
                  <SelectTrigger className="border-border rounded-sm">
                    <SelectValue placeholder="Pick a thread..." />
                  </SelectTrigger>
                  <SelectContent>
                    {readyThreads.map((t) => (
                      <SelectItem key={t.id} value={t.id}>
                        {t.title} &middot; {t.current_stage}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {selectedProtoThread && (
                  <p className="text-[11px] text-muted-foreground font-mono">
                    {sourcesFromThread(selectedProtoThread).length} source file
                    {sourcesFromThread(selectedProtoThread).length === 1 ? "" : "s"} from this thread
                  </p>
                )}
              </div>
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">
                    Format
                  </Label>
                  <Select value={protoFormat} onValueChange={setProtoFormat}>
                    <SelectTrigger className="border-border rounded-sm">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {protoFormats.map((f) => (
                        <SelectItem key={f} value={f}>
                          {f}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">
                    Hints (optional)
                  </Label>
                  <Textarea
                    value={protoHints}
                    onChange={(e) => setProtoHints(e.target.value)}
                    rows={2}
                    placeholder="Any guidance for the prototype generation..."
                    className="border-border rounded-sm"
                  />
                </div>
              </div>
              <Button
                type="submit"
                disabled={createPrototypeMutation.isPending || !selectedProtoThread}
                className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal gap-1.5"
              >
                {createPrototypeMutation.isPending ? (
                  <>
                    <Loader2 className="h-4 w-4 animate-spin" />
                    Generating...
                  </>
                ) : (
                  "Generate Prototype"
                )}
              </Button>
            </form>
          </CardContent>
        </Card>
      )}

      {/* Stage distribution row */}
      <div className="flex flex-wrap gap-3 mb-5">
        {detail.stage_counts.map((sc) => (
          <div
            key={sc.stage}
            className="flex items-center gap-2 rounded-lg border border-border bg-card px-4 py-3"
          >
            <span
              className="inline-block h-2.5 w-2.5 rounded-full flex-shrink-0"
              style={{ background: stageColorFor(type ?? "", sc.stage) }}
            />
            <span className="text-[13px] font-medium text-foreground capitalize">
              {sc.stage}
            </span>
            <span className="text-[18px] font-bold tabular-nums text-foreground">
              {sc.count}
            </span>
          </div>
        ))}
      </div>

      {/* Usage card */}
      {usage && <UsageCard usage={usage} hasSelection={selected.size > 0} />}

      {/* Items table */}
      <Card className="rounded-[14px] border-border shadow-stripe mt-5">
        <CardHeader className="pb-3">
          <CardTitle className="text-[15px] font-semibold tracking-tight text-foreground">
            Items
            {selected.size > 0 && (
              <span className="ml-2 text-xs font-normal text-muted-foreground">
                {selected.size} selected
              </span>
            )}
          </CardTitle>
        </CardHeader>
        <CardContent className="p-0">
          {items.length === 0 ? (
            <div className="py-10 text-center text-sm text-muted-foreground">
              No items in this pipeline.
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-10 pl-4">
                    <input
                      type="checkbox"
                      checked={allSelected}
                      onChange={toggleAll}
                      className="h-4 w-4 rounded border-border"
                      style={{ accentColor: stageColorFor(type ?? "", detail.stage_counts[0]?.stage ?? "") }}
                      aria-label="Select all items"
                    />
                  </TableHead>
                  <TableHead>Title</TableHead>
                  <TableHead>Stage</TableHead>
                  <TableHead>Updated</TableHead>
                  <TableHead className="w-16" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((item) => (
                  <TableRow key={item.id}>
                    <TableCell className="pl-4">
                      <input
                        type="checkbox"
                        checked={selected.has(item.id)}
                        onChange={() => toggleItem(item.id)}
                        className="h-4 w-4 rounded border-border"
                        style={{ accentColor: stageColorFor(type ?? "", item.stage) }}
                        aria-label={`Select ${item.title}`}
                      />
                    </TableCell>
                    <TableCell className="font-medium text-foreground max-w-[300px] truncate">
                      {item.title}
                    </TableCell>
                    <TableCell>
                      <span className="inline-flex items-center gap-1.5 text-[12px]">
                        <span
                          className="inline-block h-1.5 w-1.5 rounded-full flex-shrink-0"
                          style={{ background: stageColorFor(type ?? "", item.stage) }}
                        />
                        <span className="capitalize text-muted-foreground">
                          {item.stage}
                        </span>
                      </span>
                    </TableCell>
                    <TableCell className="text-muted-foreground text-[13px] tabular-nums">
                      {formatDate(item.updated)}
                    </TableCell>
                    <TableCell>
                      <Button asChild variant="ghost" size="sm" className="h-7 px-2 text-xs">
                        <Link to={`/dashboard/${type}/${item.id}`}>View</Link>
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </>
  )
}

// --- Usage aggregate card ---

function UsageCard({
  usage,
  hasSelection,
}: {
  usage: ItemUsageAggregate
  hasSelection: boolean
}) {
  const stats = [
    { label: "Calls", value: formatTokens(usage.calls) },
    { label: "Cost", value: formatCost(usage.cost_usd) },
    { label: "Input tokens", value: formatTokens(usage.input_tokens) },
    { label: "Output tokens", value: formatTokens(usage.output_tokens) },
  ]

  return (
    <Card className="rounded-[14px] border-border shadow-stripe">
      <CardHeader className="pb-3">
        <CardTitle className="text-[15px] font-semibold tracking-tight text-foreground">
          Usage
          {hasSelection && (
            <span className="ml-2 text-xs font-normal text-muted-foreground">
              (filtered by selection)
            </span>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
          {stats.map((s) => (
            <div key={s.label}>
              <p className="text-[11px] text-muted-foreground mb-0.5">
                {s.label}
              </p>
              <p className="text-[18px] font-bold tabular-nums tracking-tight text-foreground">
                {s.value}
              </p>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}

// --- Back link ---

function BackLink() {
  return (
    <Link
      to="/dashboard"
      className="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors mb-4"
    >
      <ArrowLeft className="h-4 w-4" />
      Dashboard
    </Link>
  )
}

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
import { Breadcrumbs } from "@/components/layout/breadcrumbs"
import { ProductSubnav } from "@/components/product-subnav"
import { SkeletonPage } from "@/components/shared/skeleton-card"
import { PipelineStageCards } from "@/components/shared/pipeline-stage-cards"
import {
  PipelineItemsTable,
  type PipelineExtraColumn,
  type PipelineItemsTableItem,
} from "@/components/shared/pipeline-items-table"
import { api, getErrorMessage } from "@/lib/api"
import { toast } from "sonner"
import type { CardDetail, ItemUsageAggregate, Project, Prototype, ShapeUpThread } from "@/types/api"
import { Loader2, Plus, Upload, X } from "lucide-react"

function formatCost(usd: number): string {
  return `$${usd.toFixed(4)}`
}

function formatTokens(n: number): string {
  return n.toLocaleString()
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

interface DashboardCardPageProps {
  /** Type is passed explicitly from the router. Falls back to the
      legacy `:type` URL param when absent (e.g. during migration). */
  type?: string
}

export function DashboardCardPage({ type: typeProp }: DashboardCardPageProps = {}) {
  const params = useParams<{ type: string }>()
  const type = typeProp ?? params.type
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [stageFilter, setStageFilter] = useState<string | null>(null)
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

  // Prototype list is the source of truth for the Source Thread
  // column on the prototypes dashboard — CardItem from the backend
  // only carries the common fields (id, title, stage, updated), so
  // we map prototype.source_thread → thread.title client-side.
  const { data: protoList } = useQuery<Prototype[]>({
    queryKey: ["prototypes"],
    queryFn: api.listPrototypes,
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
      if (!selectedProtoThread) throw new Error("No spec selected")
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
        <Breadcrumbs
          items={[
            { label: "Dashboard", to: "/dashboard" },
            { label: cardTitles[type ?? ""] ?? type ?? "Card" },
          ]}
        />
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
  // Stage-filtered view of items — the stage cards at the top drive
  // this, so clicking a card narrows the table to that stage. Null
  // means "all items, no filter".
  const visibleItems = stageFilter
    ? items.filter((i) => i.stage === stageFilter)
    : items
  const allIds = visibleItems.map((i) => i.id)
  const allSelected =
    visibleItems.length > 0 && allIds.every((id) => selected.has(id))

  // Per-type extra columns. Prototypes share the Source Thread
  // column with PRDs — it's the most useful piece of cross-surface
  // context and avoids forcing users to click through to find it.
  const extraColumns: PipelineExtraColumn<PipelineItemsTableItem>[] =
    isPrototype
      ? [
          {
            header: "Source Spec",
            cell: (item) => {
              const proto = (protoList ?? []).find((p) => p.id === item.id)
              const thread = proto?.source_thread
                ? (threads ?? []).find((t) => t.id === proto.source_thread)
                : undefined
              if (!thread) {
                return <span className="text-xs text-muted-foreground">—</span>
              }
              return (
                <Link
                  to={`/product/${thread.id}`}
                  className="text-xs text-primary hover:underline"
                >
                  {thread.title}
                </Link>
              )
            },
          },
        ]
      : []

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
      setSelected((prev) => {
        const next = new Set(prev)
        for (const id of allIds) next.delete(id)
        return next
      })
    } else {
      setSelected((prev) => {
        const next = new Set(prev)
        for (const id of allIds) next.add(id)
        return next
      })
    }
  }

  return (
    <>
      <Breadcrumbs
        items={[
          { label: "Dashboard", to: "/dashboard" },
          { label: detail.label || cardTitles[type ?? ""] || (type ?? "Card") },
        ]}
      />

      {/* Product surfaces (Threads list and Prototypes list) share a
          subnav so the four cross-thread views feel like one section. */}
      {(isProduct || isPrototype) && <ProductSubnav />}

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
            title={readyThreads.length === 0 ? "Advance a spec to breadboard first" : undefined}
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
                  Source spec (must have a breadboard)
                </Label>
                <Select value={protoThreadId} onValueChange={setProtoThreadId}>
                  <SelectTrigger className="border-border rounded-sm">
                    <SelectValue placeholder="Pick a spec..." />
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
                    {sourcesFromThread(selectedProtoThread).length === 1 ? "" : "s"} from this spec
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

      {/* Stage distribution — reusable pipeline-stage cards. Clicking
          a card filters the items table below; hovering reveals the
          titles of items currently in that stage. */}
      <PipelineStageCards
        type={type ?? ""}
        items={items}
        selectedStage={stageFilter}
        onSelectStage={(s) => {
          setStageFilter(s)
          setSelected(new Set())
        }}
      />

      {/* Usage card */}
      {usage && <UsageCard usage={usage} hasSelection={selected.size > 0} />}

      {/* Items table */}
      <Card className="rounded-[14px] border-border shadow-stripe mt-5">
        <CardHeader className="pb-3 flex flex-row items-center justify-between gap-3 space-y-0">
          <CardTitle className="text-[15px] font-semibold tracking-tight text-foreground">
            Items
            {stageFilter && (
              <span className="ml-2 text-xs font-normal text-muted-foreground">
                filtered by {stageFilter}
              </span>
            )}
            {selected.size > 0 && (
              <span className="ml-2 text-xs font-normal text-muted-foreground">
                · {selected.size} selected
              </span>
            )}
          </CardTitle>
          {stageFilter && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setStageFilter(null)}
              className="h-7 px-2 text-xs gap-1"
            >
              <X className="h-3 w-3" /> Clear filter
            </Button>
          )}
        </CardHeader>
        <CardContent className="p-0">
          <PipelineItemsTable
            type={type ?? ""}
            items={visibleItems.map((i) => ({
              id: i.id,
              title: i.title,
              stage: i.stage,
              updated: i.updated,
            }))}
            selected={selected}
            onToggleItem={toggleItem}
            onToggleAll={toggleAll}
            allSelected={allSelected}
            extraColumns={extraColumns}
            emptyMessage={
              items.length === 0
                ? "No items in this pipeline."
                : `No items in the ${stageFilter} stage.`
            }
          />
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


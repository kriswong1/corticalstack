import { useState } from "react"
import { useParams, useNavigate, Link } from "react-router-dom"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { PageHeader } from "@/components/layout/page-header"
import { Breadcrumbs } from "@/components/layout/breadcrumbs"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"
import { api, getErrorMessage } from "@/lib/api"
import type { ProjectStatus } from "@/types/api"
import { Pencil, Trash2 } from "lucide-react"

export function ProjectDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [editOpen, setEditOpen] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)

  const { data: content, isLoading, error } = useQuery({
    queryKey: ["project-content", id],
    queryFn: () => api.getProjectContent(id ?? ""),
    enabled: !!id,
  })

  const updateMutation = useMutation({
    mutationFn: (body: {
      name?: string
      description?: string
      status?: ProjectStatus
    }) => api.updateProject(id ?? "", body),
    onSuccess: (updated) => {
      queryClient.invalidateQueries({ queryKey: ["projects"] })
      queryClient.invalidateQueries({ queryKey: ["project-content"] })
      toast.success("Project updated")
      setEditOpen(false)
      // If slug changed, the URL we're on still resolves via UUID — no
      // navigation required. Keeping URLs UUID-keyed is the whole point.
      if (updated.uuid !== id) navigate(`/projects/${updated.uuid}`, { replace: true })
    },
    onError: (err) => toast.error(getErrorMessage(err)),
  })

  const deleteMutation = useMutation({
    mutationFn: () => api.deleteProject(id ?? ""),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["projects"] })
      toast.success("Project deleted (moved to .trash)")
      navigate("/projects")
    },
    onError: (err) => toast.error(getErrorMessage(err)),
  })

  if (isLoading) {
    return <p className="text-muted-foreground">Loading...</p>
  }
  if (error || !content) {
    return (
      <div className="space-y-2">
        <Breadcrumbs items={[
          { label: "Dashboard", to: "/dashboard" },
          { label: "Projects", to: "/projects" },
          { label: id ?? "?" },
        ]} />
        <p className="text-sm text-destructive">
          {error ? getErrorMessage(error) : "Project not found"}
        </p>
      </div>
    )
  }

  const p = content.project
  const counts = content.counts

  return (
    <>
      <Breadcrumbs items={[
        { label: "Dashboard", to: "/dashboard" },
        { label: "Projects", to: "/projects" },
        { label: p.name },
      ]} />
      <PageHeader title={p.name} description={p.description ?? p.slug}>
        <Button
          variant="outline"
          onClick={() => setEditOpen(true)}
          className="border-border rounded-sm font-normal gap-1.5"
        >
          <Pencil className="h-3.5 w-3.5" /> Edit
        </Button>
        <Button
          variant="outline"
          onClick={() => setConfirmDelete(true)}
          className="border-border rounded-sm font-normal gap-1.5 text-destructive hover:text-destructive"
        >
          <Trash2 className="h-3.5 w-3.5" /> Delete
        </Button>
      </PageHeader>

      {content.warnings && content.warnings.length > 0 && (
        <div className="mb-4 rounded-sm border border-amber-500/40 bg-amber-50/10 p-3 text-xs text-amber-300">
          Some content failed to load: {content.warnings.join("; ")}
        </div>
      )}

      <Tabs defaultValue="overview" className="space-y-4">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="pipeline">
            Pipeline ({counts.prds + counts.prototypes + counts.usecases + counts.threads})
          </TabsTrigger>
          <TabsTrigger value="tasks">Tasks ({counts.actions})</TabsTrigger>
          <TabsTrigger value="notes">
            Notes ({counts.documents + counts.meetings})
          </TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-4">
          <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
            <CountCard label="Tasks" value={counts.actions} />
            <CountCard label="PRDs" value={counts.prds} />
            <CountCard label="Prototypes" value={counts.prototypes} />
            <CountCard label="Use Cases" value={counts.usecases} />
            <CountCard label="Threads" value={counts.threads} />
            <CountCard label="Documents" value={counts.documents} />
            <CountCard label="Meetings" value={counts.meetings} />
          </div>
          <Card className="rounded-md border-border shadow-stripe-elevated">
            <CardHeader>
              <CardTitle className="text-base font-light">Metadata</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2 text-sm font-light text-muted-foreground">
              <Row k="UUID" v={<span className="font-mono">{p.uuid}</span>} />
              <Row k="Slug" v={<span className="font-mono">{p.slug}</span>} />
              <Row k="Status" v={<Badge>{p.status}</Badge>} />
              <Row k="Created" v={new Date(p.created).toLocaleString()} />
              {p.tags && p.tags.length > 0 && (
                <Row
                  k="Tags"
                  v={
                    <div className="flex flex-wrap gap-1">
                      {p.tags.map((t) => (
                        <Badge key={t} variant="outline" className="text-[10px]">
                          {t}
                        </Badge>
                      ))}
                    </div>
                  }
                />
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="pipeline" className="space-y-4">
          {counts.prds + counts.prototypes + counts.usecases + counts.threads === 0 ? (
            <EmptyHint>No pipeline artifacts yet. Pitches → PRDs → Prototypes → Use Cases all live here when associated with this project.</EmptyHint>
          ) : (
            <>
              {content.threads.length > 0 && (
                <Section title={`Threads (${content.threads.length})`}>
                  {content.threads.map((t) => (
                    <RowItem
                      key={t.id}
                      to={`/product/${t.id}`}
                      title={t.title}
                      meta={`stage: ${t.current_stage}`}
                    />
                  ))}
                </Section>
              )}
              {content.prds.length > 0 && (
                <Section title={`PRDs (${content.prds.length})`}>
                  {content.prds.map((x) => (
                    <RowItem
                      key={x.id}
                      to={`/prds/${x.id}`}
                      title={x.title}
                      meta={`v${x.version} · ${x.status}`}
                    />
                  ))}
                </Section>
              )}
              {content.prototypes.length > 0 && (
                <Section title={`Prototypes (${content.prototypes.length})`}>
                  {content.prototypes.map((x) => (
                    <RowItem
                      key={x.id}
                      to={`/prototypes/${x.id}`}
                      title={x.title}
                      meta={`${x.format} · ${x.stage}`}
                    />
                  ))}
                </Section>
              )}
              {content.usecases.length > 0 && (
                <Section title={`Use Cases (${content.usecases.length})`}>
                  {content.usecases.map((x) => (
                    <RowItem
                      key={x.id}
                      to={`/usecases`}
                      title={x.title}
                      meta={(x.actors ?? []).join(", ")}
                    />
                  ))}
                </Section>
              )}
            </>
          )}
        </TabsContent>

        <TabsContent value="tasks" className="space-y-4">
          {content.actions.length === 0 ? (
            <EmptyHint>No actions tagged with this project.</EmptyHint>
          ) : (
            <Section title={`Actions (${content.actions.length})`}>
              {content.actions.map((a) => (
                <RowItem
                  key={a.id}
                  to={`/actions?status=${a.status}`}
                  title={a.title || a.description}
                  meta={`${a.status}${a.priority ? ` · ${a.priority}` : ""}${a.owner ? ` · ${a.owner}` : ""}`}
                />
              ))}
            </Section>
          )}
        </TabsContent>

        <TabsContent value="notes" className="space-y-4">
          {counts.documents + counts.meetings === 0 ? (
            <EmptyHint>No documents or meetings tagged with this project.</EmptyHint>
          ) : (
            <>
              {content.documents.length > 0 && (
                <Section title={`Documents (${content.documents.length})`}>
                  {content.documents.map((d) => (
                    <RowItem
                      key={d.id}
                      to={`/documents/${d.id}`}
                      title={d.title}
                      meta={d.stage}
                    />
                  ))}
                </Section>
              )}
              {content.meetings.length > 0 && (
                <Section title={`Meetings (${content.meetings.length})`}>
                  {content.meetings.map((m) => (
                    <RowItem
                      key={m.id}
                      to={`/meetings/${m.id}`}
                      title={m.title}
                      meta={m.stage}
                    />
                  ))}
                </Section>
              )}
            </>
          )}
        </TabsContent>
      </Tabs>

      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent className="shadow-stripe-deep rounded-md">
          <DialogHeader>
            <DialogTitle className="text-[22px] font-light tracking-[-0.22px]">
              Edit Project
            </DialogTitle>
          </DialogHeader>
          <EditForm
            initial={{ name: p.name, description: p.description ?? "", status: p.status }}
            onSubmit={(patch) => updateMutation.mutate(patch)}
            submitting={updateMutation.isPending}
          />
        </DialogContent>
      </Dialog>

      <Dialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <DialogContent className="shadow-stripe-deep rounded-md">
          <DialogHeader>
            <DialogTitle>Delete project?</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            The project folder moves to <code>vault/.trash/projects/</code>.
            References in other notes are left in place; restore by moving the
            folder back. UUID stays stable across the round trip.
          </p>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDelete(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={deleteMutation.isPending}
              onClick={() => deleteMutation.mutate()}
            >
              {deleteMutation.isPending ? "Deleting..." : "Delete"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function CountCard({ label, value }: { label: string; value: number }) {
  return (
    <Card className="rounded-md border-border shadow-stripe-elevated">
      <CardContent className="py-4">
        <div className="text-2xl font-light">{value}</div>
        <div className="text-xs text-muted-foreground">{label}</div>
      </CardContent>
    </Card>
  )
}

function Row({ k, v }: { k: string; v: React.ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4">
      <span className="text-xs uppercase tracking-wide text-muted-foreground">{k}</span>
      <span>{v}</span>
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <Card className="rounded-md border-border shadow-stripe-elevated">
      <CardHeader className="pb-2">
        <CardTitle className="text-base font-light">{title}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-1">{children}</CardContent>
    </Card>
  )
}

function RowItem({ to, title, meta }: { to: string; title: string; meta?: string }) {
  return (
    <Link
      to={to}
      className="flex items-baseline justify-between gap-4 py-1.5 border-b border-border/40 last:border-0 hover:bg-muted/20 -mx-2 px-2 rounded-sm transition-colors"
    >
      <span className="text-sm truncate">{title || "(untitled)"}</span>
      {meta && (
        <span className="text-xs text-muted-foreground font-mono whitespace-nowrap">
          {meta}
        </span>
      )}
    </Link>
  )
}

function EmptyHint({ children }: { children: React.ReactNode }) {
  return (
    <p className="text-sm text-muted-foreground italic px-1">{children}</p>
  )
}

function EditForm({
  initial,
  onSubmit,
  submitting,
}: {
  initial: { name: string; description: string; status: ProjectStatus }
  onSubmit: (patch: { name?: string; description?: string; status?: ProjectStatus }) => void
  submitting: boolean
}) {
  const [name, setName] = useState(initial.name)
  const [description, setDescription] = useState(initial.description)
  const [status, setStatus] = useState<ProjectStatus>(initial.status)

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        const patch: { name?: string; description?: string; status?: ProjectStatus } = {}
        if (name !== initial.name) patch.name = name
        if (description !== initial.description) patch.description = description
        if (status !== initial.status) patch.status = status
        onSubmit(patch)
      }}
    >
      <div className="space-y-2">
        <Label className="text-sm font-normal">Name</Label>
        <Input value={name} onChange={(e) => setName(e.target.value)} className="border-border rounded-sm" />
      </div>
      <div className="space-y-2">
        <Label className="text-sm font-normal">Description</Label>
        <Input value={description} onChange={(e) => setDescription(e.target.value)} className="border-border rounded-sm" />
      </div>
      <div className="space-y-2">
        <Label className="text-sm font-normal">Status</Label>
        <Select value={status} onValueChange={(v) => setStatus(v as ProjectStatus)}>
          <SelectTrigger className="border-border rounded-sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="active">active</SelectItem>
            <SelectItem value="paused">paused</SelectItem>
            <SelectItem value="archived">archived</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <Button
        type="submit"
        disabled={submitting || !name.trim()}
        className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
      >
        {submitting ? "Saving..." : "Save"}
      </Button>
    </form>
  )
}

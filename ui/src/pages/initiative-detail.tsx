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
import type { InitiativeStatus } from "@/types/api"
import { Pencil, Trash2 } from "lucide-react"

export function InitiativeDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [editOpen, setEditOpen] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)

  const { data: content, isLoading, error } = useQuery({
    queryKey: ["initiative-content", id],
    queryFn: () => api.getInitiativeContent(id ?? ""),
    enabled: !!id,
  })

  const updateMutation = useMutation({
    mutationFn: (body: {
      name?: string
      description?: string
      status?: InitiativeStatus
      owner?: string
    }) => api.updateInitiative(id ?? "", body),
    onSuccess: (updated) => {
      queryClient.invalidateQueries({ queryKey: ["initiatives"] })
      queryClient.invalidateQueries({ queryKey: ["initiative-content"] })
      toast.success("Initiative updated")
      setEditOpen(false)
      if (updated.uuid !== id) navigate(`/initiatives/${updated.uuid}`, { replace: true })
    },
    onError: (err) => toast.error(getErrorMessage(err)),
  })

  const deleteMutation = useMutation({
    mutationFn: () => api.deleteInitiative(id ?? ""),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["initiatives"] })
      toast.success("Initiative deleted (moved to .trash)")
      navigate("/initiatives")
    },
    onError: (err) => toast.error(getErrorMessage(err)),
  })

  if (isLoading) return <p className="text-muted-foreground">Loading...</p>
  if (error || !content) {
    return (
      <div className="space-y-2">
        <Breadcrumbs items={[
          { label: "Dashboard", to: "/dashboard" },
          { label: "Initiatives", to: "/initiatives" },
          { label: id ?? "?" },
        ]} />
        <p className="text-sm text-destructive">
          {error ? getErrorMessage(error) : "Initiative not found"}
        </p>
      </div>
    )
  }

  const i = content.initiative

  return (
    <>
      <Breadcrumbs items={[
        { label: "Dashboard", to: "/dashboard" },
        { label: "Initiatives", to: "/initiatives" },
        { label: i.name },
      ]} />
      <PageHeader title={i.name} description={i.description ?? i.slug}>
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

      <div className="space-y-4">
        <Card className="rounded-md border-border shadow-stripe-elevated">
          <CardHeader>
            <CardTitle className="text-base font-light">Linked Projects ({content.counts.projects})</CardTitle>
          </CardHeader>
          <CardContent className="space-y-1">
            {content.projects.length === 0 ? (
              <p className="text-sm text-muted-foreground italic">
                No projects linked yet. Edit a project and set its Initiative to attach.
              </p>
            ) : (
              content.projects.map((p) => (
                <Link
                  key={p.uuid}
                  to={`/projects/${p.uuid}`}
                  className="flex items-baseline justify-between gap-4 py-1.5 border-b border-border/40 last:border-0 hover:bg-muted/20 -mx-2 px-2 rounded-sm transition-colors"
                >
                  <span className="text-sm truncate">{p.name}</span>
                  <span className="text-xs text-muted-foreground font-mono whitespace-nowrap">
                    {p.status}
                  </span>
                </Link>
              ))
            )}
          </CardContent>
        </Card>

        <Card className="rounded-md border-border shadow-stripe-elevated">
          <CardHeader>
            <CardTitle className="text-base font-light">Metadata</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm font-light text-muted-foreground">
            <Row k="UUID" v={<span className="font-mono">{i.uuid}</span>} />
            <Row k="Slug" v={<span className="font-mono">{i.slug}</span>} />
            <Row k="Status" v={<Badge>{i.status}</Badge>} />
            {i.owner && <Row k="Owner" v={i.owner} />}
            {i.target_date && <Row k="Target" v={new Date(i.target_date).toLocaleDateString()} />}
            {i.linear_id && <Row k="Linear" v={<span className="font-mono">{i.linear_id}</span>} />}
            <Row k="Created" v={new Date(i.created).toLocaleString()} />
          </CardContent>
        </Card>
      </div>

      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent className="shadow-stripe-deep rounded-md">
          <DialogHeader>
            <DialogTitle className="text-[22px] font-light tracking-[-0.22px]">
              Edit Initiative
            </DialogTitle>
          </DialogHeader>
          <EditForm
            initial={{
              name: i.name,
              description: i.description ?? "",
              status: i.status,
              owner: i.owner ?? "",
            }}
            onSubmit={(patch) => updateMutation.mutate(patch)}
            submitting={updateMutation.isPending}
          />
        </DialogContent>
      </Dialog>

      <Dialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <DialogContent className="shadow-stripe-deep rounded-md">
          <DialogHeader>
            <DialogTitle>Delete initiative?</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            The initiative folder moves to <code>vault/.trash/initiatives/</code>.
            Projects that referenced it keep their <code>initiative_id</code> until next save;
            the canonicalizer drops dangling references on next write.
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

function Row({ k, v }: { k: string; v: React.ReactNode }) {
  return (
    <div className="flex items-baseline gap-3">
      <span className="text-xs uppercase tracking-wide text-muted-foreground/60 w-20">{k}</span>
      <span className="text-sm">{v}</span>
    </div>
  )
}

function EditForm({
  initial,
  onSubmit,
  submitting,
}: {
  initial: { name: string; description: string; status: InitiativeStatus; owner: string }
  onSubmit: (patch: { name?: string; description?: string; status?: InitiativeStatus; owner?: string }) => void
  submitting: boolean
}) {
  const [name, setName] = useState(initial.name)
  const [description, setDescription] = useState(initial.description)
  const [status, setStatus] = useState<InitiativeStatus>(initial.status)
  const [owner, setOwner] = useState(initial.owner)

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        const patch: { name?: string; description?: string; status?: InitiativeStatus; owner?: string } = {}
        if (name !== initial.name) patch.name = name
        if (description !== initial.description) patch.description = description
        if (status !== initial.status) patch.status = status
        if (owner !== initial.owner) patch.owner = owner
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
        <Label className="text-sm font-normal">Owner</Label>
        <Input value={owner} onChange={(e) => setOwner(e.target.value)} className="border-border rounded-sm" />
      </div>
      <div className="space-y-2">
        <Label className="text-sm font-normal">Status</Label>
        <Select value={status} onValueChange={(v) => setStatus(v as InitiativeStatus)}>
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

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
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog"
import { api, getErrorMessage } from "@/lib/api"
import { Pencil, Trash2 } from "lucide-react"

export function WorkspaceDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [editOpen, setEditOpen] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)

  const { data: content, isLoading, error } = useQuery({
    queryKey: ["workspace-content", id],
    queryFn: () => api.getWorkspaceContent(id ?? ""),
    enabled: !!id,
  })

  const updateMutation = useMutation({
    mutationFn: (body: {
      name?: string
      description?: string
      linear_workspace_id?: string
      linear_team_key?: string
      linear_api_key_env?: string
    }) => api.updateWorkspace(id ?? "", body),
    onSuccess: (updated) => {
      queryClient.invalidateQueries({ queryKey: ["workspaces"] })
      queryClient.invalidateQueries({ queryKey: ["workspace-content"] })
      toast.success("Workspace updated")
      setEditOpen(false)
      if (updated.uuid !== id) navigate(`/workspaces/${updated.uuid}`, { replace: true })
    },
    onError: (err) => toast.error(getErrorMessage(err)),
  })

  const deleteMutation = useMutation({
    mutationFn: () => api.deleteWorkspace(id ?? ""),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["workspaces"] })
      toast.success("Workspace deleted (moved to .trash)")
      navigate("/workspaces")
    },
    onError: (err) => toast.error(getErrorMessage(err)),
  })

  if (isLoading) return <p className="text-muted-foreground">Loading...</p>
  if (error || !content) {
    return (
      <div className="space-y-2">
        <Breadcrumbs items={[
          { label: "Dashboard", to: "/dashboard" },
          { label: "Workspaces", to: "/workspaces" },
          { label: id ?? "?" },
        ]} />
        <p className="text-sm text-destructive">
          {error ? getErrorMessage(error) : "Workspace not found"}
        </p>
      </div>
    )
  }

  const w = content.workspace

  return (
    <>
      <Breadcrumbs items={[
        { label: "Dashboard", to: "/dashboard" },
        { label: "Workspaces", to: "/workspaces" },
        { label: w.name },
      ]} />
      <PageHeader title={w.name} description={w.description ?? w.slug}>
        <Button variant="outline" onClick={() => setEditOpen(true)} className="border-border rounded-sm font-normal gap-1.5">
          <Pencil className="h-3.5 w-3.5" /> Edit
        </Button>
        <Button variant="outline" onClick={() => setConfirmDelete(true)} className="border-border rounded-sm font-normal gap-1.5 text-destructive hover:text-destructive">
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
                No projects linked yet. Edit a project and set its Workspace to attach.
              </p>
            ) : (
              content.projects.map((p) => (
                <Link
                  key={p.uuid}
                  to={`/projects/${p.uuid}`}
                  className="flex items-baseline justify-between gap-4 py-1.5 border-b border-border/40 last:border-0 hover:bg-muted/20 -mx-2 px-2 rounded-sm transition-colors"
                >
                  <span className="text-sm truncate">{p.name}</span>
                  <span className="text-xs text-muted-foreground font-mono">{p.status}</span>
                </Link>
              ))
            )}
          </CardContent>
        </Card>

        <Card className="rounded-md border-border shadow-stripe-elevated">
          <CardHeader>
            <CardTitle className="text-base font-light">Linear Routing</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm font-light text-muted-foreground">
            <Row k="UUID" v={<span className="font-mono">{w.uuid}</span>} />
            <Row k="Slug" v={<span className="font-mono">{w.slug}</span>} />
            {w.linear_workspace_id && <Row k="Linear ID" v={<span className="font-mono">{w.linear_workspace_id}</span>} />}
            {w.linear_team_key && <Row k="Default Team" v={<span className="font-mono">{w.linear_team_key}</span>} />}
            {w.linear_api_key_env && (
              <Row k="API Key Env" v={<span className="font-mono">{w.linear_api_key_env}</span>} />
            )}
            <Row k="Created" v={new Date(w.created).toLocaleString()} />
          </CardContent>
        </Card>
      </div>

      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent className="shadow-stripe-deep rounded-md">
          <DialogHeader>
            <DialogTitle className="text-[22px] font-light tracking-[-0.22px]">
              Edit Workspace
            </DialogTitle>
          </DialogHeader>
          <EditForm
            initial={{
              name: w.name,
              description: w.description ?? "",
              linear_workspace_id: w.linear_workspace_id ?? "",
              linear_team_key: w.linear_team_key ?? "",
              linear_api_key_env: w.linear_api_key_env ?? "",
            }}
            onSubmit={(patch) => updateMutation.mutate(patch)}
            submitting={updateMutation.isPending}
          />
        </DialogContent>
      </Dialog>

      <Dialog open={confirmDelete} onOpenChange={setConfirmDelete}>
        <DialogContent className="shadow-stripe-deep rounded-md">
          <DialogHeader>
            <DialogTitle>Delete workspace?</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            The workspace folder moves to <code>vault/.trash/workspaces/</code>.
            Projects that referenced it will fall through to <code>LINEAR_TEAM_KEY</code> /
            <code>LINEAR_API_KEY</code> on next sync.
          </p>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmDelete(false)}>
              Cancel
            </Button>
            <Button variant="destructive" disabled={deleteMutation.isPending} onClick={() => deleteMutation.mutate()}>
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
      <span className="text-xs uppercase tracking-wide text-muted-foreground/60 w-24">{k}</span>
      <span className="text-sm">{v}</span>
    </div>
  )
}

function EditForm({
  initial,
  onSubmit,
  submitting,
}: {
  initial: {
    name: string
    description: string
    linear_workspace_id: string
    linear_team_key: string
    linear_api_key_env: string
  }
  onSubmit: (patch: {
    name?: string
    description?: string
    linear_workspace_id?: string
    linear_team_key?: string
    linear_api_key_env?: string
  }) => void
  submitting: boolean
}) {
  const [name, setName] = useState(initial.name)
  const [description, setDescription] = useState(initial.description)
  const [linearWorkspaceID, setLinearWorkspaceID] = useState(initial.linear_workspace_id)
  const [linearTeamKey, setLinearTeamKey] = useState(initial.linear_team_key)
  const [linearAPIKeyEnv, setLinearAPIKeyEnv] = useState(initial.linear_api_key_env)

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        const patch: {
          name?: string
          description?: string
          linear_workspace_id?: string
          linear_team_key?: string
          linear_api_key_env?: string
        } = {}
        if (name !== initial.name) patch.name = name
        if (description !== initial.description) patch.description = description
        if (linearWorkspaceID !== initial.linear_workspace_id) patch.linear_workspace_id = linearWorkspaceID
        if (linearTeamKey !== initial.linear_team_key) patch.linear_team_key = linearTeamKey
        if (linearAPIKeyEnv !== initial.linear_api_key_env) patch.linear_api_key_env = linearAPIKeyEnv
        onSubmit(patch)
      }}
    >
      <FieldEdit label="Name" value={name} onChange={setName} />
      <FieldEdit label="Description" value={description} onChange={setDescription} />
      <FieldEdit label="Linear Workspace ID" value={linearWorkspaceID} onChange={setLinearWorkspaceID} mono />
      <FieldEdit label="Default Team Key" value={linearTeamKey} onChange={setLinearTeamKey} mono />
      <FieldEdit label="API Key Env Var" value={linearAPIKeyEnv} onChange={setLinearAPIKeyEnv} mono />
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

function FieldEdit({
  label,
  value,
  onChange,
  mono,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  mono?: boolean
}) {
  return (
    <div className="space-y-2">
      <Label className="text-sm font-normal">{label}</Label>
      <Input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={`border-border rounded-sm ${mono ? "font-mono text-xs" : ""}`}
      />
    </div>
  )
}

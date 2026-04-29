import { useState } from "react"
import { Link } from "react-router-dom"
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
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog"
import { api, getErrorMessage } from "@/lib/api"
import { Plus } from "lucide-react"

export function WorkspacesPage() {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [linearWorkspaceID, setLinearWorkspaceID] = useState("")
  const [linearTeamKey, setLinearTeamKey] = useState("")
  const [linearAPIKeyEnv, setLinearAPIKeyEnv] = useState("")

  const { data: workspaces, isLoading } = useQuery({
    queryKey: ["workspaces"],
    queryFn: api.listWorkspaces,
  })

  const createMutation = useMutation({
    mutationFn: api.createWorkspace,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["workspaces"] })
      setOpen(false)
      setName("")
      setDescription("")
      setLinearWorkspaceID("")
      setLinearTeamKey("")
      setLinearAPIKeyEnv("")
      toast.success("Workspace created")
    },
    onError: (err) => toast.error(getErrorMessage(err)),
  })

  return (
    <>
      <Breadcrumbs items={[{ label: "Dashboard", to: "/dashboard" }, { label: "Workspaces" }]} />
      <PageHeader title="Workspaces" description="Per-tenancy Linear sync targets — API key + team defaults">
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal gap-1.5">
              <Plus className="h-4 w-4" /> New Workspace
            </Button>
          </DialogTrigger>
          <DialogContent className="shadow-stripe-deep rounded-md">
            <DialogHeader>
              <DialogTitle className="text-[22px] font-light tracking-[-0.22px]">
                Create Workspace
              </DialogTitle>
            </DialogHeader>
            <form
              className="space-y-4"
              onSubmit={(e) => {
                e.preventDefault()
                if (!name.trim()) return
                createMutation.mutate({
                  name,
                  description: description || undefined,
                  linear_workspace_id: linearWorkspaceID || undefined,
                  linear_team_key: linearTeamKey || undefined,
                  linear_api_key_env: linearAPIKeyEnv || undefined,
                })
              }}
            >
              <Field label="Name" value={name} onChange={setName} placeholder="Beaconize" />
              <Field label="Description" value={description} onChange={setDescription} />
              <Field label="Linear Workspace ID" value={linearWorkspaceID} onChange={setLinearWorkspaceID} placeholder="(optional)" mono />
              <Field label="Default Team Key" value={linearTeamKey} onChange={setLinearTeamKey} placeholder="BCN" mono />
              <Field
                label="API Key Env Var"
                value={linearAPIKeyEnv}
                onChange={setLinearAPIKeyEnv}
                placeholder="(optional) BEACONIZE_KEY"
                mono
                help="If set, sync calls for projects in this workspace read the API key from this env var instead of LINEAR_API_KEY."
              />
              <Button
                type="submit"
                disabled={createMutation.isPending || !name.trim()}
                className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
              >
                {createMutation.isPending ? "Creating..." : "Create"}
              </Button>
            </form>
          </DialogContent>
        </Dialog>
      </PageHeader>

      {isLoading ? (
        <p className="text-muted-foreground">Loading...</p>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
          {workspaces?.map((w) => (
            <Link
              key={w.uuid}
              to={`/workspaces/${w.uuid}`}
              className="block transition-shadow hover:shadow-stripe-deep"
            >
              <Card className="rounded-md border-border shadow-stripe-elevated h-full">
                <CardHeader className="pb-2">
                  <div className="flex items-center justify-between">
                    <CardTitle className="text-base font-light text-foreground truncate">
                      {w.name}
                    </CardTitle>
                    {w.linear_team_key && (
                      <Badge variant="outline" className="text-[10px] font-mono rounded-sm px-1.5 py-px">
                        {w.linear_team_key}
                      </Badge>
                    )}
                  </div>
                </CardHeader>
                <CardContent>
                  {w.description && (
                    <p className="text-sm font-light text-muted-foreground mb-2">
                      {w.description}
                    </p>
                  )}
                  <p className="mt-2 text-xs text-muted-foreground font-mono">
                    {w.slug}
                    {w.linear_api_key_env ? ` · key: ${w.linear_api_key_env}` : ""}
                  </p>
                </CardContent>
              </Card>
            </Link>
          ))}
          {workspaces?.length === 0 && (
            <p className="text-sm text-muted-foreground col-span-full">
              No workspaces yet. Create one to route projects through a different Linear API key + team.
            </p>
          )}
        </div>
      )}
    </>
  )
}

function Field({
  label,
  value,
  onChange,
  placeholder,
  mono,
  help,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  placeholder?: string
  mono?: boolean
  help?: string
}) {
  return (
    <div className="space-y-2">
      <Label className="text-[var(--stripe-label)] text-sm font-normal">{label}</Label>
      <Input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className={`border-border rounded-sm ${mono ? "font-mono text-xs" : ""}`}
      />
      {help && <p className="text-[11px] text-muted-foreground">{help}</p>}
    </div>
  )
}

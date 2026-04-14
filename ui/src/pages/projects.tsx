import { useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { PageHeader } from "@/components/layout/page-header"
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
import { Plus, RefreshCw } from "lucide-react"

export function ProjectsPage() {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")

  const { data: projects, isLoading } = useQuery({
    queryKey: ["projects"],
    queryFn: api.listProjects,
  })

  const createMutation = useMutation({
    mutationFn: api.createProject,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["projects"] })
      setOpen(false)
      setName("")
      setDescription("")
      toast.success("Project created")
    },
    onError: (err) => {
      toast.error(getErrorMessage(err))
    },
  })

  const syncMutation = useMutation({
    mutationFn: api.syncProjects,
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ["projects"] })
      if (result.created_count > 0) {
        toast.success(
          `Synced: created ${result.created_count} project(s): ${result.created.join(", ")}`,
        )
      } else {
        toast.info("All projects already in sync.")
      }
    },
    onError: (err) => {
      toast.error(getErrorMessage(err))
    },
  })

  const statusColor: Record<string, string> = {
    active:
      "bg-[rgba(21,190,83,0.2)] text-[var(--stripe-success-text)] border-[rgba(21,190,83,0.4)]",
    paused: "bg-muted text-muted-foreground",
    archived: "bg-muted text-muted-foreground",
  }

  return (
    <>
      <PageHeader title="Projects" description="Manage projects">
        <Button
          variant="outline"
          onClick={() => syncMutation.mutate()}
          disabled={syncMutation.isPending}
          className="border-border rounded-sm font-normal gap-1.5"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${syncMutation.isPending ? "animate-spin" : ""}`} />
          Sync from Vault
        </Button>
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal gap-1.5">
              <Plus className="h-4 w-4" /> New Project
            </Button>
          </DialogTrigger>
          <DialogContent className="shadow-stripe-deep rounded-md">
            <DialogHeader>
              <DialogTitle className="text-[22px] font-light tracking-[-0.22px]">
                Create Project
              </DialogTitle>
            </DialogHeader>
            <form
              className="space-y-4"
              onSubmit={(e) => {
                e.preventDefault()
                if (!name.trim()) return
                createMutation.mutate({ name, description })
              }}
            >
              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">Name</Label>
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  className="border-border rounded-sm"
                />
              </div>
              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">Description</Label>
                <Input
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  className="border-border rounded-sm"
                />
              </div>
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
          {projects?.map((p) => (
            <Card key={p.id} className="rounded-md border-border shadow-stripe-elevated">
              <CardHeader className="pb-2">
                <div className="flex items-center justify-between">
                  <CardTitle className="text-base font-light text-foreground truncate">
                    {p.name}
                  </CardTitle>
                  <Badge className={`text-[10px] font-light rounded-sm px-1.5 py-px ${statusColor[p.status] ?? ""}`}>
                    {p.status}
                  </Badge>
                </div>
              </CardHeader>
              <CardContent>
                {p.description && (
                  <p className="text-sm font-light text-muted-foreground mb-2">
                    {p.description}
                  </p>
                )}
                {p.tags && p.tags.length > 0 && (
                  <div className="flex flex-wrap gap-1">
                    {p.tags.map((tag) => (
                      <Badge key={tag} variant="outline" className="text-[10px] font-normal rounded-sm px-1.5">
                        {tag}
                      </Badge>
                    ))}
                  </div>
                )}
                <p className="mt-2 text-xs text-muted-foreground font-mono">
                  {p.id}
                </p>
              </CardContent>
            </Card>
          ))}
          {projects?.length === 0 && (
            <p className="text-sm text-muted-foreground col-span-full">No projects yet.</p>
          )}
        </div>
      )}
    </>
  )
}

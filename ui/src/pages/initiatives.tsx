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

export function InitiativesPage() {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [owner, setOwner] = useState("")

  const { data: initiatives, isLoading } = useQuery({
    queryKey: ["initiatives"],
    queryFn: api.listInitiatives,
  })

  const createMutation = useMutation({
    mutationFn: api.createInitiative,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["initiatives"] })
      setOpen(false)
      setName("")
      setDescription("")
      setOwner("")
      toast.success("Initiative created")
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
      <Breadcrumbs items={[{ label: "Dashboard", to: "/dashboard" }, { label: "Initiatives" }]} />
      <PageHeader title="Initiatives" description="Strategic themes that group projects">
        <Dialog open={open} onOpenChange={setOpen}>
          <DialogTrigger asChild>
            <Button className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal gap-1.5">
              <Plus className="h-4 w-4" /> New Initiative
            </Button>
          </DialogTrigger>
          <DialogContent className="shadow-stripe-deep rounded-md">
            <DialogHeader>
              <DialogTitle className="text-[22px] font-light tracking-[-0.22px]">
                Create Initiative
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
                  owner: owner || undefined,
                })
              }}
            >
              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">Name</Label>
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="BI Layer"
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
              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">Owner</Label>
                <Input
                  value={owner}
                  onChange={(e) => setOwner(e.target.value)}
                  placeholder="Optional"
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
          {initiatives?.map((i) => (
            <Link
              key={i.uuid}
              to={`/initiatives/${i.uuid}`}
              className="block transition-shadow hover:shadow-stripe-deep"
            >
              <Card className="rounded-md border-border shadow-stripe-elevated h-full">
                <CardHeader className="pb-2">
                  <div className="flex items-center justify-between">
                    <CardTitle className="text-base font-light text-foreground truncate">
                      {i.name}
                    </CardTitle>
                    <Badge className={`text-[10px] font-light rounded-sm px-1.5 py-px ${statusColor[i.status] ?? ""}`}>
                      {i.status}
                    </Badge>
                  </div>
                </CardHeader>
                <CardContent>
                  {i.description && (
                    <p className="text-sm font-light text-muted-foreground mb-2">
                      {i.description}
                    </p>
                  )}
                  <p className="mt-2 text-xs text-muted-foreground font-mono">
                    {i.slug}
                    {i.owner ? ` · ${i.owner}` : ""}
                  </p>
                </CardContent>
              </Card>
            </Link>
          ))}
          {initiatives?.length === 0 && (
            <p className="text-sm text-muted-foreground col-span-full">
              No initiatives yet. Create one to group projects under a strategic theme.
            </p>
          )}
        </div>
      )}
    </>
  )
}

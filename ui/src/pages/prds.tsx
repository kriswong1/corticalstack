import { useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { PageHeader } from "@/components/layout/page-header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { api } from "@/lib/api"
import { Plus } from "lucide-react"

const statusColors: Record<string, string> = {
  draft: "bg-muted text-muted-foreground",
  review: "bg-secondary text-secondary-foreground",
  approved: "bg-[rgba(21,190,83,0.2)] text-[var(--stripe-success-text)] border-[rgba(21,190,83,0.4)]",
  shipped: "bg-primary/20 text-primary",
  archived: "bg-muted text-muted-foreground",
}

export function PRDsPage() {
  const queryClient = useQueryClient()
  const [showForm, setShowForm] = useState(false)
  const [pitchPath, setPitchPath] = useState("")
  const [extraPaths, setExtraPaths] = useState("")
  const [extraTags, setExtraTags] = useState("")
  const [projectIds, setProjectIds] = useState("")

  const { data: prds, isLoading } = useQuery({
    queryKey: ["prds"],
    queryFn: api.listPRDs,
  })

  const createMutation = useMutation({
    mutationFn: () =>
      api.createPRD({
        pitch_path: pitchPath,
        extra_context_paths: extraPaths.split("\n").map((s) => s.trim()).filter(Boolean),
        extra_context_tags: extraTags.split(",").map((s) => s.trim()).filter(Boolean),
        project_ids: projectIds.split(",").map((s) => s.trim()).filter(Boolean),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["prds"] })
      setShowForm(false)
      setPitchPath("")
      setExtraPaths("")
      setExtraTags("")
      setProjectIds("")
    },
  })

  return (
    <>
      <PageHeader title="PRDs" description="Product Requirements Documents">
        <Button
          onClick={() => setShowForm(!showForm)}
          className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal gap-1.5"
        >
          <Plus className="h-4 w-4" /> New PRD
        </Button>
      </PageHeader>

      {showForm && (
        <Card className="mb-6 rounded-md border-border shadow-stripe">
          <CardContent className="pt-6">
            <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); createMutation.mutate() }}>
              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">Pitch Path</Label>
                <Input value={pitchPath} onChange={(e) => setPitchPath(e.target.value)} placeholder="shapeup/pitch/..." className="border-border rounded-sm" />
              </div>
              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">Extra Context Paths (one per line)</Label>
                <Input value={extraPaths} onChange={(e) => setExtraPaths(e.target.value)} className="border-border rounded-sm" />
              </div>
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">Extra Tags (comma-separated)</Label>
                  <Input value={extraTags} onChange={(e) => setExtraTags(e.target.value)} className="border-border rounded-sm" />
                </div>
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">Project IDs (comma-separated)</Label>
                  <Input value={projectIds} onChange={(e) => setProjectIds(e.target.value)} className="border-border rounded-sm" />
                </div>
              </div>
              <Button type="submit" disabled={createMutation.isPending || !pitchPath.trim()} className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal">
                {createMutation.isPending ? "Synthesizing..." : "Synthesize PRD"}
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
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Title</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Status</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Version</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Open Questions</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {prds?.map((prd) => (
                <TableRow key={prd.id}>
                  <TableCell className="text-sm font-light">{prd.title}</TableCell>
                  <TableCell>
                    <Badge className={`text-[10px] font-light rounded-sm px-1.5 py-px ${statusColors[prd.status] ?? ""}`}>
                      {prd.status}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground font-[feature-settings:'tnum']">
                    v{prd.version}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground font-[feature-settings:'tnum']">
                    {prd.open_questions_count}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground font-mono">
                    {new Date(prd.created).toLocaleDateString()}
                  </TableCell>
                </TableRow>
              ))}
              {prds?.length === 0 && (
                <TableRow>
                  <TableCell colSpan={5} className="text-center text-sm text-muted-foreground py-8">No PRDs yet.</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      )}
    </>
  )
}

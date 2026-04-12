import { useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { PageHeader } from "@/components/layout/page-header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { api } from "@/lib/api"
import { Plus, ArrowRight } from "lucide-react"

const stages = ["raw", "frame", "shape", "breadboard", "pitch"]

const stageColors: Record<string, string> = {
  raw: "bg-muted text-muted-foreground",
  frame: "bg-secondary text-secondary-foreground",
  shape: "bg-primary/20 text-primary",
  breadboard: "bg-primary/30 text-primary",
  pitch: "bg-[rgba(21,190,83,0.2)] text-[var(--stripe-success-text)] border-[rgba(21,190,83,0.4)]",
}

export function ProductPage() {
  const queryClient = useQueryClient()
  const [showForm, setShowForm] = useState(false)
  const [title, setTitle] = useState("")
  const [content, setContent] = useState("")
  const [projectIds, setProjectIds] = useState("")

  const { data: threads, isLoading } = useQuery({
    queryKey: ["shapeup-threads"],
    queryFn: api.listThreads,
  })

  const createMutation = useMutation({
    mutationFn: api.createIdea,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["shapeup-threads"] })
      setShowForm(false)
      setTitle("")
      setContent("")
      setProjectIds("")
    },
  })

  return (
    <>
      <PageHeader title="Product" description="ShapeUp pipeline — Raw to Pitch">
        <Button
          onClick={() => setShowForm(!showForm)}
          className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal gap-1.5"
        >
          <Plus className="h-4 w-4" /> New Idea
        </Button>
      </PageHeader>

      {showForm && (
        <Card className="mb-6 rounded-md border-border shadow-stripe">
          <CardContent className="pt-6">
            <form
              className="space-y-4"
              onSubmit={(e) => {
                e.preventDefault()
                if (!title.trim() || !content.trim()) return
                createMutation.mutate({
                  title,
                  content,
                  project_ids: projectIds.split("\n").map((s) => s.trim()).filter(Boolean),
                })
              }}
            >
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">Title</Label>
                  <Input value={title} onChange={(e) => setTitle(e.target.value)} className="border-border rounded-sm" />
                </div>
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">Projects (one per line)</Label>
                  <Input value={projectIds} onChange={(e) => setProjectIds(e.target.value)} className="border-border rounded-sm" />
                </div>
              </div>
              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">Content</Label>
                <Textarea value={content} onChange={(e) => setContent(e.target.value)} rows={4} className="border-border rounded-sm" />
              </div>
              <Button
                type="submit"
                disabled={createMutation.isPending}
                className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
              >
                {createMutation.isPending ? "Creating..." : "Create Idea"}
              </Button>
            </form>
          </CardContent>
        </Card>
      )}

      {isLoading ? (
        <p className="text-muted-foreground">Loading...</p>
      ) : (
        <div className="space-y-4">
          {threads?.map((thread) => (
            <ThreadCard key={thread.id} thread={thread} />
          ))}
          {threads?.length === 0 && (
            <p className="text-sm text-muted-foreground">No threads yet. Create an idea to start.</p>
          )}
        </div>
      )}
    </>
  )
}

function ThreadCard({ thread }: { thread: { id: string; title: string; current_stage: string; projects?: string[]; artifacts: { id: string; stage: string; title: string }[] } }) {
  const queryClient = useQueryClient()
  const [targetStage, setTargetStage] = useState("")
  const [hints, setHints] = useState("")

  const advanceMutation = useMutation({
    mutationFn: () => api.advanceThread(thread.id, { target_stage: targetStage, hints }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["shapeup-threads"] })
      setTargetStage("")
      setHints("")
    },
  })

  const currentIdx = stages.indexOf(thread.current_stage)
  const nextStages = stages.slice(currentIdx + 1)

  return (
    <Card className="rounded-md border-border shadow-stripe-elevated">
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between">
          <CardTitle className="text-base font-light text-foreground">{thread.title}</CardTitle>
          <Badge className={`text-[10px] font-light rounded-sm px-1.5 py-px ${stageColors[thread.current_stage] ?? ""}`}>
            {thread.current_stage}
          </Badge>
        </div>
      </CardHeader>
      <CardContent>
        <div className="flex items-center gap-1 mb-3">
          {stages.map((s, i) => (
            <span key={s} className="flex items-center gap-1">
              <span className={`text-xs ${stages.indexOf(thread.current_stage) >= i ? "text-primary font-normal" : "text-muted-foreground font-light"}`}>
                {s}
              </span>
              {i < stages.length - 1 && <ArrowRight className="h-3 w-3 text-muted-foreground" />}
            </span>
          ))}
        </div>

        {thread.projects && thread.projects.length > 0 && (
          <div className="flex flex-wrap gap-1 mb-3">
            {thread.projects.map((pid) => (
              <Badge key={pid} variant="outline" className="text-[10px] font-normal rounded-sm px-1">
                {pid}
              </Badge>
            ))}
          </div>
        )}

        <p className="text-xs text-muted-foreground mb-3">
          {thread.artifacts.length} artifact(s)
        </p>

        {nextStages.length > 0 && (
          <div className="flex items-end gap-3 border-t border-border pt-3">
            <div className="space-y-1">
              <Label className="text-xs text-[var(--stripe-label)]">Advance to</Label>
              <Select value={targetStage} onValueChange={setTargetStage}>
                <SelectTrigger className="h-7 w-32 text-xs border-border rounded-sm">
                  <SelectValue placeholder="Stage..." />
                </SelectTrigger>
                <SelectContent>
                  {nextStages.map((s) => (
                    <SelectItem key={s} value={s}>{s}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex-1 space-y-1">
              <Label className="text-xs text-[var(--stripe-label)]">Hints</Label>
              <Input
                value={hints}
                onChange={(e) => setHints(e.target.value)}
                placeholder="Optional guidance..."
                className="h-7 text-xs border-border rounded-sm"
              />
            </div>
            <Button
              size="sm"
              onClick={() => advanceMutation.mutate()}
              disabled={!targetStage || advanceMutation.isPending}
              className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal text-xs h-7"
            >
              {advanceMutation.isPending ? "Advancing..." : "Advance"}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

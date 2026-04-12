import { useState } from "react"
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
import type { PreviewResult } from "@/types/api"
import { api } from "@/lib/api"

const intentions = [
  "learning",
  "information",
  "research",
  "project-application",
  "other",
]

interface PreviewPanelProps {
  preview: PreviewResult
  jobId: string
  onConfirmed: () => void
}

export function PreviewPanel({ preview, jobId, onConfirmed }: PreviewPanelProps) {
  const [title, setTitle] = useState(preview.suggested_title ?? "")
  const [intention, setIntention] = useState(preview.intention)
  const [projects, setProjects] = useState(
    (preview.suggested_project_ids ?? []).join("\n"),
  )
  const [why, setWhy] = useState("")
  const [confirming, setConfirming] = useState(false)

  async function handleConfirm() {
    setConfirming(true)
    try {
      await api.confirmJob(jobId, {
        title,
        intention,
        project_ids: projects
          .split("\n")
          .map((s) => s.trim())
          .filter(Boolean),
        why,
      })
      onConfirmed()
    } catch (err) {
      alert("Confirm failed: " + (err instanceof Error ? err.message : String(err)))
    } finally {
      setConfirming(false)
    }
  }

  return (
    <div className="rounded-md border border-primary/30 bg-secondary/30 p-4 space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-base font-normal text-foreground">
          Claude's Proposal
        </h3>
        {preview.confidence != null && (
          <span className="text-xs text-muted-foreground">
            Confidence: {(preview.confidence * 100).toFixed(0)}%
          </span>
        )}
      </div>

      {preview.reasoning && (
        <p className="text-xs font-light text-muted-foreground">
          {preview.reasoning}
        </p>
      )}

      <p className="text-sm font-light text-foreground">{preview.summary}</p>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <div className="space-y-2">
          <Label className="text-[var(--stripe-label)] text-sm font-normal">
            Title
          </Label>
          <Input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            className="border-border rounded-sm"
          />
        </div>
        <div className="space-y-2">
          <Label className="text-[var(--stripe-label)] text-sm font-normal">
            Intention
          </Label>
          <Select value={intention} onValueChange={setIntention}>
            <SelectTrigger className="border-border rounded-sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {intentions.map((i) => (
                <SelectItem key={i} value={i}>
                  {i}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="space-y-2">
        <Label className="text-[var(--stripe-label)] text-sm font-normal">
          Projects (one per line)
        </Label>
        <Textarea
          value={projects}
          onChange={(e) => setProjects(e.target.value)}
          rows={3}
          className="border-border rounded-sm font-mono text-xs"
        />
      </div>

      <div className="space-y-2">
        <Label className="text-[var(--stripe-label)] text-sm font-normal">
          Why (optional context)
        </Label>
        <Input
          value={why}
          onChange={(e) => setWhy(e.target.value)}
          placeholder="Why did you save this?"
          className="border-border rounded-sm"
        />
      </div>

      <Button
        onClick={handleConfirm}
        disabled={confirming}
        className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal text-base"
      >
        {confirming ? "Confirming..." : "Confirm & Extract"}
      </Button>
    </div>
  )
}

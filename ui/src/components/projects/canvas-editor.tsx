import { useEffect, useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { api, getErrorMessage } from "@/lib/api"

interface CanvasEditorProps {
  projectId: string
}

/**
 * CanvasEditor reads + writes the user-editable `## Canvas` section of
 * a project's `project.md` manifest. Round-tripped on disk between the
 * deterministic header and footer the writer regenerates each save.
 *
 * Dirty-flag UX: only enables Save when the local buffer differs from
 * the most recently fetched value. After a successful save we replace
 * the baseline with the saved buffer so a second click without changes
 * stays disabled — the user knows the write landed.
 */
export function CanvasEditor({ projectId }: CanvasEditorProps) {
  const queryClient = useQueryClient()
  const [buffer, setBuffer] = useState("")
  const [baseline, setBaseline] = useState("")

  const { data, isLoading } = useQuery({
    queryKey: ["project-canvas", projectId],
    queryFn: () => api.getProjectCanvas(projectId),
    enabled: !!projectId,
  })

  // Hydrate the buffer once the canvas comes back. Only on initial load
  // or projectId change — otherwise typing would reset on every refetch.
  useEffect(() => {
    if (data) {
      setBuffer(data.canvas)
      setBaseline(data.canvas)
    }
  }, [data?.canvas, projectId])

  const saveMutation = useMutation({
    mutationFn: (next: string) => api.setProjectCanvas(projectId, next),
    onSuccess: (_, variables) => {
      setBaseline(variables)
      queryClient.invalidateQueries({ queryKey: ["project-canvas", projectId] })
      toast.success("Canvas saved")
    },
    onError: (err) => toast.error(getErrorMessage(err)),
  })

  const dirty = buffer !== baseline

  return (
    <Card className="rounded-md border-border shadow-stripe-elevated">
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
        <CardTitle className="text-base font-light">Canvas</CardTitle>
        <Button
          size="sm"
          disabled={!dirty || saveMutation.isPending || isLoading}
          onClick={() => saveMutation.mutate(buffer)}
          className="h-7 text-xs bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
        >
          {saveMutation.isPending ? "Saving..." : dirty ? "Save" : "Saved"}
        </Button>
      </CardHeader>
      <CardContent>
        <Textarea
          value={buffer}
          onChange={(e) => setBuffer(e.target.value)}
          rows={12}
          placeholder={
            "Bet, appetite, boundaries, open questions — whatever frames this project.\n\n" +
            "Round-tripped between the deterministic header and footer; the writer never clobbers what's between the `## Canvas` heading and the next `## ` section."
          }
          className="border-border rounded-sm font-mono text-xs"
        />
      </CardContent>
    </Card>
  )
}

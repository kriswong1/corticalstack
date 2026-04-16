import { useState } from "react"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Card, CardContent } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { api, getErrorMessage } from "@/lib/api"
import { Check, RotateCcw, Pencil, Save } from "lucide-react"
import Markdown from "react-markdown"

interface PersonaPreviewProps {
  personaName: string
  sessionId: string
  content: string
  onAccepted: () => void
  onRerun: () => void
}

export function PersonaPreview({
  personaName,
  sessionId,
  content,
  onAccepted,
  onRerun,
}: PersonaPreviewProps) {
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [editContent, setEditContent] = useState(content)

  const acceptMutation = useMutation({
    mutationFn: () =>
      api.acceptPersonaChat(personaName, { session_id: sessionId }),
    onSuccess: () => {
      toast.success(`${personaName.toUpperCase()} persona saved`)
      queryClient.invalidateQueries({ queryKey: ["persona", personaName] })
      queryClient.invalidateQueries({ queryKey: ["persona-status"] })
      queryClient.invalidateQueries({ queryKey: ["onboarding-status"] })
      onAccepted()
    },
    onError: (err) => toast.error(getErrorMessage(err)),
  })

  const saveEditMutation = useMutation({
    mutationFn: () => api.savePersona(personaName, editContent),
    onSuccess: () => {
      toast.success(`${personaName.toUpperCase()} persona saved`)
      queryClient.invalidateQueries({ queryKey: ["persona", personaName] })
      queryClient.invalidateQueries({ queryKey: ["persona-status"] })
      queryClient.invalidateQueries({ queryKey: ["onboarding-status"] })
      onAccepted()
    },
    onError: (err) => toast.error(getErrorMessage(err)),
  })

  return (
    <Card className="rounded-md border-border shadow-stripe">
      <CardContent className="p-0">
        <div className="flex items-center justify-between px-5 py-3 border-b border-border">
          <span className="text-[13px] font-semibold text-foreground">
            Preview: {personaName.toUpperCase()}.md
          </span>
          <span className="text-[11px] text-muted-foreground">
            {editing ? "Editing" : "Review the generated file"}
          </span>
        </div>

        <div className="px-5 py-4" style={{ maxHeight: "500px", overflowY: "auto" }}>
          {editing ? (
            <Textarea
              value={editContent}
              onChange={(e) => setEditContent(e.target.value)}
              rows={20}
              className="border-border rounded-sm font-mono text-xs leading-relaxed"
            />
          ) : (
            <div className="prose prose-sm max-w-none dark:prose-invert prose-headings:text-foreground prose-p:text-foreground/90 prose-code:bg-muted prose-code:px-1.5 prose-code:py-0.5 prose-code:rounded prose-pre:bg-muted/60 prose-pre:border prose-pre:border-border">
              <Markdown>{content}</Markdown>
            </div>
          )}
        </div>

        <div className="flex items-center gap-2 px-5 py-3 border-t border-border">
          {editing ? (
            <>
              <Button
                size="sm"
                variant="outline"
                onClick={() => setEditing(false)}
                className="text-[12px] rounded-sm"
              >
                Cancel
              </Button>
              <Button
                size="sm"
                onClick={() => saveEditMutation.mutate()}
                disabled={saveEditMutation.isPending}
                className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground text-[12px] rounded-sm gap-1.5"
              >
                <Save className="h-3.5 w-3.5" />
                {saveEditMutation.isPending ? "Saving..." : "Save"}
              </Button>
            </>
          ) : (
            <>
              <Button
                size="sm"
                onClick={() => acceptMutation.mutate()}
                disabled={acceptMutation.isPending}
                className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground text-[12px] rounded-sm gap-1.5"
              >
                <Check className="h-3.5 w-3.5" />
                {acceptMutation.isPending ? "Saving..." : "Accept"}
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={onRerun}
                className="text-[12px] rounded-sm gap-1.5"
              >
                <RotateCcw className="h-3.5 w-3.5" />
                Re-run
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setEditContent(content)
                  setEditing(true)
                }}
                className="text-[12px] rounded-sm gap-1.5"
              >
                <Pencil className="h-3.5 w-3.5" />
                Edit
              </Button>
            </>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

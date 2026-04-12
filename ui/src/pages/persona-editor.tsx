import { useState, useEffect } from "react"
import { useParams } from "react-router-dom"
import { useQuery, useMutation } from "@tanstack/react-query"
import { PageHeader } from "@/components/layout/page-header"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { Badge } from "@/components/ui/badge"
import { api } from "@/lib/api"
import { Save } from "lucide-react"

const titles: Record<string, string> = {
  soul: "SOUL — Extraction Style",
  user: "USER — Profile",
  memory: "MEMORY — Curated Index",
}

export function PersonaEditorPage() {
  const { name } = useParams<{ name: string }>()
  const title = titles[name ?? ""] ?? "Persona"

  const { data: persona } = useQuery({
    queryKey: ["persona", name],
    queryFn: () => api.getPersona(name!),
    enabled: !!name,
  })

  const [content, setContent] = useState("")
  const [saveStatus, setSaveStatus] = useState<"idle" | "saving" | "saved" | "error">("idle")

  useEffect(() => {
    if (persona?.content != null) {
      setContent(persona.content)
    }
  }, [persona?.content])

  const saveMutation = useMutation({
    mutationFn: () => api.savePersona(name!, content),
    onMutate: () => setSaveStatus("saving"),
    onSuccess: () => {
      setSaveStatus("saved")
      setTimeout(() => setSaveStatus("idle"), 2000)
    },
    onError: () => setSaveStatus("error"),
  })

  const charCount = content.length
  const budget = persona?.budget ?? 0
  const overBudget = budget > 0 && charCount > budget

  return (
    <>
      <PageHeader title={title} description={`Edit ${name?.toUpperCase()} persona file`}>
        <div className="flex items-center gap-3">
          {saveStatus === "saved" && (
            <span className="text-xs text-[var(--stripe-success-text)]">Saved</span>
          )}
          {saveStatus === "error" && (
            <span className="text-xs text-destructive">Error saving</span>
          )}
          <Button
            onClick={() => saveMutation.mutate()}
            disabled={saveMutation.isPending}
            className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal gap-1.5"
          >
            <Save className="h-4 w-4" />
            {saveMutation.isPending ? "Saving..." : "Save"}
          </Button>
        </div>
      </PageHeader>

      {persona && (
        <div className="flex items-center gap-3 mb-4">
          <Badge variant="outline" className="text-[11px] font-normal rounded-sm px-1.5">
            {persona.file}
          </Badge>
          <span className={`text-xs font-mono ${overBudget ? "text-destructive" : "text-muted-foreground"}`}>
            {charCount.toLocaleString()} / {budget.toLocaleString()} chars
          </span>
        </div>
      )}

      <Textarea
        value={content}
        onChange={(e) => setContent(e.target.value)}
        rows={24}
        className="border-border rounded-sm font-mono text-xs leading-relaxed"
        placeholder={`Enter ${name?.toUpperCase()} content...`}
      />
    </>
  )
}

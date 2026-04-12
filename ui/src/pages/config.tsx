import { useState, useEffect } from "react"
import { useQuery, useMutation } from "@tanstack/react-query"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Separator } from "@/components/ui/separator"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { PageHeader } from "@/components/layout/page-header"
import { api } from "@/lib/api"
import { Save } from "lucide-react"

const personaNames = ["soul", "user", "memory"] as const
const personaTitles: Record<string, string> = {
  soul: "SOUL — Extraction Style",
  user: "USER — Profile",
  memory: "MEMORY — Curated Index",
}

export function ConfigPage() {
  const { data: status, isLoading } = useQuery({
    queryKey: ["status"],
    queryFn: api.getStatus,
  })

  if (isLoading) {
    return (
      <>
        <PageHeader title="Config" description="System configuration and persona files" />
        <p className="text-muted-foreground">Loading...</p>
      </>
    )
  }

  return (
    <>
      <PageHeader title="Config" description="System configuration and persona files" />

      <div className="space-y-6">
        <Card className="rounded-md border-border shadow-stripe">
          <CardHeader>
            <CardTitle className="text-[22px] font-light tracking-[-0.22px] text-foreground">
              Environment
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <ConfigRow label="Vault Path" value={status?.vault_path ?? "—"} />
            <Separator />
            <ConfigRow label="Server Time" value={status?.server_time ?? "—"} />
          </CardContent>
        </Card>

        <Card className="rounded-md border-border shadow-stripe">
          <CardHeader>
            <CardTitle className="text-[22px] font-light tracking-[-0.22px] text-foreground">
              Integrations
            </CardTitle>
          </CardHeader>
          <CardContent>
            {status?.integrations?.map((integ) => (
              <div key={integ.id} className="flex items-center justify-between py-2">
                <div>
                  <span className="text-sm font-light text-foreground">{integ.name}</span>
                  <span className="ml-2 text-xs text-muted-foreground">({integ.id})</span>
                </div>
                <div className="flex items-center gap-2">
                  <Badge
                    variant="outline"
                    className="text-[10px] font-light rounded-sm px-1.5 py-px"
                  >
                    {integ.configured ? "Configured" : "Not configured"}
                  </Badge>
                  {integ.configured && (
                    <Badge
                      className={
                        integ.healthy
                          ? "bg-[rgba(21,190,83,0.2)] text-[var(--stripe-success-text)] border-[rgba(21,190,83,0.4)] text-[10px] font-light rounded-sm px-1.5 py-px"
                          : "bg-destructive/20 text-destructive border-destructive/40 text-[10px] font-light rounded-sm px-1.5 py-px"
                      }
                    >
                      {integ.healthy ? "Healthy" : "Error"}
                    </Badge>
                  )}
                </div>
              </div>
            ))}
            {(!status?.integrations || status.integrations.length === 0) && (
              <p className="text-sm font-light text-muted-foreground">No integrations registered</p>
            )}
          </CardContent>
        </Card>

        <Card className="rounded-md border-border shadow-stripe">
          <CardHeader>
            <CardTitle className="text-[22px] font-light tracking-[-0.22px] text-foreground">
              Pipeline
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <h3 className="text-sm font-normal text-[var(--stripe-label)] mb-2">
                Transformers
              </h3>
              <div className="flex flex-wrap gap-1.5">
                {status?.transformers?.map((t) => (
                  <Badge key={t} variant="outline" className="text-[11px] font-normal rounded-sm px-1.5">
                    {t}
                  </Badge>
                ))}
              </div>
            </div>
            <Separator />
            <div>
              <h3 className="text-sm font-normal text-[var(--stripe-label)] mb-2">
                Destinations
              </h3>
              <div className="flex flex-wrap gap-1.5">
                {status?.destinations?.map((d) => (
                  <Badge key={d} variant="outline" className="text-[11px] font-normal rounded-sm px-1.5">
                    {d}
                  </Badge>
                ))}
              </div>
            </div>
          </CardContent>
        </Card>

        <Card className="rounded-md border-border shadow-stripe">
          <CardHeader>
            <CardTitle className="text-[22px] font-light tracking-[-0.22px] text-foreground">
              Persona Files
            </CardTitle>
          </CardHeader>
          <CardContent>
            <Tabs defaultValue="soul">
              <TabsList className="mb-4">
                {personaNames.map((name) => (
                  <TabsTrigger key={name} value={name} className="uppercase text-xs">
                    {name}
                  </TabsTrigger>
                ))}
              </TabsList>
              {personaNames.map((name) => (
                <TabsContent key={name} value={name}>
                  <PersonaEditor name={name} />
                </TabsContent>
              ))}
            </Tabs>
          </CardContent>
        </Card>
      </div>
    </>
  )
}

function PersonaEditor({ name }: { name: string }) {
  const title = personaTitles[name] ?? "Persona"

  const { data: persona } = useQuery({
    queryKey: ["persona", name],
    queryFn: () => api.getPersona(name),
  })

  const [content, setContent] = useState("")
  const [saveStatus, setSaveStatus] = useState<"idle" | "saving" | "saved" | "error">("idle")

  useEffect(() => {
    if (persona?.content != null) {
      setContent(persona.content)
    }
  }, [persona?.content])

  const saveMutation = useMutation({
    mutationFn: () => api.savePersona(name, content),
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
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-base font-light text-foreground">{title}</h3>
          {persona && (
            <div className="flex items-center gap-3 mt-1">
              <Badge variant="outline" className="text-[11px] font-normal rounded-sm px-1.5">
                {persona.file}
              </Badge>
              <span className={`text-xs font-mono ${overBudget ? "text-destructive" : "text-muted-foreground"}`}>
                {charCount.toLocaleString()} / {budget.toLocaleString()} chars
              </span>
            </div>
          )}
        </div>
        <div className="flex items-center gap-2">
          {saveStatus === "saved" && (
            <span className="text-xs text-[var(--stripe-success-text)]">Saved</span>
          )}
          {saveStatus === "error" && (
            <span className="text-xs text-destructive">Error</span>
          )}
          <Button
            size="sm"
            onClick={() => saveMutation.mutate()}
            disabled={saveMutation.isPending}
            className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal gap-1.5"
          >
            <Save className="h-3.5 w-3.5" />
            {saveMutation.isPending ? "Saving..." : "Save"}
          </Button>
        </div>
      </div>

      <Textarea
        value={content}
        onChange={(e) => setContent(e.target.value)}
        rows={18}
        className="border-border rounded-sm font-mono text-xs leading-relaxed"
        placeholder={`Enter ${name.toUpperCase()} content...`}
      />
    </div>
  )
}

function ConfigRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-sm font-normal text-[var(--stripe-label)]">{label}</span>
      <span className="text-sm font-light text-foreground font-mono">{value}</span>
    </div>
  )
}

import { useEffect, useRef, useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Separator } from "@/components/ui/separator"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { PageHeader } from "@/components/layout/page-header"
import { Breadcrumbs } from "@/components/layout/breadcrumbs"
import { QuestionsModal } from "@/components/questions-modal"
import { OnboardingProgress } from "@/components/config/onboarding-progress"
import { ObsidianCard } from "@/components/config/obsidian-card"
import { DeepgramCard } from "@/components/config/deepgram-card"
import { LinearCard } from "@/components/config/linear-card"
import { PersonaTriptych } from "@/components/config/persona-triptych"
import { PersonaChat } from "@/components/config/persona-chat"
import { PersonaPreview } from "@/components/config/persona-preview"
import { api, getErrorMessage } from "@/lib/api"
import { Save, Sparkles, Settings, Plug, User } from "lucide-react"
import type { Answer, Question } from "@/types/api"

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

  const [editingPersona, setEditingPersona] = useState<string | null>(null)
  // Persona chat state machine: idle → chatting → preview
  const [chatPersona, setChatPersona] = useState<string | null>(null)
  const [chatSessionId, setChatSessionId] = useState<string | null>(null)
  const [chatResult, setChatResult] = useState<string | null>(null)
  const chatMode = chatResult ? "preview" : chatPersona ? "chatting" : "idle"

  const startChat = (name: string) => {
    setEditingPersona(null)
    setChatPersona(name)
    setChatSessionId(null)
    setChatResult(null)
  }
  const resetChat = () => {
    setChatPersona(null)
    setChatSessionId(null)
    setChatResult(null)
  }

  if (isLoading) {
    return (
      <>
        <Breadcrumbs items={[{ label: "Dashboard", to: "/dashboard" }, { label: "Config" }]} />
        <PageHeader title="Config" description="System configuration and persona files" />
        <p className="text-muted-foreground">Loading...</p>
      </>
    )
  }

  return (
    <>
      <Breadcrumbs items={[{ label: "Dashboard", to: "/dashboard" }, { label: "Config" }]} />
      <PageHeader title="Config" description="System configuration and persona files" />

      <OnboardingProgress />

      <Tabs defaultValue="integrations" className="space-y-4">
        <TabsList>
          <TabsTrigger value="environment" className="gap-1.5 text-xs">
            <Settings className="h-3.5 w-3.5" />
            Environment
          </TabsTrigger>
          <TabsTrigger value="integrations" className="gap-1.5 text-xs">
            <Plug className="h-3.5 w-3.5" />
            Integrations
          </TabsTrigger>
          <TabsTrigger value="persona" className="gap-1.5 text-xs">
            <User className="h-3.5 w-3.5" />
            Persona
          </TabsTrigger>
        </TabsList>

        {/* ── Environment Tab ── */}
        <TabsContent value="environment" className="space-y-6">
          <Card className="rounded-md border-border shadow-stripe">
            <CardHeader>
              <CardTitle className="text-[22px] font-light tracking-[-0.22px] text-foreground">
                General
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              <ConfigRow label="Server Time" value={status?.server_time ?? "—"} />
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
                <h3 className="text-sm font-normal text-[var(--stripe-label)] mb-2">Transformers</h3>
                <div className="flex flex-wrap gap-1.5">
                  {status?.transformers?.map((t) => (
                    <Badge key={t} variant="outline" className="text-[11px] font-normal rounded-sm px-1.5">{t}</Badge>
                  ))}
                </div>
              </div>
              <Separator />
              <div>
                <h3 className="text-sm font-normal text-[var(--stripe-label)] mb-2">Destinations</h3>
                <div className="flex flex-wrap gap-1.5">
                  {status?.destinations?.map((d) => (
                    <Badge key={d} variant="outline" className="text-[11px] font-normal rounded-sm px-1.5">{d}</Badge>
                  ))}
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        {/* ── Integrations Tab ── */}
        <TabsContent value="integrations" className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
            <ObsidianCard />
            <DeepgramCard />
            <LinearCard />
          </div>
        </TabsContent>

        {/* ── Persona Tab ── */}
        <TabsContent value="persona" className="space-y-6">
          {chatMode === "idle" && (
            <>
              <PersonaTriptych
                onSetup={startChat}
                onEdit={(name) => setEditingPersona(editingPersona === name ? null : name)}
                editingName={editingPersona}
              />

              {editingPersona && (
                <Card className="rounded-md border-border shadow-stripe">
                  <CardContent className="pt-5">
                    <PersonaEditor name={editingPersona} />
                  </CardContent>
                </Card>
              )}
            </>
          )}

          {chatMode === "chatting" && chatPersona && (
            <PersonaChat
              personaName={chatPersona}
              onComplete={(result, sid) => {
                setChatSessionId(sid)
                setChatResult(result)
              }}
              onCancel={resetChat}
            />
          )}

          {chatMode === "preview" && chatPersona && chatResult && (
            <PersonaPreview
              personaName={chatPersona}
              sessionId={chatSessionId ?? ""}
              content={chatResult}
              onAccepted={resetChat}
              onRerun={() => {
                setChatResult(null)
                // chatPersona stays set, so chatMode goes back to "chatting"
              }}
            />
          )}
        </TabsContent>
      </Tabs>
    </>
  )
}

function PersonaSetupForm({ onComplete }: { onComplete: () => void }) {
  const queryClient = useQueryClient()
  const [name, setName] = useState("")
  const [role, setRole] = useState("")
  const [timezone, setTimezone] = useState(Intl.DateTimeFormat().resolvedOptions().timeZone)
  const [context, setContext] = useState("")
  const [projects, setProjects] = useState("")
  const [platforms, setPlatforms] = useState("")

  const mutation = useMutation({
    mutationFn: () =>
      api.setupPersona({
        name,
        role,
        timezone,
        context,
        projects: projects.split("\n").map((s) => s.trim()).filter(Boolean),
        platforms,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["persona", "user"] })
      queryClient.invalidateQueries({ queryKey: ["onboarding-status"] })
      toast.success("USER.md generated")
      onComplete()
    },
    onError: (err) => {
      toast.error(getErrorMessage(err))
    },
  })

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (!name.trim() || !role.trim()) return
        mutation.mutate()
      }}
    >
      <h3 className="text-base font-light text-foreground">Quick Profile Setup</h3>
      <p className="text-xs text-muted-foreground">
        Fill in the basics to personalize your USER.md. Takes 30 seconds.
      </p>
      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        <div className="space-y-2">
          <Label className="text-[var(--stripe-label)] text-sm font-normal">Name *</Label>
          <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Your name" className="border-border rounded-sm" />
        </div>
        <div className="space-y-2">
          <Label className="text-[var(--stripe-label)] text-sm font-normal">Role *</Label>
          <Input value={role} onChange={(e) => setRole(e.target.value)} placeholder="e.g. Senior Engineer" className="border-border rounded-sm" />
        </div>
        <div className="space-y-2">
          <Label className="text-[var(--stripe-label)] text-sm font-normal">Timezone</Label>
          <Input value={timezone} onChange={(e) => setTimezone(e.target.value)} className="border-border rounded-sm" />
        </div>
      </div>
      <div className="space-y-2">
        <Label className="text-[var(--stripe-label)] text-sm font-normal">
          What do you do and why are you using CorticalStack?
        </Label>
        <Textarea value={context} onChange={(e) => setContext(e.target.value)} rows={2} placeholder="1-2 sentences" className="border-border rounded-sm" />
      </div>
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <div className="space-y-2">
          <Label className="text-[var(--stripe-label)] text-sm font-normal">Current projects (one per line)</Label>
          <Textarea value={projects} onChange={(e) => setProjects(e.target.value)} rows={2} className="border-border rounded-sm text-xs" />
        </div>
        <div className="space-y-2">
          <Label className="text-[var(--stripe-label)] text-sm font-normal">Tools/platforms you use</Label>
          <Input value={platforms} onChange={(e) => setPlatforms(e.target.value)} placeholder="e.g. Obsidian, Linear, VS Code" className="border-border rounded-sm" />
        </div>
      </div>
      <Button
        type="submit"
        disabled={mutation.isPending || !name.trim() || !role.trim()}
        className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
      >
        {mutation.isPending ? "Saving..." : "Generate USER.md"}
      </Button>
    </form>
  )
}

function PersonaEditor({ name }: { name: string }) {
  const queryClient = useQueryClient()
  const title = personaTitles[name] ?? "Persona"

  const { data: persona } = useQuery({
    queryKey: ["persona", name],
    queryFn: () => api.getPersona(name),
  })

  const [content, setContent] = useState(persona?.content ?? "")
  const [syncedContent, setSyncedContent] = useState(persona?.content ?? "")
  if (persona?.content != null && persona.content !== syncedContent) {
    setSyncedContent(persona.content)
    setContent(persona.content)
  }
  const [saveStatus, setSaveStatus] = useState<"idle" | "saving" | "saved" | "error">("idle")
  const [modalOpen, setModalOpen] = useState(false)
  const [questions, setQuestions] = useState<Question[] | null>(null)
  const saveStatusTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    return () => {
      if (saveStatusTimerRef.current != null) {
        clearTimeout(saveStatusTimerRef.current)
        saveStatusTimerRef.current = null
      }
    }
  }, [])

  const saveMutation = useMutation({
    mutationFn: () => api.savePersona(name, content),
    onMutate: () => setSaveStatus("saving"),
    onSuccess: () => {
      setSaveStatus("saved")
      queryClient.invalidateQueries({ queryKey: ["persona", name] })
      queryClient.invalidateQueries({ queryKey: ["onboarding-status"] })
      if (saveStatusTimerRef.current != null) {
        clearTimeout(saveStatusTimerRef.current)
      }
      saveStatusTimerRef.current = setTimeout(() => {
        setSaveStatus("idle")
        saveStatusTimerRef.current = null
      }, 2000)
    },
    onError: (err) => {
      setSaveStatus("error")
      toast.error(getErrorMessage(err))
    },
  })

  const questionsMutation = useMutation({
    mutationFn: () => api.personaEnhanceQuestions(name, content),
    onSuccess: (resp) => setQuestions(resp.questions ?? []),
    onError: (err) => {
      setQuestions([])
      toast.error(`Failed to fetch enhance questions: ${getErrorMessage(err)}`)
    },
  })

  const enhanceMutation = useMutation({
    mutationFn: (answers: Answer[]) =>
      api.enhancePersona(name, {
        content,
        questions: questions ?? undefined,
        answers: answers.length > 0 ? answers : undefined,
      }),
    onSuccess: (result) => {
      setContent(result.content)
      setSaveStatus("idle")
      setQuestions(null)
      setModalOpen(false)
      toast.success(`Enhanced ${name.toUpperCase()}`)
    },
    onError: (err) => {
      setQuestions(null)
      setModalOpen(false)
      toast.error(`Enhance failed: ${getErrorMessage(err)}`)
    },
  })

  const startEnhance = () => {
    if (!content.trim()) return
    setQuestions(null)
    setModalOpen(true)
    questionsMutation.mutate()
  }

  const enhancing = enhanceMutation.isPending || questionsMutation.isPending

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
            variant="outline"
            onClick={startEnhance}
            disabled={enhancing || !content.trim()}
            className="border-border rounded-sm font-normal gap-1.5"
          >
            <Sparkles className={`h-3.5 w-3.5 ${enhancing ? "animate-spin" : ""}`} />
            {enhancing ? "Enhancing..." : "Enhance with AI"}
          </Button>
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

      <QuestionsModal
        open={modalOpen}
        onOpenChange={(next) => {
          if (!next && !enhanceMutation.isPending) {
            setModalOpen(false)
            setQuestions(null)
          }
        }}
        title={`Enhance ${name.toUpperCase()}`}
        description="Answer these so Claude can tailor the improvements."
        questions={questions}
        loading={questionsMutation.isPending}
        submitting={enhanceMutation.isPending}
        onSubmit={(answers) => enhanceMutation.mutate(answers)}
        onSkip={() => enhanceMutation.mutate([])}
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

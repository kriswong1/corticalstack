import { useState, useRef } from "react"
import { PageHeader } from "@/components/layout/page-header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { JobProgress } from "@/components/ingest/job-progress"
import { PreviewPanel } from "@/components/ingest/preview-panel"
import { useJobStream } from "@/hooks/use-job-stream"
import { api } from "@/lib/api"
import { FileText, Link, Upload, Mic } from "lucide-react"

export function IngestPage() {
  const [jobId, setJobId] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [confirmed, setConfirmed] = useState(false)
  const stream = useJobStream(jobId)

  async function submitJob(fn: () => Promise<{ job_id: string }>) {
    setSubmitting(true)
    setJobId(null)
    setConfirmed(false)
    try {
      const { job_id } = await fn()
      setJobId(job_id)
    } catch (err) {
      alert("Submit failed: " + (err instanceof Error ? err.message : String(err)))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <>
      <PageHeader
        title="Ingest"
        description="Submit text, files, or URLs for processing"
      />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <div>
          <Tabs defaultValue="text">
            <TabsList className="mb-4">
              <TabsTrigger value="text" className="gap-1.5">
                <FileText className="h-3.5 w-3.5" /> Text
              </TabsTrigger>
              <TabsTrigger value="url" className="gap-1.5">
                <Link className="h-3.5 w-3.5" /> URL
              </TabsTrigger>
              <TabsTrigger value="file" className="gap-1.5">
                <Upload className="h-3.5 w-3.5" /> File
              </TabsTrigger>
              <TabsTrigger value="audio" className="gap-1.5">
                <Mic className="h-3.5 w-3.5" /> Audio
              </TabsTrigger>
            </TabsList>

            <TabsContent value="text">
              <TextForm onSubmit={submitJob} submitting={submitting} />
            </TabsContent>
            <TabsContent value="url">
              <URLForm onSubmit={submitJob} submitting={submitting} />
            </TabsContent>
            <TabsContent value="file">
              <FileForm onSubmit={submitJob} submitting={submitting} />
            </TabsContent>
            <TabsContent value="audio">
              <FileForm onSubmit={submitJob} submitting={submitting} isAudio />
            </TabsContent>
          </Tabs>
        </div>

        <div className="space-y-4">
          {jobId && (
            <JobProgress
              jobId={jobId}
              status={stream.status}
              messages={stream.messages}
              notePath={stream.notePath}
              error={stream.error}
              done={stream.done}
            />
          )}

          {stream.preview &&
            stream.status === "awaiting_confirmation" &&
            !confirmed &&
            jobId && (
              <PreviewPanel
                preview={stream.preview}
                jobId={jobId}
                onConfirmed={() => setConfirmed(true)}
              />
            )}
        </div>
      </div>
    </>
  )
}

function TextForm({
  onSubmit,
  submitting,
}: {
  onSubmit: (fn: () => Promise<{ job_id: string }>) => void
  submitting: boolean
}) {
  const [text, setText] = useState("")
  const [title, setTitle] = useState("")

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (!text.trim()) return
        onSubmit(() => api.ingestText({ text, title }))
      }}
    >
      <div className="space-y-2">
        <Label className="text-[var(--stripe-label)] text-sm font-normal">
          Title (optional)
        </Label>
        <Input
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="Auto-generated if blank"
          className="border-border rounded-sm"
        />
      </div>
      <div className="space-y-2">
        <Label className="text-[var(--stripe-label)] text-sm font-normal">
          Content
        </Label>
        <Textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          rows={10}
          placeholder="Paste text to ingest..."
          className="border-border rounded-sm"
        />
      </div>
      <Button
        type="submit"
        disabled={submitting || !text.trim()}
        className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
      >
        {submitting ? "Submitting..." : "Ingest Text"}
      </Button>
    </form>
  )
}

function URLForm({
  onSubmit,
  submitting,
}: {
  onSubmit: (fn: () => Promise<{ job_id: string }>) => void
  submitting: boolean
}) {
  const [url, setUrl] = useState("")

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (!url.trim()) return
        onSubmit(() => api.ingestURL({ url }))
      }}
    >
      <div className="space-y-2">
        <Label className="text-[var(--stripe-label)] text-sm font-normal">
          URL
        </Label>
        <Input
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="https://..."
          type="url"
          className="border-border rounded-sm"
        />
      </div>
      <Button
        type="submit"
        disabled={submitting || !url.trim()}
        className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
      >
        {submitting ? "Submitting..." : "Ingest URL"}
      </Button>
    </form>
  )
}

function FileForm({
  onSubmit,
  submitting,
  isAudio,
}: {
  onSubmit: (fn: () => Promise<{ job_id: string }>) => void
  submitting: boolean
  isAudio?: boolean
}) {
  const inputRef = useRef<HTMLInputElement>(null)

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        const file = inputRef.current?.files?.[0]
        if (!file) return
        const fd = new FormData()
        fd.append("file", file)
        onSubmit(() => api.ingestFile(fd))
      }}
    >
      <div className="space-y-2">
        <Label className="text-[var(--stripe-label)] text-sm font-normal">
          {isAudio ? "Audio File" : "File"}
        </Label>
        <Input
          ref={inputRef}
          type="file"
          accept={isAudio ? "audio/*" : undefined}
          className="border-border rounded-sm"
        />
      </div>
      <Button
        type="submit"
        disabled={submitting}
        className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal"
      >
        {submitting ? "Uploading..." : isAudio ? "Ingest Audio" : "Ingest File"}
      </Button>
    </form>
  )
}

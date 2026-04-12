import { useEffect, useRef } from "react"
import { Badge } from "@/components/ui/badge"
import { ScrollArea } from "@/components/ui/scroll-area"
import type { JobStatus } from "@/types/api"

interface JobProgressProps {
  jobId: string
  status: JobStatus | null
  messages: string[]
  notePath: string | null
  error: string | null
  done: boolean
}

const statusColors: Record<string, string> = {
  pending: "bg-muted text-muted-foreground",
  transforming: "bg-secondary text-secondary-foreground",
  classifying: "bg-secondary text-secondary-foreground",
  awaiting_confirmation: "bg-primary/20 text-primary",
  extracting: "bg-secondary text-secondary-foreground",
  routing: "bg-secondary text-secondary-foreground",
  completed:
    "bg-[rgba(21,190,83,0.2)] text-[var(--stripe-success-text)] border-[rgba(21,190,83,0.4)]",
  failed: "bg-destructive/20 text-destructive border-destructive/40",
}

export function JobProgress({
  jobId,
  status,
  messages,
  notePath,
  error,
  done,
}: JobProgressProps) {
  const logEndRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: "smooth" })
  }, [messages.length])

  return (
    <div className="rounded-md border border-border p-4 shadow-stripe-ambient">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <span className="text-xs font-mono text-muted-foreground">
            {jobId}
          </span>
          {status && (
            <Badge
              className={`text-[10px] font-light rounded-sm px-1.5 py-px ${statusColors[status] ?? ""}`}
            >
              {status}
            </Badge>
          )}
        </div>
        {!done && status && (
          <span className="h-2 w-2 rounded-full bg-primary animate-pulse" />
        )}
      </div>

      <ScrollArea className="h-48 rounded-sm border border-border bg-muted/30 p-3">
        <ul className="space-y-1 font-mono text-xs text-muted-foreground">
          {messages.map((msg, i) => (
            <li key={i}>{msg}</li>
          ))}
        </ul>
        <div ref={logEndRef} />
      </ScrollArea>

      {error && (
        <p className="mt-2 text-sm text-destructive font-light">{error}</p>
      )}

      {notePath && (
        <div className="mt-3">
          <a
            href={`/library?note=${encodeURIComponent(notePath)}`}
            className="text-sm font-normal text-primary hover:underline"
          >
            View note: {notePath}
          </a>
        </div>
      )}
    </div>
  )
}

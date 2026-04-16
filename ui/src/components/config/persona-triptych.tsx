import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { api } from "@/lib/api"
import { CheckCircle2, Circle, Pencil, MessageSquare } from "lucide-react"

const personaDescriptions: Record<string, string> = {
  soul: "Extraction tone, structure conventions, domain focus, tagging philosophy",
  user: "Your role, timezone, platforms, PKM goals, what good output looks like",
  memory: "Active decisions, important vault notes, open questions you're tracking",
}

interface PersonaTriptychProps {
  onSetup: (name: string) => void
  onEdit: (name: string) => void
  editingName: string | null
}

export function PersonaTriptych({ onSetup, onEdit, editingName }: PersonaTriptychProps) {
  const { data } = useQuery({
    queryKey: ["persona-status"],
    queryFn: api.getPersonaStatus,
    staleTime: 10_000,
  })

  const personas = data?.personas ?? []

  return (
    <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
      {personas.map((p) => (
        <Card
          key={p.name}
          className={`rounded-md border-border shadow-stripe transition-colors ${
            editingName === p.name ? "ring-2 ring-primary/40" : ""
          }`}
        >
          <CardContent className="pt-5 pb-5 space-y-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                {p.configured ? (
                  <CheckCircle2 className="h-4 w-4 text-[var(--stripe-success-text,#15be53)]" />
                ) : (
                  <Circle className="h-4 w-4 text-muted-foreground" />
                )}
                <h3 className="text-[15px] font-semibold text-foreground uppercase">
                  {p.name}
                </h3>
              </div>
              <Badge
                className={`text-[10px] font-medium rounded-sm px-2 py-0.5 ${
                  p.configured
                    ? "bg-[rgba(21,190,83,0.15)] text-[var(--stripe-success-text,#15be53)] border-[rgba(21,190,83,0.4)]"
                    : "bg-muted text-muted-foreground border-border"
                }`}
              >
                {p.configured ? "Configured" : "Not Set Up"}
              </Badge>
            </div>

            <p className="text-xs text-muted-foreground">
              {personaDescriptions[p.name] ?? p.file}
            </p>

            {p.summary && (
              <p className="text-xs text-foreground/80 italic truncate">
                {p.summary}
              </p>
            )}

            <div className="flex items-center justify-between pt-1">
              <span className="text-[10px] font-mono text-muted-foreground">
                {p.char_count.toLocaleString()} / {p.budget.toLocaleString()}
              </span>
              <div className="flex items-center gap-1.5">
                {p.configured && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => onEdit(p.name)}
                    className="h-7 text-[11px] rounded-sm gap-1 px-2"
                  >
                    <Pencil className="h-3 w-3" />
                    Edit
                  </Button>
                )}
                <Button
                  size="sm"
                  onClick={() => onSetup(p.name)}
                  className="h-7 text-[11px] rounded-sm gap-1 px-2 bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground"
                >
                  <MessageSquare className="h-3 w-3" />
                  {p.configured ? "Reconfigure" : "Set up"}
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  )
}

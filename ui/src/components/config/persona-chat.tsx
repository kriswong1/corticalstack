import { useEffect, useRef, useState } from "react"
import { useMutation } from "@tanstack/react-query"
import { toast } from "sonner"
import { Card, CardContent } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { api, getErrorMessage } from "@/lib/api"
import { Loader2, Send, Square, MessageSquare } from "lucide-react"
import type { PersonaChatMessage } from "@/types/api"
import Markdown from "react-markdown"

interface PersonaChatProps {
  personaName: string
  onComplete: (result: string, sessionId: string) => void
  onCancel: () => void
}

export function PersonaChat({ personaName, onComplete, onCancel }: PersonaChatProps) {
  const [sessionId, setSessionId] = useState<string | null>(null)
  const [messages, setMessages] = useState<PersonaChatMessage[]>([])
  const [turn, setTurn] = useState(0)
  const [maxTurns, setMaxTurns] = useState(10)
  const [input, setInput] = useState("")
  const [isLoading, setIsLoading] = useState(false)
  const scrollRef = useRef<HTMLDivElement>(null)

  // Auto-scroll to bottom when messages change
  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [messages, isLoading])

  // Start the chat session on mount
  const startMutation = useMutation({
    mutationFn: () => api.startPersonaChat(personaName),
    onSuccess: (data) => {
      setSessionId(data.session_id)
      setMessages([data.message])
      setTurn(data.turn)
      setMaxTurns(data.max_turns)
    },
    onError: (err) => {
      toast.error(`Failed to start chat: ${getErrorMessage(err)}`)
      onCancel()
    },
  })

  useEffect(() => {
    startMutation.mutate()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const continueMutation = useMutation({
    mutationFn: (userInput: string) =>
      api.continuePersonaChat(personaName, {
        session_id: sessionId!,
        input: userInput,
      }),
    onSuccess: (data) => {
      setMessages((prev) => [...prev, data.message])
      setTurn(data.turn)
      setIsLoading(false)
      if (data.done && data.result) {
        onComplete(data.result, sessionId!)
      }
    },
    onError: (err) => {
      setIsLoading(false)
      toast.error(`Chat error: ${getErrorMessage(err)}`)
    },
  })

  const finishMutation = useMutation({
    mutationFn: () =>
      api.finishPersonaChat(personaName, { session_id: sessionId! }),
    onSuccess: (data) => {
      setIsLoading(false)
      if (data.content) {
        onComplete(data.content, sessionId!)
      }
    },
    onError: (err) => {
      setIsLoading(false)
      toast.error(`Finish error: ${getErrorMessage(err)}`)
    },
  })

  const sendMessage = (text: string) => {
    if (!text.trim() || !sessionId || isLoading) return
    setMessages((prev) => [...prev, { role: "user", content: text.trim() }])
    setInput("")
    setIsLoading(true)
    continueMutation.mutate(text.trim())
  }

  const handleDoneEarly = () => {
    if (!sessionId || isLoading) return
    setIsLoading(true)
    finishMutation.mutate()
  }

  const isStarting = startMutation.isPending

  return (
    <Card className="rounded-md border-border shadow-stripe">
      <CardContent className="p-0">
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-3 border-b border-border">
          <div className="flex items-center gap-2">
            <MessageSquare className="h-4 w-4 text-primary" />
            <span className="text-[13px] font-semibold text-foreground">
              Setting up: {personaName.toUpperCase()}
            </span>
          </div>
          <div className="flex items-center gap-3">
            <span className="text-[11px] text-muted-foreground tabular-nums">
              Turn {turn} of {maxTurns}
            </span>
            <Button
              size="sm"
              variant="outline"
              onClick={handleDoneEarly}
              disabled={isLoading || !sessionId || turn === 0}
              className="h-7 text-[11px] rounded-sm gap-1 px-2"
            >
              <Square className="h-3 w-3" />
              Done early
            </Button>
          </div>
        </div>

        {/* Messages */}
        <div
          ref={scrollRef}
          className="px-5 py-4 space-y-4 overflow-y-auto"
          style={{ maxHeight: "400px", minHeight: "200px" }}
        >
          {isStarting && (
            <div className="flex items-center gap-2 text-sm text-muted-foreground py-8 justify-center">
              <Loader2 className="h-4 w-4 animate-spin" />
              Starting conversation with Claude...
            </div>
          )}

          {messages.map((msg, i) => (
            <div
              key={i}
              className={`flex ${msg.role === "user" ? "justify-end" : "justify-start"}`}
            >
              <div
                className={`max-w-[80%] rounded-lg px-4 py-2.5 text-sm ${
                  msg.role === "user"
                    ? "bg-primary text-primary-foreground"
                    : "bg-muted/60 border border-border text-foreground"
                }`}
              >
                <div className="prose prose-sm max-w-none dark:prose-invert prose-p:my-1 prose-li:my-0.5">
                  <Markdown>{msg.content}</Markdown>
                </div>

                {msg.options && msg.options.length > 0 && (
                  <div className="flex flex-wrap gap-1.5 mt-2.5 pt-2 border-t border-border/50">
                    {msg.options.map((opt, j) => (
                      <button
                        key={j}
                        onClick={() => sendMessage(opt)}
                        disabled={isLoading}
                        className="text-[11px] px-2.5 py-1 rounded-full border border-border bg-background hover:bg-muted text-foreground transition-colors disabled:opacity-50"
                      >
                        {opt}
                      </button>
                    ))}
                  </div>
                )}
              </div>
            </div>
          ))}

          {isLoading && (
            <div className="flex justify-start">
              <div className="bg-muted/60 border border-border rounded-lg px-4 py-2.5">
                <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
              </div>
            </div>
          )}
        </div>

        {/* Input */}
        <div className="px-5 py-3 border-t border-border">
          <form
            className="flex items-center gap-2"
            onSubmit={(e) => {
              e.preventDefault()
              sendMessage(input)
            }}
          >
            <Input
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder="Type your answer..."
              disabled={isLoading || !sessionId}
              className="border-border rounded-sm text-sm"
            />
            <Button
              type="submit"
              size="sm"
              disabled={isLoading || !input.trim() || !sessionId}
              className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm h-9 px-3"
            >
              <Send className="h-4 w-4" />
            </Button>
          </form>
        </div>
      </CardContent>
    </Card>
  )
}

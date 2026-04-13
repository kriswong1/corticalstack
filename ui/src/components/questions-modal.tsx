import { useEffect, useState } from "react"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import type { Question, Answer } from "@/types/api"

interface QuestionsModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description?: string
  questions: Question[] | null
  loading: boolean
  submitting: boolean
  onSubmit: (answers: Answer[]) => void
  onSkip: () => void
}

export function QuestionsModal({
  open,
  onOpenChange,
  title,
  description,
  questions,
  loading,
  submitting,
  onSubmit,
  onSkip,
}: QuestionsModalProps) {
  const [values, setValues] = useState<Record<string, string>>({})

  useEffect(() => {
    if (!questions) {
      setValues({})
      return
    }
    const seed: Record<string, string> = {}
    for (const q of questions) {
      seed[q.id] = q.default ?? ""
    }
    setValues(seed)
  }, [questions])

  const handleSubmit = () => {
    const answers: Answer[] = (questions ?? []).map((q) => ({
      id: q.id,
      value: values[q.id] ?? "",
    }))
    onSubmit(answers)
  }

  const empty = !loading && questions && questions.length === 0

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description && <DialogDescription>{description}</DialogDescription>}
        </DialogHeader>

        <div className="max-h-[60vh] space-y-5 overflow-y-auto">
          {loading && (
            <p className="text-sm text-muted-foreground">
              Asking Claude what to clarify...
            </p>
          )}

          {empty && (
            <p className="text-sm text-muted-foreground">
              No clarifying questions — the context looks complete. You can
              generate now.
            </p>
          )}

          {!loading &&
            questions &&
            questions.length > 0 &&
            questions.map((q, i) => (
              <div key={q.id} className="space-y-2">
                <Label className="text-sm font-normal text-[var(--stripe-label)]">
                  <span className="mr-1 text-muted-foreground">{i + 1}.</span>
                  {q.prompt}
                </Label>

                {q.kind === "choice" && q.choices && q.choices.length > 0 ? (
                  <div className="flex flex-wrap gap-1.5">
                    {q.choices.map((choice) => {
                      const selected = values[q.id] === choice
                      return (
                        <button
                          key={choice}
                          type="button"
                          onClick={() =>
                            setValues((v) => ({ ...v, [q.id]: choice }))
                          }
                          className={`rounded-sm border px-2.5 py-1 text-xs transition ${
                            selected
                              ? "border-primary bg-primary/10 text-primary"
                              : "border-border bg-background text-foreground hover:border-primary/50"
                          }`}
                        >
                          {choice}
                        </button>
                      )
                    })}
                  </div>
                ) : (
                  <Textarea
                    value={values[q.id] ?? ""}
                    onChange={(e) =>
                      setValues((v) => ({ ...v, [q.id]: e.target.value }))
                    }
                    rows={2}
                    className="rounded-sm border-border text-sm"
                    placeholder="Your answer..."
                  />
                )}
              </div>
            ))}
        </div>

        <DialogFooter>
          <Button
            variant="ghost"
            onClick={onSkip}
            disabled={submitting || loading}
            className="rounded-sm font-normal"
          >
            Skip & generate
          </Button>
          <Button
            onClick={empty ? onSkip : handleSubmit}
            disabled={submitting || loading}
            className="rounded-sm bg-primary font-normal text-primary-foreground hover:bg-[var(--stripe-purple-hover)]"
          >
            {submitting
              ? "Generating..."
              : empty
                ? "Generate"
                : "Submit & generate"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

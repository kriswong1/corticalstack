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
  // Contract:
  //   onSubmit(answers) — user answered (may include empty string values
  //     if a question was left blank); parent should treat this as "the
  //     user engaged with the Q&A flow".
  //   onSkip() — user explicitly declined, OR the backend returned zero
  //     questions and the Generate button short-circuits. Parents that
  //     need to distinguish these two should push the branch up to their
  //     own callbacks (e.g. track `questions?.length === 0` before
  //     opening the modal).
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

  // Seed/merge `values` when the set of question IDs changes. Keying on
  // the joined-id string instead of the `questions` array reference
  // prevents the effect from firing when a parent does `setQuestions(
  // [...prev])` or otherwise produces an identity-new array with the
  // same content. We also merge with the previous values so answers the
  // user already typed survive incidental parent re-renders.
  const questionsKey = questions ? questions.map((q) => q.id).join("|") : ""
  useEffect(() => {
    if (!questions) {
      setValues({})
      return
    }
    setValues((prev) => {
      const seed: Record<string, string> = {}
      for (const q of questions) {
        seed[q.id] = prev[q.id] ?? q.default ?? ""
      }
      return seed
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [questionsKey])

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
            questions.map((q, i) => {
              const promptId = `qm-prompt-${q.id}`
              const textareaId = `qm-input-${q.id}`
              const isChoice =
                q.kind === "choice" && q.choices && q.choices.length > 0
              return (
                <div key={q.id} className="space-y-2">
                  {/*
                    The prompt is a styled heading rather than a real <Label>
                    because Radix's <Label> requires htmlFor / a nested input
                    to announce the association to screen readers. For the
                    choice branch there is no single input to point at, so we
                    use a plain <div id> and wire `aria-labelledby` on the
                    radiogroup below. For the textarea branch we use a real
                    <Label htmlFor>.
                  */}
                  {isChoice ? (
                    <div
                      id={promptId}
                      className="text-sm font-normal text-muted-foreground"
                    >
                      <span className="mr-1 text-muted-foreground">
                        {i + 1}.
                      </span>
                      {q.prompt}
                    </div>
                  ) : (
                    <Label
                      htmlFor={textareaId}
                      className="text-sm font-normal text-muted-foreground"
                    >
                      <span className="mr-1 text-muted-foreground">
                        {i + 1}.
                      </span>
                      {q.prompt}
                    </Label>
                  )}

                  {isChoice ? (
                    <div
                      role="radiogroup"
                      aria-labelledby={promptId}
                      className="flex flex-wrap gap-1.5"
                    >
                      {q.choices!.map((choice) => {
                        const selected = values[q.id] === choice
                        return (
                          <button
                            key={choice}
                            type="button"
                            role="radio"
                            aria-checked={selected}
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
                      id={textareaId}
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
              )
            })}
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

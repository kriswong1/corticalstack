import { useQuery } from "@tanstack/react-query"
import { api } from "@/lib/api"
import { Check } from "lucide-react"

export function OnboardingProgress() {
  const { data } = useQuery({
    queryKey: ["onboarding-status"],
    queryFn: api.getOnboardingStatus,
    staleTime: 30_000,
  })

  if (!data) return null

  return (
    <div className="flex items-center gap-4 rounded-lg border border-border bg-muted/30 px-5 py-3 mb-6">
      <span className="text-[13px] font-semibold text-foreground tabular-nums">
        {data.configured_count} of {data.total} configured
      </span>
      <div className="flex items-center gap-1.5">
        {data.items.map((item) => (
          <div
            key={item.id}
            className="flex items-center gap-1"
            title={`${item.label}: ${item.configured ? "Configured" : "Not configured"}`}
          >
            <div
              className={`h-2 w-8 rounded-full transition-colors ${
                item.configured
                  ? "bg-[var(--stripe-success-text,#15be53)]"
                  : "bg-border"
              }`}
            />
            {item.configured && (
              <Check className="h-3 w-3 text-[var(--stripe-success-text,#15be53)]" />
            )}
          </div>
        ))}
      </div>
      <div className="flex items-center gap-2 ml-auto">
        {data.items.map((item) => (
          <span
            key={item.id}
            className={`text-[10px] ${
              item.configured ? "text-muted-foreground" : "text-foreground font-medium"
            }`}
          >
            {item.label}
          </span>
        ))}
      </div>
    </div>
  )
}

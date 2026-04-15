import { useQuery } from "@tanstack/react-query"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Badge } from "@/components/ui/badge"
import { PageHeader } from "@/components/layout/page-header"
import { SkeletonPage } from "@/components/shared/skeleton-card"
import { api } from "@/lib/api"
import type { UsageInvocation, UsageModelTotals } from "@/types/api"

// Shared card chrome — matches the dashboard's hero card styling so the
// /usage page feels like part of the same surface, not a bolt-on tool.
const CARD_CLASS =
  "rounded-[14px] border-border shadow-stripe hover:shadow-stripe-elevated transition-shadow"

function formatNumber(n: number): string {
  return n.toLocaleString()
}

function formatCost(usd: number): string {
  if (usd >= 1) return `$${usd.toFixed(2)}`
  if (usd >= 0.01) return `$${usd.toFixed(4)}`
  return `$${usd.toFixed(6)}`
}

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

function formatTimestamp(iso: string): string {
  const d = new Date(iso)
  return d.toLocaleString([], {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  })
}

// Strip the "claude-" prefix and provider suffix for compact display.
function shortModel(model?: string): string {
  if (!model) return "—"
  return model.replace(/^claude-/, "")
}

function topModelByCalls(byModel: Record<string, UsageModelTotals>): string {
  let best = ""
  let bestCalls = -1
  for (const [name, totals] of Object.entries(byModel)) {
    if (totals.calls > bestCalls) {
      best = name
      bestCalls = totals.calls
    }
  }
  return best || "—"
}

export function UsagePage() {
  const {
    data: summary,
    isLoading: summaryLoading,
    error: summaryError,
  } = useQuery({
    queryKey: ["usage", "summary", "24h"],
    queryFn: () => api.getUsageSummary("24h"),
    staleTime: 60_000,
    refetchInterval: 60_000,
    refetchOnWindowFocus: false,
  })

  const {
    data: recent,
    isLoading: recentLoading,
    error: recentError,
  } = useQuery({
    queryKey: ["usage", "recent", 50],
    queryFn: () => api.getUsageRecent(50),
    staleTime: 60_000,
    refetchInterval: 60_000,
    refetchOnWindowFocus: false,
  })

  if (summaryLoading || recentLoading) return <SkeletonPage />

  if (summaryError || recentError || !summary || !recent) {
    return (
      <>
        <PageHeader title="Usage" description="Claude CLI token usage" />
        <Card className="rounded-md border-destructive/40 bg-destructive/5">
          <CardContent className="py-6">
            <p className="text-sm text-destructive">
              Could not load usage telemetry. The recorder may not be wired up,
              or no calls have been made yet.
            </p>
          </CardContent>
        </Card>
      </>
    )
  }

  const totalTokens =
    summary.total_input_tokens +
    summary.total_output_tokens +
    summary.total_cache_creation_tokens +
    summary.total_cache_read_tokens

  return (
    <>
      <PageHeader
        title="Usage"
        description="Claude CLI token usage · trailing 24h"
      />

      {/* Row 1: summary cards */}
      <div className="grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-4 mb-5">
        <SummaryCard label="Calls" value={formatNumber(summary.total_calls)} />
        <SummaryCard label="Cost (USD)" value={formatCost(summary.total_cost_usd)} />
        <SummaryCard label="Total tokens" value={formatNumber(totalTokens)} />
        <SummaryCard label="Top model" value={shortModel(topModelByCalls(summary.by_model))} />
      </div>

      {/* Row 2: token breakdown */}
      <Card className={`${CARD_CLASS} mb-5`}>
        <CardHeader className="pb-4">
          <CardTitle className="text-[15px] font-semibold tracking-tight text-foreground">
            Token breakdown · 24h
          </CardTitle>
          <p className="mt-0.5 text-xs text-muted-foreground">
            Per-bucket totals across every Claude call in the trailing window
          </p>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
            <TokenStat label="Input" value={summary.total_input_tokens} />
            <TokenStat label="Output" value={summary.total_output_tokens} />
            <TokenStat label="Cache create" value={summary.total_cache_creation_tokens} />
            <TokenStat label="Cache read" value={summary.total_cache_read_tokens} />
          </div>
        </CardContent>
      </Card>

      {/* Row 3: recent invocations */}
      <Card className={CARD_CLASS}>
        <CardHeader className="pb-4">
          <CardTitle className="text-[15px] font-semibold tracking-tight text-foreground">
            Recent invocations
          </CardTitle>
          <p className="mt-0.5 text-xs text-muted-foreground">
            Newest first · {recent.length} call{recent.length === 1 ? "" : "s"}
          </p>
        </CardHeader>
        <CardContent>
          {recent.length === 0 ? (
            <p className="py-8 text-center text-sm text-muted-foreground">
              No Claude calls recorded yet. Run an ingest to populate this view.
            </p>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Time</TableHead>
                    <TableHead>Model</TableHead>
                    <TableHead className="text-right">Input</TableHead>
                    <TableHead className="text-right">Output</TableHead>
                    <TableHead className="text-right">Cache R</TableHead>
                    <TableHead className="text-right">Cache W</TableHead>
                    <TableHead className="text-right">Cost</TableHead>
                    <TableHead className="text-right">Duration</TableHead>
                    <TableHead>Caller</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {recent.map((inv, i) => (
                    <InvocationRow key={`${inv.session_id ?? "no-sess"}-${i}`} inv={inv} />
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
    </>
  )
}

function SummaryCard({ label, value }: { label: string; value: string }) {
  return (
    <Card className={CARD_CLASS}>
      <CardContent className="py-5">
        <p className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
          {label}
        </p>
        <p className="mt-1.5 text-[24px] font-bold tracking-tight text-foreground tabular-nums">
          {value}
        </p>
      </CardContent>
    </Card>
  )
}

function TokenStat({ label, value }: { label: string; value: number }) {
  return (
    <div>
      <p className="text-[11px] font-medium text-muted-foreground uppercase tracking-wide">
        {label}
      </p>
      <p className="mt-1 text-[18px] font-semibold tracking-tight text-foreground tabular-nums">
        {formatNumber(value)}
      </p>
    </div>
  )
}

function InvocationRow({ inv }: { inv: UsageInvocation }) {
  const isError = !!inv.error
  return (
    <TableRow className={isError ? "bg-destructive/5" : undefined}>
      <TableCell className="text-xs text-muted-foreground tabular-nums whitespace-nowrap">
        {formatTimestamp(inv.timestamp)}
      </TableCell>
      <TableCell>
        <Badge variant="outline" className="font-mono text-[11px]">
          {shortModel(inv.model)}
        </Badge>
      </TableCell>
      <TableCell className="text-right tabular-nums text-xs">
        {formatNumber(inv.input_tokens)}
      </TableCell>
      <TableCell className="text-right tabular-nums text-xs">
        {formatNumber(inv.output_tokens)}
      </TableCell>
      <TableCell className="text-right tabular-nums text-xs">
        {formatNumber(inv.cache_read_tokens)}
      </TableCell>
      <TableCell className="text-right tabular-nums text-xs">
        {formatNumber(inv.cache_creation_tokens)}
      </TableCell>
      <TableCell className="text-right tabular-nums text-xs font-medium">
        {formatCost(inv.cost_usd)}
      </TableCell>
      <TableCell className="text-right tabular-nums text-xs text-muted-foreground">
        {formatDuration(inv.duration_ms)}
      </TableCell>
      <TableCell className="text-xs text-muted-foreground">
        {inv.caller_hint || (isError ? <span className="text-destructive">{inv.error}</span> : "—")}
      </TableCell>
    </TableRow>
  )
}

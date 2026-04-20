import { useMemo } from "react"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import {
  PIPELINE_ACCENT,
  colorFor,
  stageLabel,
  stageOrders,
  withAlpha,
} from "@/lib/pipeline-stages"

// PipelineStageCards renders the stage-distribution row at the top of
// a pipeline dashboard. Each card mirrors the StageNode visual from
// the item-pipeline page, but the circle contains the count of items
// currently in that stage instead of a check/circle icon. Cards are
// clickable to filter a table below; hover reveals the item titles in
// that stage.
//
// Reusable across every pipeline type (product, meeting, document,
// prototype) — the stage order, colors, and labels come from the
// shared `pipeline-stages` module, so this component stays generic.

export interface PipelineStageItem {
  id: string
  title: string
  stage: string
}

interface PipelineStageCardsProps {
  type: string
  items: PipelineStageItem[]
  selectedStage: string | null
  onSelectStage: (stage: string | null) => void
  /** Override the canonical stage order for types whose stages are
      data-driven (e.g. use cases grouped by primary actor). When
      omitted, falls back to stageOrders[type]. */
  stages?: string[]
  /** Override the label shown on each card. Defaults to stageLabel. */
  labelFor?: (stage: string) => string
  /** Override the color per stage. Defaults to colorFor(type, stage). */
  colorFor?: (stage: string) => string
  /** Override the accent color used for connectors + empty circles.
      Defaults to PIPELINE_ACCENT[type]. */
  accent?: string
}

export function PipelineStageCards({
  type,
  items,
  selectedStage,
  onSelectStage,
  stages,
  labelFor,
  colorFor: colorForProp,
  accent: accentProp,
}: PipelineStageCardsProps) {
  const accent = accentProp ?? PIPELINE_ACCENT[type] ?? "#8B8FA3"
  const resolveColor = (s: string): string =>
    colorForProp ? colorForProp(s) : colorFor(type, s)
  const resolveLabel = (s: string): string =>
    labelFor ? labelFor(s) : stageLabel(s)

  // Group items per stage in the canonical order so every stage gets
  // a card even when empty — keeps the row shape stable.
  const { order, itemsByStage } = useMemo(() => {
    const canonical = stages ?? stageOrders[type] ?? []
    const grouped = new Map<string, PipelineStageItem[]>()
    for (const s of canonical) grouped.set(s, [])
    for (const item of items) {
      const bucket = grouped.get(item.stage)
      if (bucket) bucket.push(item)
    }
    return { order: canonical, itemsByStage: grouped }
  }, [items, type, stages])

  if (order.length === 0) return null

  return (
    <div className="overflow-x-auto mb-5">
      <div className="flex items-start gap-0 min-w-max px-2 py-2">
        {order.map((stage, idx) => {
          const stageItems = itemsByStage.get(stage) ?? []
          const isSelected = selectedStage === stage
          const anySelected = selectedStage !== null
          return (
            <div key={stage} className="flex items-start">
              <StageCountNode
                label={resolveLabel(stage)}
                count={stageItems.length}
                titles={stageItems.map((i) => i.title)}
                color={resolveColor(stage)}
                accent={accent}
                isSelected={isSelected}
                isDimmed={anySelected && !isSelected}
                onClick={() =>
                  onSelectStage(isSelected ? null : stage)
                }
              />
              {idx < order.length - 1 && (
                <Connector color={accent} />
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

function StageCountNode({
  label,
  count,
  titles,
  color,
  accent,
  isSelected,
  isDimmed,
  onClick,
}: {
  label: string
  count: number
  titles: string[]
  color: string
  accent: string
  isSelected: boolean
  isDimmed: boolean
  onClick: () => void
}) {
  const hasItems = count > 0

  const node = (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "relative flex flex-col items-center gap-2 rounded-xl border-2 px-6 py-4 transition-all min-w-[130px]",
        "cursor-pointer hover:scale-[1.02] hover:shadow-md",
      )}
      style={{
        borderColor: isSelected ? color : withAlpha(color, 0.35),
        background: isSelected
          ? withAlpha(color, 0.12)
          : withAlpha(color, 0.05),
        opacity: isDimmed ? 0.55 : 1,
        boxShadow: isSelected
          ? `0 0 0 2px var(--background), 0 0 0 4px ${withAlpha(color, 0.5)}`
          : "none",
      }}
      aria-pressed={isSelected}
      aria-label={`${label} — ${count} item${count === 1 ? "" : "s"}`}
    >
      {/* Count circle — replaces the check/circle icon on the
          per-item StageNode with the number of items in this stage. */}
      <div
        className="flex items-center justify-center h-8 w-8 rounded-full transition-colors"
        style={{
          background: hasItems ? color : withAlpha("#8B8FA3", 0.12),
          border: hasItems ? "none" : `2px solid ${withAlpha(accent, 0.2)}`,
        }}
      >
        <span
          className={cn(
            "text-[13px] font-bold tabular-nums leading-none",
            hasItems ? "text-white" : "text-muted-foreground/60",
          )}
        >
          {count}
        </span>
      </div>

      <span className="text-[11px] font-bold tracking-[0.06em] uppercase text-foreground">
        {label}
      </span>

      <span className="text-[10px] text-muted-foreground/70 italic">
        {count === 0
          ? "Empty"
          : count === 1
            ? "1 item"
            : `${count} items`}
      </span>
    </button>
  )

  if (!hasItems) return node

  return (
    <Tooltip>
      <TooltipTrigger asChild>{node}</TooltipTrigger>
      <TooltipContent side="bottom" className="max-w-sm">
        <div className="flex flex-col gap-1">
          <span className="text-[10px] font-bold tracking-[0.06em] uppercase opacity-70">
            {label} — {count} item{count === 1 ? "" : "s"}
          </span>
          <ul className="flex flex-col gap-0.5">
            {titles.slice(0, 12).map((t, i) => (
              <li key={i} className="text-xs truncate">
                {t}
              </li>
            ))}
            {titles.length > 12 && (
              <li className="text-xs opacity-60 italic">
                +{titles.length - 12} more
              </li>
            )}
          </ul>
        </div>
      </TooltipContent>
    </Tooltip>
  )
}

// Static connector between stage cards. Matches the item-pipeline
// arrow shape but without the active-flow animation — the dashboard
// row is a filter bar, not a live advance flow.
function Connector({ color }: { color: string }) {
  return (
    <div className="flex items-center justify-center self-center px-1 pt-2">
      <svg
        width="56"
        height="20"
        viewBox="0 0 56 20"
        aria-hidden
        className="overflow-visible"
      >
        <line
          x1="0"
          y1="10"
          x2="46"
          y2="10"
          stroke={color}
          strokeOpacity={0.25}
          strokeWidth="2"
          strokeLinecap="round"
        />
        <polygon
          points="44,4 56,10 44,16"
          fill={color}
          fillOpacity={0.3}
        />
      </svg>
    </div>
  )
}

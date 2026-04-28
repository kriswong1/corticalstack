import type { ReactNode } from "react"
import { Link } from "react-router-dom"
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table"
import { Button } from "@/components/ui/button"
import { colorFor, routeFor, stageLabel } from "@/lib/pipeline-stages"

// PipelineItemsTable is the shared, extensible items table rendered
// below the stage cards on every pipeline dashboard. It always has
// the same three base columns (Title / Stage / Updated) so every
// pipeline feels consistent; callers can append 1–2 type-specific
// columns via `extraColumns` when a common field doesn't exist
// (e.g. "Source Thread" for prototype + PRD).
//
// Selection state is controlled by the parent so both the dashboard
// card page (which drives per-item usage fetching off selection) and
// future consumers can bind the checkboxes to their own logic.

export interface PipelineItemsTableItem {
  id: string
  title: string
  stage: string
  updated?: string
}

export interface PipelineExtraColumn<T extends PipelineItemsTableItem> {
  header: string
  cell: (item: T) => ReactNode
  headClassName?: string
  cellClassName?: string
}

export interface PipelineItemsTableProps<T extends PipelineItemsTableItem> {
  type: string
  items: T[]
  selected: Set<string>
  onToggleItem: (id: string) => void
  onToggleAll: () => void
  allSelected: boolean
  extraColumns?: PipelineExtraColumn<T>[]
  /** Path prefix for the View link; defaults to `/${type}`. */
  viewLinkPrefix?: string
  /** Override the row's View link target. */
  viewLinkFor?: (item: T) => string
  /**
   * When set, the View control renders as a button that calls this
   * callback instead of a navigation Link. Used by surfaces that show
   * an in-place preview dialog (e.g. PRDs) rather than routing to a
   * detail page. Takes precedence over viewLinkFor / viewLinkPrefix.
   */
  onViewItem?: (item: T) => void
  /** Override the stage-column color. Defaults to shared colorFor. */
  colorForStage?: (stage: string) => string
  /** Override the stage-column label. Defaults to shared stageLabel. */
  labelForStage?: (stage: string) => string
  emptyMessage?: string
}

function formatDate(iso?: string): string {
  if (!iso) return "-"
  return new Date(iso).toLocaleDateString()
}

export function PipelineItemsTable<T extends PipelineItemsTableItem>({
  type,
  items,
  selected,
  onToggleItem,
  onToggleAll,
  allSelected,
  extraColumns,
  viewLinkPrefix,
  viewLinkFor,
  onViewItem,
  colorForStage,
  labelForStage,
  emptyMessage = "No items in this pipeline.",
}: PipelineItemsTableProps<T>) {
  const resolveColor = (s: string) =>
    colorForStage ? colorForStage(s) : colorFor(type, s)
  const resolveLabel = (s: string) =>
    labelForStage ? labelForStage(s) : stageLabel(s)

  if (items.length === 0) {
    return (
      <div className="py-10 text-center text-sm text-muted-foreground">
        {emptyMessage}
      </div>
    )
  }

  const firstStage = items[0]?.stage ?? ""

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className="w-10 pl-4">
            <input
              type="checkbox"
              checked={allSelected}
              onChange={onToggleAll}
              className="h-4 w-4 rounded border-border"
              style={{ accentColor: resolveColor(firstStage) }}
              aria-label="Select all items"
            />
          </TableHead>
          <TableHead>Title</TableHead>
          <TableHead>Stage</TableHead>
          {extraColumns?.map((col) => (
            <TableHead key={col.header} className={col.headClassName}>
              {col.header}
            </TableHead>
          ))}
          <TableHead>Updated</TableHead>
          <TableHead className="w-16" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((item) => (
          <TableRow key={item.id}>
            <TableCell className="pl-4">
              <input
                type="checkbox"
                checked={selected.has(item.id)}
                onChange={() => onToggleItem(item.id)}
                className="h-4 w-4 rounded border-border"
                style={{ accentColor: resolveColor(item.stage) }}
                aria-label={`Select ${item.title}`}
              />
            </TableCell>
            <TableCell className="font-medium text-foreground max-w-[300px] truncate">
              {item.title}
            </TableCell>
            <TableCell>
              <span className="inline-flex items-center gap-1.5 text-[12px]">
                <span
                  className="inline-block h-1.5 w-1.5 rounded-full flex-shrink-0"
                  style={{ background: resolveColor(item.stage) }}
                />
                <span className="text-muted-foreground">
                  {resolveLabel(item.stage)}
                </span>
              </span>
            </TableCell>
            {extraColumns?.map((col) => (
              <TableCell key={col.header} className={col.cellClassName}>
                {col.cell(item)}
              </TableCell>
            ))}
            <TableCell className="text-muted-foreground text-[13px] tabular-nums">
              {formatDate(item.updated)}
            </TableCell>
            <TableCell>
              {onViewItem ? (
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-7 px-2 text-xs"
                  onClick={() => onViewItem(item)}
                >
                  View
                </Button>
              ) : (
                <Button asChild variant="ghost" size="sm" className="h-7 px-2 text-xs">
                  <Link
                    to={
                      viewLinkFor
                        ? viewLinkFor(item)
                        : viewLinkPrefix
                          ? `${viewLinkPrefix}/${item.id}`
                          : routeFor(type, item.id)
                    }
                  >
                    View
                  </Link>
                </Button>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

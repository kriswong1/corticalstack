import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { PageHeader } from "@/components/layout/page-header"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { api } from "@/lib/api"
import type { ActionStatus } from "@/types/api"
import { RefreshCw } from "lucide-react"

const allStatuses: ActionStatus[] = [
  "pending",
  "ack",
  "doing",
  "done",
  "deferred",
  "cancelled",
]

const statusColors: Record<string, string> = {
  pending: "bg-muted text-muted-foreground",
  ack: "bg-secondary text-secondary-foreground",
  doing: "bg-primary/20 text-primary",
  done: "bg-[rgba(21,190,83,0.2)] text-[var(--stripe-success-text)] border-[rgba(21,190,83,0.4)]",
  deferred: "bg-muted text-muted-foreground",
  cancelled: "bg-destructive/20 text-destructive",
}

export function ActionsPage() {
  const queryClient = useQueryClient()

  const { data: actions, isLoading } = useQuery({
    queryKey: ["actions"],
    queryFn: () => api.listActions(),
  })

  const { data: counts } = useQuery({
    queryKey: ["action-counts"],
    queryFn: api.getActionCounts,
  })

  const statusMutation = useMutation({
    mutationFn: ({ id, status }: { id: string; status: string }) =>
      api.setActionStatus(id, status),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["actions"] })
      queryClient.invalidateQueries({ queryKey: ["action-counts"] })
    },
  })

  const reconcileMutation = useMutation({
    mutationFn: api.reconcileActions,
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: ["actions"] })
      queryClient.invalidateQueries({ queryKey: ["action-counts"] })
      alert(
        `Reconciled: scanned ${result.scanned}, matched ${result.lines_matched}, updated ${result.updated}`,
      )
    },
  })

  return (
    <>
      <PageHeader title="Actions" description="Track action items across projects">
        <Button
          variant="outline"
          onClick={() => reconcileMutation.mutate()}
          disabled={reconcileMutation.isPending}
          className="border-border rounded-sm font-normal gap-1.5"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${reconcileMutation.isPending ? "animate-spin" : ""}`} />
          Reconcile
        </Button>
      </PageHeader>

      {counts && (
        <div className="flex flex-wrap gap-2 mb-4">
          {allStatuses.map((s) => (
            <Badge
              key={s}
              className={`text-[10px] font-light rounded-sm px-1.5 py-px ${statusColors[s] ?? ""}`}
            >
              {s}: {counts[s] ?? 0}
            </Badge>
          ))}
        </div>
      )}

      {isLoading ? (
        <p className="text-muted-foreground">Loading...</p>
      ) : (
        <div className="rounded-md border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Description</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal w-32">Status</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Source</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Projects</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {actions?.map((action) => (
                <TableRow key={action.id}>
                  <TableCell className="text-sm font-light">
                    {action.description}
                  </TableCell>
                  <TableCell>
                    <Select
                      value={action.status}
                      onValueChange={(value) =>
                        statusMutation.mutate({ id: action.id, status: value })
                      }
                    >
                      <SelectTrigger className="h-7 w-28 text-xs border-border rounded-sm">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {allStatuses.map((s) => (
                          <SelectItem key={s} value={s}>
                            {s}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </TableCell>
                  <TableCell className="text-xs font-light text-muted-foreground truncate max-w-[200px]">
                    {action.source_title || action.source_note}
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {action.project_ids?.map((pid) => (
                        <Badge
                          key={pid}
                          variant="outline"
                          className="text-[10px] font-normal rounded-sm px-1"
                        >
                          {pid}
                        </Badge>
                      ))}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {actions?.length === 0 && (
                <TableRow>
                  <TableCell colSpan={4} className="text-center text-sm text-muted-foreground py-8">
                    No actions tracked yet.
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      )}
    </>
  )
}

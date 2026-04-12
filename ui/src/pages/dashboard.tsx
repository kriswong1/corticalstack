import { useQuery } from "@tanstack/react-query"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { PageHeader } from "@/components/layout/page-header"
import { SkeletonPage } from "@/components/shared/skeleton-card"
import { api } from "@/lib/api"
import {
  FolderOpen,
  Workflow,
  HardDriveDownload,
  Plug,
} from "lucide-react"

export function DashboardPage() {
  const { data: status, isLoading } = useQuery({
    queryKey: ["status"],
    queryFn: api.getStatus,
    staleTime: 30_000,
  })

  if (isLoading) {
    return <SkeletonPage />
  }

  return (
    <>
      <PageHeader title="Dashboard" description="Local Obsidian ingest pipeline" />

      <div className="grid grid-cols-1 gap-5 md:grid-cols-2 lg:grid-cols-4">
        <Card className="rounded-md border-border shadow-stripe-elevated">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-normal text-muted-foreground">
              Vault
            </CardTitle>
            <FolderOpen className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <p className="text-lg font-light text-foreground truncate">
              {status?.vault_path ?? "—"}
            </p>
          </CardContent>
        </Card>

        <Card className="rounded-md border-border shadow-stripe-elevated">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-normal text-muted-foreground">
              Transformers
            </CardTitle>
            <Workflow className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-light text-foreground">
              {status?.transformers?.length ?? 0}
            </p>
            <div className="mt-2 flex flex-wrap gap-1">
              {status?.transformers?.map((t) => (
                <Badge
                  key={t}
                  variant="outline"
                  className="text-[11px] font-normal rounded-sm px-1.5"
                >
                  {t}
                </Badge>
              ))}
            </div>
          </CardContent>
        </Card>

        <Card className="rounded-md border-border shadow-stripe-elevated">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-normal text-muted-foreground">
              Destinations
            </CardTitle>
            <HardDriveDownload className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-light text-foreground">
              {status?.destinations?.length ?? 0}
            </p>
            <div className="mt-2 flex flex-wrap gap-1">
              {status?.destinations?.map((d) => (
                <Badge
                  key={d}
                  variant="outline"
                  className="text-[11px] font-normal rounded-sm px-1.5"
                >
                  {d}
                </Badge>
              ))}
            </div>
          </CardContent>
        </Card>

        <Card className="rounded-md border-border shadow-stripe-elevated">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-normal text-muted-foreground">
              Integrations
            </CardTitle>
            <Plug className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            {status?.integrations?.map((integ) => (
              <div key={integ.id} className="flex items-center justify-between py-1">
                <span className="text-sm font-light text-foreground">
                  {integ.name}
                </span>
                <Badge
                  className={
                    integ.healthy
                      ? "bg-[rgba(21,190,83,0.2)] text-[var(--stripe-success-text)] border-[rgba(21,190,83,0.4)] text-[10px] font-light rounded-sm px-1.5 py-px"
                      : integ.configured
                        ? "bg-destructive/20 text-destructive border-destructive/40 text-[10px] font-light rounded-sm px-1.5 py-px"
                        : "bg-muted text-muted-foreground text-[10px] font-light rounded-sm px-1.5 py-px"
                  }
                >
                  {integ.healthy ? "Healthy" : integ.configured ? "Error" : "Not configured"}
                </Badge>
              </div>
            ))}
            {(!status?.integrations || status.integrations.length === 0) && (
              <p className="text-sm font-light text-muted-foreground">None configured</p>
            )}
          </CardContent>
        </Card>
      </div>

      <div className="mt-8">
        <h2 className="text-[22px] font-light tracking-[-0.22px] text-foreground mb-3">
          Content Types
        </h2>
        <div className="flex gap-2">
          {status?.content_types?.map((ct) => (
            <Badge
              key={ct}
              className="bg-secondary text-secondary-foreground text-[11px] font-normal rounded-sm px-2 py-0.5"
            >
              {ct}
            </Badge>
          ))}
        </div>
      </div>
    </>
  )
}

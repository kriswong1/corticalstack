import { useQuery } from "@tanstack/react-query"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Separator } from "@/components/ui/separator"
import { PageHeader } from "@/components/layout/page-header"
import { api } from "@/lib/api"

export function ConfigPage() {
  const { data: status, isLoading } = useQuery({
    queryKey: ["status"],
    queryFn: api.getStatus,
  })

  if (isLoading) {
    return (
      <>
        <PageHeader title="Config" description="System configuration" />
        <p className="text-muted-foreground">Loading...</p>
      </>
    )
  }

  return (
    <>
      <PageHeader title="Config" description="System configuration" />

      <div className="space-y-6">
        <Card className="rounded-md border-border shadow-stripe">
          <CardHeader>
            <CardTitle className="text-[22px] font-light tracking-[-0.22px] text-foreground">
              Environment
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            <ConfigRow label="Vault Path" value={status?.vault_path ?? "—"} />
            <Separator />
            <ConfigRow label="Server Time" value={status?.server_time ?? "—"} />
          </CardContent>
        </Card>

        <Card className="rounded-md border-border shadow-stripe">
          <CardHeader>
            <CardTitle className="text-[22px] font-light tracking-[-0.22px] text-foreground">
              Integrations
            </CardTitle>
          </CardHeader>
          <CardContent>
            {status?.integrations?.map((integ) => (
              <div key={integ.id} className="flex items-center justify-between py-2">
                <div>
                  <span className="text-sm font-light text-foreground">{integ.name}</span>
                  <span className="ml-2 text-xs text-muted-foreground">({integ.id})</span>
                </div>
                <div className="flex items-center gap-2">
                  <Badge
                    variant="outline"
                    className="text-[10px] font-light rounded-sm px-1.5 py-px"
                  >
                    {integ.configured ? "Configured" : "Not configured"}
                  </Badge>
                  {integ.configured && (
                    <Badge
                      className={
                        integ.healthy
                          ? "bg-[rgba(21,190,83,0.2)] text-[var(--stripe-success-text)] border-[rgba(21,190,83,0.4)] text-[10px] font-light rounded-sm px-1.5 py-px"
                          : "bg-destructive/20 text-destructive border-destructive/40 text-[10px] font-light rounded-sm px-1.5 py-px"
                      }
                    >
                      {integ.healthy ? "Healthy" : "Error"}
                    </Badge>
                  )}
                </div>
              </div>
            ))}
            {(!status?.integrations || status.integrations.length === 0) && (
              <p className="text-sm font-light text-muted-foreground">No integrations registered</p>
            )}
          </CardContent>
        </Card>

        <Card className="rounded-md border-border shadow-stripe">
          <CardHeader>
            <CardTitle className="text-[22px] font-light tracking-[-0.22px] text-foreground">
              Pipeline
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <h3 className="text-sm font-normal text-[var(--stripe-label)] mb-2">
                Transformers
              </h3>
              <div className="flex flex-wrap gap-1.5">
                {status?.transformers?.map((t) => (
                  <Badge key={t} variant="outline" className="text-[11px] font-normal rounded-sm px-1.5">
                    {t}
                  </Badge>
                ))}
              </div>
            </div>
            <Separator />
            <div>
              <h3 className="text-sm font-normal text-[var(--stripe-label)] mb-2">
                Destinations
              </h3>
              <div className="flex flex-wrap gap-1.5">
                {status?.destinations?.map((d) => (
                  <Badge key={d} variant="outline" className="text-[11px] font-normal rounded-sm px-1.5">
                    {d}
                  </Badge>
                ))}
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </>
  )
}

function ConfigRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between">
      <span className="text-sm font-normal text-[var(--stripe-label)]">{label}</span>
      <span className="text-sm font-light text-foreground font-mono">{value}</span>
    </div>
  )
}

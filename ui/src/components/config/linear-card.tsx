import { Badge } from "@/components/ui/badge"
import { Card, CardContent } from "@/components/ui/card"
import { Clock } from "lucide-react"

export function LinearCard() {
  return (
    <Card className="rounded-md border-border shadow-stripe opacity-60">
      <CardContent className="pt-5 pb-5 space-y-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <Clock className="h-4 w-4 text-muted-foreground" />
            <h3 className="text-[15px] font-semibold text-muted-foreground">Linear</h3>
          </div>
          <Badge className="bg-muted text-muted-foreground border-border text-[10px] font-medium rounded-sm px-2 py-0.5">
            Coming Soon
          </Badge>
        </div>
        <p className="text-xs text-muted-foreground">
          Push PRDs and pitches to Linear as structured issues. Research spike in progress — integration
          ships in a future cycle.
        </p>
      </CardContent>
    </Card>
  )
}

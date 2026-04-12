import { useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { PageHeader } from "@/components/layout/page-header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { api } from "@/lib/api"
import { Plus } from "lucide-react"

const formats = ["screen-flow", "component-spec", "user-journey"]

export function PrototypesPage() {
  const queryClient = useQueryClient()
  const [showForm, setShowForm] = useState(false)
  const [title, setTitle] = useState("")
  const [sourcePaths, setSourcePaths] = useState("")
  const [format, setFormat] = useState("screen-flow")
  const [hints, setHints] = useState("")

  const { data: prototypes, isLoading } = useQuery({
    queryKey: ["prototypes"],
    queryFn: api.listPrototypes,
  })

  const createMutation = useMutation({
    mutationFn: () =>
      api.createPrototype({
        title,
        source_paths: sourcePaths.split("\n").map((s) => s.trim()).filter(Boolean),
        format,
        hints,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["prototypes"] })
      setShowForm(false)
      setTitle("")
      setSourcePaths("")
      setHints("")
    },
  })

  return (
    <>
      <PageHeader title="Prototypes" description="Design specs for external tools">
        <Button
          onClick={() => setShowForm(!showForm)}
          className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal gap-1.5"
        >
          <Plus className="h-4 w-4" /> New Prototype
        </Button>
      </PageHeader>

      {showForm && (
        <Card className="mb-6 rounded-md border-border shadow-stripe">
          <CardContent className="pt-6">
            <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); createMutation.mutate() }}>
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">Title</Label>
                  <Input value={title} onChange={(e) => setTitle(e.target.value)} className="border-border rounded-sm" />
                </div>
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">Format</Label>
                  <Select value={format} onValueChange={setFormat}>
                    <SelectTrigger className="border-border rounded-sm">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {formats.map((f) => (
                        <SelectItem key={f} value={f}>{f}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>
              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">Source Paths (one per line)</Label>
                <Textarea value={sourcePaths} onChange={(e) => setSourcePaths(e.target.value)} rows={3} className="border-border rounded-sm font-mono text-xs" />
              </div>
              <div className="space-y-2">
                <Label className="text-[var(--stripe-label)] text-sm font-normal">Hints</Label>
                <Input value={hints} onChange={(e) => setHints(e.target.value)} className="border-border rounded-sm" />
              </div>
              <Button type="submit" disabled={createMutation.isPending || !title.trim()} className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal">
                {createMutation.isPending ? "Generating..." : "Generate Prototype"}
              </Button>
            </form>
          </CardContent>
        </Card>
      )}

      {isLoading ? (
        <p className="text-muted-foreground">Loading...</p>
      ) : (
        <div className="rounded-md border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Title</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Format</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Status</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Created</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {prototypes?.map((p) => (
                <TableRow key={p.id}>
                  <TableCell className="text-sm font-light">{p.title}</TableCell>
                  <TableCell>
                    <Badge variant="outline" className="text-[10px] font-normal rounded-sm px-1.5">{p.format}</Badge>
                  </TableCell>
                  <TableCell>
                    <Badge className="text-[10px] font-light rounded-sm px-1.5 py-px bg-secondary text-secondary-foreground">{p.status}</Badge>
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground font-mono">
                    {new Date(p.created).toLocaleDateString()}
                  </TableCell>
                </TableRow>
              ))}
              {prototypes?.length === 0 && (
                <TableRow>
                  <TableCell colSpan={4} className="text-center text-sm text-muted-foreground py-8">No prototypes yet.</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      )}
    </>
  )
}

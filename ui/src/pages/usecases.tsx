import { useState } from "react"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { PageHeader } from "@/components/layout/page-header"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Card, CardContent } from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { api } from "@/lib/api"

export function UseCasesPage() {
  const queryClient = useQueryClient()
  const [sourcePath, setSourcePath] = useState("")
  const [docHint, setDocHint] = useState("")
  const [description, setDescription] = useState("")
  const [actorsHint, setActorsHint] = useState("")

  const { data: useCases, isLoading } = useQuery({
    queryKey: ["usecases"],
    queryFn: api.listUseCases,
  })

  const fromDocMutation = useMutation({
    mutationFn: () => api.generateFromDoc({ source_path: sourcePath, hint: docHint }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["usecases"] })
      setSourcePath("")
      setDocHint("")
    },
  })

  const fromTextMutation = useMutation({
    mutationFn: () => api.generateFromText({ description, actors_hint: actorsHint }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["usecases"] })
      setDescription("")
      setActorsHint("")
    },
  })

  return (
    <>
      <PageHeader title="Use Cases" description="Generated use case specifications" />

      <Card className="mb-6 rounded-md border-border shadow-stripe">
        <CardContent className="pt-6">
          <Tabs defaultValue="from-text">
            <TabsList className="mb-4">
              <TabsTrigger value="from-text">From Text</TabsTrigger>
              <TabsTrigger value="from-doc">From Document</TabsTrigger>
            </TabsList>
            <TabsContent value="from-text">
              <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); fromTextMutation.mutate() }}>
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">Description</Label>
                  <Textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={3} className="border-border rounded-sm" />
                </div>
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">Actors Hint</Label>
                  <Input value={actorsHint} onChange={(e) => setActorsHint(e.target.value)} className="border-border rounded-sm" />
                </div>
                <Button type="submit" disabled={fromTextMutation.isPending || !description.trim()} className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal">
                  {fromTextMutation.isPending ? "Generating..." : "Generate"}
                </Button>
              </form>
            </TabsContent>
            <TabsContent value="from-doc">
              <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); fromDocMutation.mutate() }}>
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">Source Path</Label>
                  <Input value={sourcePath} onChange={(e) => setSourcePath(e.target.value)} placeholder="notes/..." className="border-border rounded-sm" />
                </div>
                <div className="space-y-2">
                  <Label className="text-[var(--stripe-label)] text-sm font-normal">Hint</Label>
                  <Input value={docHint} onChange={(e) => setDocHint(e.target.value)} className="border-border rounded-sm" />
                </div>
                <Button type="submit" disabled={fromDocMutation.isPending || !sourcePath.trim()} className="bg-primary hover:bg-[var(--stripe-purple-hover)] text-primary-foreground rounded-sm font-normal">
                  {fromDocMutation.isPending ? "Generating..." : "Generate"}
                </Button>
              </form>
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>

      {isLoading ? (
        <p className="text-muted-foreground">Loading...</p>
      ) : (
        <div className="rounded-md border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Title</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Actors</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Steps</TableHead>
                <TableHead className="text-[var(--stripe-label)] text-[13px] font-normal">Tags</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {useCases?.map((uc) => (
                <TableRow key={uc.id}>
                  <TableCell className="text-sm font-light">{uc.title}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">{uc.actors?.join(", ")}</TableCell>
                  <TableCell className="text-xs text-muted-foreground font-[feature-settings:'tnum']">{uc.main_flow?.length ?? 0}</TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {uc.tags?.map((tag) => (
                        <Badge key={tag} variant="outline" className="text-[10px] font-normal rounded-sm px-1">{tag}</Badge>
                      ))}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
              {useCases?.length === 0 && (
                <TableRow>
                  <TableCell colSpan={4} className="text-center text-sm text-muted-foreground py-8">No use cases yet.</TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      )}
    </>
  )
}

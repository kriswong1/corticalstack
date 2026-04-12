import { useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { useSearchParams } from "react-router-dom"
import { PageHeader } from "@/components/layout/page-header"
import { ScrollArea } from "@/components/ui/scroll-area"
import { api } from "@/lib/api"
import type { VaultTreeNode } from "@/types/api"
import { ChevronRight, ChevronDown, File, Folder } from "lucide-react"

export function LibraryPage() {
  const [searchParams] = useSearchParams()
  const initialNote = searchParams.get("note")
  const [selectedPath, setSelectedPath] = useState<string | null>(initialNote)

  const { data: tree } = useQuery({
    queryKey: ["vault-tree"],
    queryFn: api.getVaultTree,
  })

  const { data: fileContent } = useQuery({
    queryKey: ["vault-file", selectedPath],
    queryFn: () => api.getVaultFile(selectedPath!),
    enabled: !!selectedPath,
  })

  return (
    <>
      <PageHeader title="Library" description="Browse vault contents" />

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[320px_1fr] h-[calc(100vh-220px)]">
        <ScrollArea className="rounded-md border border-border p-3">
          {tree ? (
            <TreeNode node={tree} onSelect={setSelectedPath} selectedPath={selectedPath} depth={0} />
          ) : (
            <p className="text-sm text-muted-foreground">Loading...</p>
          )}
        </ScrollArea>

        <ScrollArea className="rounded-md border border-border p-4">
          {selectedPath ? (
            <pre className="whitespace-pre-wrap font-mono text-xs text-foreground leading-relaxed">
              {fileContent ?? "Loading..."}
            </pre>
          ) : (
            <p className="text-sm text-muted-foreground">
              Select a file to preview
            </p>
          )}
        </ScrollArea>
      </div>
    </>
  )
}

function TreeNode({
  node,
  onSelect,
  selectedPath,
  depth,
}: {
  node: VaultTreeNode
  onSelect: (path: string) => void
  selectedPath: string | null
  depth: number
}) {
  const [open, setOpen] = useState(depth < 1)

  if (node.is_dir) {
    return (
      <div>
        <button
          onClick={() => setOpen(!open)}
          className="flex w-full items-center gap-1 rounded-sm px-1 py-0.5 text-sm font-light text-foreground hover:bg-accent"
          style={{ paddingLeft: `${depth * 12 + 4}px` }}
        >
          {open ? (
            <ChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
          ) : (
            <ChevronRight className="h-3.5 w-3.5 text-muted-foreground" />
          )}
          <Folder className="h-3.5 w-3.5 text-primary/70" />
          <span className="truncate">{node.name}</span>
        </button>
        {open &&
          node.children?.map((child) => (
            <TreeNode
              key={child.path}
              node={child}
              onSelect={onSelect}
              selectedPath={selectedPath}
              depth={depth + 1}
            />
          ))}
      </div>
    )
  }

  return (
    <button
      onClick={() => onSelect(node.path)}
      className={`flex w-full items-center gap-1 rounded-sm px-1 py-0.5 text-sm font-light transition-colors ${
        selectedPath === node.path
          ? "bg-secondary text-secondary-foreground"
          : "text-foreground hover:bg-accent"
      }`}
      style={{ paddingLeft: `${depth * 12 + 4}px` }}
    >
      <File className="h-3.5 w-3.5 text-muted-foreground" />
      <span className="truncate">{node.name}</span>
    </button>
  )
}

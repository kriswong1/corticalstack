import { useState, useMemo } from "react"
import { useQuery } from "@tanstack/react-query"
import { useSearchParams } from "react-router-dom"
import { PageHeader } from "@/components/layout/page-header"
import { ScrollArea } from "@/components/ui/scroll-area"
import { api } from "@/lib/api"
import type { VaultTreeNode } from "@/types/api"
import { ChevronRight, ChevronDown, File, Folder, X } from "lucide-react"

// filterTree returns a pruned copy of the vault tree showing only nodes
// that match the active date/type filters from the URL. A directory is
// kept iff any descendant leaf file matches. Leaves match when: (a) the
// file is under the requested type folder (or no type filter), AND (b)
// the filename starts with the requested date prefix (or no date filter).
// Returns null when the whole branch is filtered out.
//
// Root-level files: a file directly at the vault root (e.g. README.md)
// has no "type folder" parent, so the `typeFilter` clause would always
// exclude it when a type filter is active. We treat root-level files as
// matching iff the type filter is literally `"root"`; otherwise they
// pass through as "no type" and get hidden (since the user asked for a
// specific type). The dashboard deep-links never emit `type=root` today
// so this preserves existing behavior while documenting the edge case.
function filterTree(
  node: VaultTreeNode,
  dateFilter: string | null,
  typeFilter: string | null,
  parentFolder: string | null,
): VaultTreeNode | null {
  if (!node.is_dir) {
    // Root-level files (parentFolder === null) match a literal
    // "root" type filter, otherwise fall through to the standard
    // "parent folder must equal type filter" check.
    if (typeFilter) {
      if (parentFolder === null) {
        if (typeFilter !== "root") return null
      } else if (parentFolder !== typeFilter) {
        return null
      }
    }
    // Date filter: filenames follow "YYYY-MM-DD_slug.md" convention so a
    // simple prefix check is enough. Notes without a date-prefixed name
    // are hidden under a date filter.
    if (dateFilter && !node.name.startsWith(dateFilter)) return null
    return node
  }

  // Directories: recurse. A directory only survives if it contains at
  // least one surviving leaf — empty folders under an active filter are
  // pruned so the tree shows the user exactly what matched.
  const childFolder = node.path === "" ? null : node.name
  const kept = (node.children ?? [])
    .map((c) => filterTree(c, dateFilter, typeFilter, childFolder))
    .filter((c): c is VaultTreeNode => c !== null)

  if (kept.length === 0) {
    // Keep the root dir even when empty so we always have a container.
    if (node.path === "") {
      return { ...node, children: [] }
    }
    return null
  }
  return { ...node, children: kept }
}

export function LibraryPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const selectedPath = searchParams.get("note")
  const dateFilter = searchParams.get("date")
  const typeFilter = searchParams.get("type")
  const hasFilter = !!(dateFilter || typeFilter)

  const selectPath = (path: string) => {
    const next = new URLSearchParams(searchParams)
    next.set("note", path)
    setSearchParams(next, { replace: false })
  }

  const clearFilters = () => {
    const next = new URLSearchParams(searchParams)
    next.delete("date")
    next.delete("type")
    setSearchParams(next, { replace: true })
  }

  const { data: tree } = useQuery({
    queryKey: ["vault-tree"],
    queryFn: api.getVaultTree,
  })

  const { data: fileContent } = useQuery({
    queryKey: ["vault-file", selectedPath],
    queryFn: () => api.getVaultFile(selectedPath!),
    enabled: !!selectedPath,
  })

  const filteredTree = useMemo(() => {
    if (!tree) return null
    if (!hasFilter) return tree
    return filterTree(tree, dateFilter, typeFilter, null)
  }, [tree, dateFilter, typeFilter, hasFilter])

  return (
    <>
      <PageHeader title="Library" description="Browse vault contents" />

      {hasFilter && (
        <div className="mb-3 flex items-center gap-2 rounded-sm border border-border bg-muted/30 px-3 py-2">
          <span className="text-xs text-muted-foreground">Filtered:</span>
          {typeFilter && (
            <span className="rounded-sm bg-secondary px-1.5 py-0.5 text-[11px] font-normal text-secondary-foreground">
              type: {typeFilter}
            </span>
          )}
          {dateFilter && (
            <span className="rounded-sm bg-secondary px-1.5 py-0.5 text-[11px] font-normal text-secondary-foreground">
              date: {dateFilter}
            </span>
          )}
          <button
            onClick={clearFilters}
            className="ml-auto inline-flex items-center gap-1 text-xs text-primary hover:underline"
          >
            <X className="h-3 w-3" /> Clear
          </button>
        </div>
      )}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[320px_1fr] h-[calc(100vh-220px)]">
        <ScrollArea className="rounded-md border border-border p-3">
          {filteredTree ? (
            <TreeNode node={filteredTree} onSelect={selectPath} selectedPath={selectedPath} depth={0} />
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
  // Auto-expand directories by default when filters are narrowing the
  // tree — the user is already arriving here from a deep link, hiding
  // the matches behind a collapsed folder wastes the click.
  const [open, setOpen] = useState(depth < 2)

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

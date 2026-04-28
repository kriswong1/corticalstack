// Shared pipeline-stage constants and helpers used by both the
// dashboard card page (aggregate count cards) and the item pipeline
// page (per-item stage flow). Keeping this in one place ensures the
// two surfaces always show the same stage order, colors, and labels.

// pipelineRoutePrefix maps an internal type key to its URL prefix.
// Canonical type keys are singular ("product", "meeting", ...) but
// URLs are pluralized where it reads more naturally. All internal
// links should build paths off this map rather than hard-coding them.
export const pipelineRoutePrefix: Record<string, string> = {
  product: "/product",
  meeting: "/meetings",
  document: "/documents",
  prototype: "/prototypes",
  prd: "/prds",
  usecase: "/usecases",
}

export function routeFor(type: string, id?: string): string {
  const prefix = pipelineRoutePrefix[type] ?? `/${type}`
  return id ? `${prefix}/${id}` : prefix
}

export const stageOrders: Record<string, string[]> = {
  product: ["idea", "frame", "shape", "breadboard", "pitch"],
  // Meetings can enter at either Audio (file dropped or uploaded,
  // awaiting Deepgram) or Transcript (text supplied directly), then
  // progress through Note once Claude extracts the summary.
  meeting: ["audio", "transcript", "note"],
  document: ["input", "note"],
  prototype: ["breadboard", "in_progress", "final"],
  // PRDs use `status` as the pipeline axis since they don't have a
  // stage field. Order follows the natural doc lifecycle.
  prd: ["draft", "review", "approved", "shipped", "archived"],
}

export const PIPELINE_ACCENT: Record<string, string> = {
  product: "#9B8AFF",
  meeting: "#47B5E8",
  document: "#48D597",
  prototype: "#E8C547",
  prd: "#E85B9B",
}

export const stageColors: Record<string, Record<string, string>> = {
  product: {
    idea: "#8B8FA3",
    frame: "#47B5E8",
    shape: "#9B8AFF",
    breadboard: "#E85B9B",
    pitch: "#48D597",
  },
  meeting: {
    audio: "#E8C547",
    transcript: "#47B5E8",
    note: "#48D597",
  },
  document: {
    input: "#8B8FA3",
    note: "#48D597",
  },
  prototype: {
    breadboard: "#E85B9B",
    in_progress: "#E8C547",
    final: "#48D597",
  },
  prd: {
    draft: "#8B8FA3",
    review: "#E8C547",
    approved: "#9B8AFF",
    shipped: "#48D597",
    archived: "#6B7280",
  },
}

export const stageLabels: Record<string, string> = {
  idea: "Idea",
  frame: "Frame",
  shape: "Shape",
  breadboard: "Breadboard",
  pitch: "Pitch",
  audio: "Audio",
  transcript: "Transcript",
  note: "Note",
  input: "Input",
  in_progress: "In Progress",
  final: "Final",
  draft: "Draft",
  review: "Review",
  approved: "Approved",
  shipped: "Shipped",
  archived: "Archived",
}

export function colorFor(type: string, stage: string): string {
  return stageColors[type]?.[stage] ?? PIPELINE_ACCENT[type] ?? "#8B8FA3"
}

export function withAlpha(hex: string, alpha: number): string {
  const r = parseInt(hex.slice(1, 3), 16)
  const g = parseInt(hex.slice(3, 5), 16)
  const b = parseInt(hex.slice(5, 7), 16)
  return `rgba(${r}, ${g}, ${b}, ${alpha})`
}

export function stageLabel(s: string): string {
  return stageLabels[s] ?? s.charAt(0).toUpperCase() + s.slice(1)
}

export function normalizeStage(type: string, stage: string): string {
  if (type === "product" && stage === "raw") return "idea"
  return stage
}

// TypeScript types matching Go struct JSON tags

// --- Jobs ---

export type JobStatus =
  | "pending"
  | "transforming"
  | "classifying"
  | "awaiting_confirmation"
  | "extracting"
  | "routing"
  | "completed"
  | "failed"

export interface PreviewResult {
  intention: string
  confidence: number
  summary: string
  suggested_title?: string
  suggested_project_ids?: string[]
  suggested_tags?: string[]
  reasoning?: string
}

export interface Job {
  id: string
  label: string
  status: JobStatus
  created_at: string
  started_at?: string
  ended_at?: string
  error?: string
  note_path?: string
  messages?: string[]
  preview?: PreviewResult
  transformer?: string
}

export interface ConfirmPayload {
  intention: string
  project_ids: string[]
  why: string
  title: string
}

export interface JobEvent {
  job_id: string
  status: JobStatus
  message: string
}

// --- Actions ---

// CurrentActionStatus is the set of statuses the UI actively shows in
// dropdowns and filter chips. Everything in this union has a color and
// label defined in actions.tsx.
export type CurrentActionStatus =
  | "inbox"
  | "next"
  | "waiting"
  | "doing"
  | "someday"
  | "deferred"
  | "done"
  | "cancelled"

// LegacyActionStatus covers values still accepted by the API for
// migration from older data, but not surfaced in the UI. Keep them in
// the main ActionStatus union so incoming API responses still type-check.
export type LegacyActionStatus = "pending" | "ack"

export type ActionStatus = CurrentActionStatus | LegacyActionStatus

export type ActionPriority = "p1" | "p2" | "p3"

export type ActionEffort = "xs" | "s" | "m" | "l" | "xl"

export interface Action {
  id: string
  title?: string
  description: string
  owner: string
  deadline?: string
  status: ActionStatus
  priority?: ActionPriority
  effort?: ActionEffort
  context?: string
  source_note: string
  source_title?: string
  project_ids?: string[]
  created: string
  updated: string
}

export interface ReconcileResult {
  scanned: number
  lines_matched: number
  unique_actions: number
  updated: number
}

// --- Projects ---

export type ProjectStatus = "active" | "paused" | "archived"

export interface Project {
  id: string
  name: string
  status: ProjectStatus
  description?: string
  tags?: string[]
  created: string
}

export interface CreateProjectRequest {
  name: string
  description?: string
  tags?: string[]
}

// --- Vault ---

export interface VaultTreeNode {
  name: string
  path: string
  is_dir: boolean
  children?: VaultTreeNode[]
}

// --- ShapeUp ---

export type ShapeUpStage =
  | "raw"
  | "frame"
  | "shape"
  | "breadboard"
  | "pitch"

export interface Artifact {
  id: string
  stage: ShapeUpStage
  thread: string
  parent_id?: string
  title: string
  path: string
  projects?: string[]
  appetite?: string
  status: string
  created: string
}

export interface ShapeUpThread {
  id: string
  title: string
  projects?: string[]
  current_stage: ShapeUpStage
  artifacts: Artifact[]
}

export interface CreateIdeaRequest {
  title: string
  content: string
  project_ids?: string[]
}

export interface AdvanceRequest {
  target_stage: string
  hints?: string
  questions?: Question[]
  answers?: Answer[]
}

// --- Use Cases ---

export interface AltFlow {
  name: string
  at_step: number
  flow: string[]
}

export interface SourceRef {
  type: string
  path?: string
}

export interface UseCase {
  id: string
  title: string
  actors: string[]
  secondary_actors?: string[]
  preconditions?: string[]
  main_flow: string[]
  alternative_flows?: AltFlow[]
  postconditions?: string[]
  business_rules?: string[]
  non_functional?: string[]
  source?: SourceRef[]
  tags?: string[]
  projects?: string[]
  path?: string
  created: string
}

export interface FromDocRequest {
  source_path: string
  hint?: string
  project_ids?: string[]
  questions?: Question[]
  answers?: Answer[]
}

export interface FromTextRequest {
  description: string
  actors_hint?: string
  project_ids?: string[]
  questions?: Question[]
  answers?: Answer[]
}

export interface UseCaseFromDocQuestionsRequest {
  source_path: string
  hint?: string
}

export interface UseCaseFromTextQuestionsRequest {
  description: string
  actors_hint?: string
}

export interface GenerateUseCasesResponse {
  created: UseCase[]
  errors?: string[]
}

// --- Prototypes ---

export interface Prototype {
  id: string
  title: string
  format: string
  source_refs?: string[]
  source_thread?: string
  projects?: string[]
  status: string
  spec?: string
  has_html?: boolean
  folder_path?: string
  created: string
}

export interface CreatePrototypeRequest {
  title: string
  source_paths: string[]
  format: string
  hints?: string
  project_ids?: string[]
  source_thread?: string
  questions?: Question[]
  answers?: Answer[]
}

export interface PrototypeQuestionsRequest {
  title: string
  format: string
  source_paths: string[]
  hints?: string
}

// --- PRDs ---

export type PRDStatus =
  | "draft"
  | "review"
  | "approved"
  | "shipped"
  | "archived"

export interface PRD {
  id: string
  version: number
  status: PRDStatus
  title: string
  source_pitch: string
  source_thread?: string
  context_refs?: string[]
  projects?: string[]
  open_questions_count: number
  path?: string
  created: string
}

export interface CreatePRDRequest {
  pitch_path: string
  extra_context_tags?: string[]
  extra_context_paths?: string[]
  project_ids?: string[]
  questions?: Question[]
  answers?: Answer[]
}

export interface PRDQuestionsRequest {
  pitch_path: string
  extra_context_tags?: string[]
  extra_context_paths?: string[]
  project_ids?: string[]
}

// --- Persona ---

export interface PersonaResponse {
  name: string
  file: string
  content: string
  budget: number
}

export interface PersonaEnhanceRequest {
  content: string
  user_context?: string
  questions?: Question[]
  answers?: Answer[]
}

// --- Dashboard operating view ---

export interface IngestBucket {
  type: string
  count: number
}

export interface IngestDay {
  date: string // YYYY-MM-DD
  buckets: IngestBucket[]
  count: number
}

export interface IngestWidget {
  days: IngestDay[] // exactly 30 entries, oldest → newest
  types: string[] // alphabetical legend
  total: number
}

export interface ActionsWidget {
  open: number
  in_progress: number
  blocked: number
  done: number
  stalled: number
  total: number
}

export interface ProjectTouch {
  id: string
  name: string
  last_touched: string
}

export interface ProjectsWidget {
  active: number
  top: ProjectTouch[]
}

export interface PipelineStage {
  stage: string
  count: number
  stalled: number
}

export interface PipelineWidget {
  stages: PipelineStage[]
  total: number
  stalled_total: number
}

export interface DashboardSnapshot {
  ingest_activity: IngestWidget
  actions: ActionsWidget
  active_projects: ProjectsWidget
  product_pipeline: PipelineWidget
  computed_at: string
  stale: boolean
  stale_attempt_at?: string
  stale_reason?: string
  all_empty: boolean
}

// --- Questions (shared Q&A pattern) ---

export interface Question {
  id: string
  prompt: string
  kind: "text" | "choice"
  choices?: string[]
  default?: string
}

export interface Answer {
  id: string
  value: string
}

export interface QuestionsResponse {
  questions: Question[]
}

// --- Status ---

export interface IntegrationStatus {
  id: string
  name: string
  configured: boolean
  healthy: boolean
  error?: string
}

export interface StatusResponse {
  ok: boolean
  vault_path: string
  transformers: string[]
  destinations: string[]
  integrations: IntegrationStatus[]
  server_time: string
  content_types: string[]
}

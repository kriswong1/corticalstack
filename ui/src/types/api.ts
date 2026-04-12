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

export type ActionStatus =
  | "pending"
  | "ack"
  | "doing"
  | "done"
  | "deferred"
  | "cancelled"

export interface Action {
  id: string
  description: string
  owner: string
  deadline?: string
  status: ActionStatus
  source_note: string
  source_title?: string
  project_ids?: string[]
  created: string
  updated: string
}

export interface ReconcileResult {
  scanned: number
  lines_matched: number
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
}

export interface FromTextRequest {
  description: string
  actors_hint?: string
  project_ids?: string[]
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
}

// --- Persona ---

export interface PersonaResponse {
  name: string
  file: string
  content: string
  budget: number
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

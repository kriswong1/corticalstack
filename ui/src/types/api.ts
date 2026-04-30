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
  /**
   * Phase 4: when no active project fits but the content clearly belongs
   * to a *new* project, Claude returns the proposed name here. The
   * preview panel surfaces a "Create new project «foo»?" affordance —
   * confirming creates the project explicitly. Ingest no longer
   * silently auto-creates from arbitrary frontmatter strings.
   */
  proposed_project_name?: string
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
  // MS-To-Do-flavored fields. JSON-only — these don't round-trip
  // through the Obsidian markdown line.
  my_day?: boolean
  starred?: boolean
  parent_id?: string
  // L4 (Linear integration)
  linear_issue_id?: string
  created: string
  updated: string
}

export interface CreateActionRequest {
  title?: string
  description?: string
  owner?: string
  deadline?: string
  priority?: ActionPriority
  effort?: ActionEffort
  context?: string
  project_ids?: string[]
  my_day?: boolean
  starred?: boolean
  parent_id?: string
  status?: ActionStatus
}

export interface ReconcileResult {
  scanned: number
  lines_matched: number
  unique_actions: number
  updated: number
}

// --- Projects ---

export type ProjectStatus = "active" | "paused" | "archived"

// Project carries both a canonical UUID (stable across rename) and a slug
// (renameable filesystem alias). The UUID is what other entities'
// project_ids arrays reference; the slug is what shows up in URLs and the
// vault directory name.
export interface Project {
  uuid: string
  slug: string
  name: string
  status: ProjectStatus
  description?: string
  tags?: string[]
  created: string

  // L2 (Linear integration)
  initiative_id?: string
  linear_project_id?: string
  last_synced_at?: string

  // L7 (Workspace + Team layers)
  workspace_id?: string
  team_key?: string
}

// --- Initiatives (L2 strategic tier above Projects) ---

export type InitiativeStatus = "active" | "paused" | "archived"

export interface Initiative {
  uuid: string
  slug: string
  name: string
  status: InitiativeStatus
  description?: string
  target_date?: string
  owner?: string
  parent_initiative_id?: string
  team_id?: string
  team_key?: string  // L7
  linear_id?: string
  created: string
}

export interface InitiativeCounts {
  projects: number
}

export interface InitiativeContent {
  initiative: Initiative
  projects: {
    uuid: string
    slug: string
    name: string
    status: ProjectStatus
    description?: string
  }[]
  counts: InitiativeCounts
}

export interface CreateInitiativeRequest {
  name: string
  description?: string
  owner?: string
  target_date?: string
  parent_initiative_id?: string
  team_id?: string
}

export interface UpdateInitiativeRequest {
  name?: string
  description?: string
  status?: InitiativeStatus
  owner?: string
  target_date?: string
  parent_initiative_id?: string
  team_id?: string
  team_key?: string  // L7
}

// --- Workspaces (L7 — top-level tenancy boundary) ---

export interface Workspace {
  uuid: string
  slug: string
  name: string
  description?: string
  linear_workspace_id?: string
  linear_team_key?: string
  linear_api_key_env?: string
  created: string
}

export interface WorkspaceCounts {
  projects: number
}

export interface WorkspaceContent {
  workspace: Workspace
  projects: {
    uuid: string
    slug: string
    name: string
    status: ProjectStatus
    description?: string
  }[]
  counts: WorkspaceCounts
}

export interface CreateWorkspaceRequest {
  name: string
  description?: string
  linear_workspace_id?: string
  linear_team_key?: string
  linear_api_key_env?: string
}

export interface UpdateWorkspaceRequest {
  name?: string
  description?: string
  linear_workspace_id?: string
  linear_team_key?: string
  linear_api_key_env?: string
}

// --- Linear sync (L3 + L4) ---

export interface LinearSyncPreview {
  project_name: string
  project_action: "create" | "update"
  initiative_action?: "create" | "update" | ""
  initiative_name?: string
  documents_to_create: number
  documents_to_update: number
  document_titles?: string[]
  issues_to_create: number
  issues_to_update: number
  team_key: string
  warnings?: string[]
}

export interface LinearSyncError {
  entity: string
  error: string
}

export interface LinearSyncResult {
  project_linear_id?: string
  created?: string[]
  updated?: string[]
  errors?: LinearSyncError[]
}

export interface LinearSyncResponse {
  preview: LinearSyncPreview
  result?: LinearSyncResult
}

// L6 — Generate Issues from PRD
export interface LinearGeneratePreview {
  prd_id: string
  issues_to_create: number
  issues_already_mapped: number
  milestones_to_create: number
  milestones_already_mapped: number
  new_criterion_texts?: string[]
  new_milestone_names?: string[]
  warnings?: string[]
}

export interface LinearGenerateResult {
  prd_id: string
  issues_created?: string[]
  milestones_created?: string[]
  errors?: LinearSyncError[]
}

export interface LinearGenerateResponse {
  preview: LinearGeneratePreview
  result?: LinearGenerateResult
}

export interface CreateProjectRequest {
  name: string
  description?: string
  tags?: string[]
}

export interface UpdateProjectRequest {
  name?: string
  description?: string
  status?: ProjectStatus
  tags?: string[]
  // L2 (Linear integration). Empty string clears the link.
  initiative_id?: string
  // L7. Empty string clears.
  workspace_id?: string
  team_key?: string
}

export interface ProjectCounts {
  actions: number
  prds: number
  prototypes: number
  usecases: number
  threads: number
  documents: number
  meetings: number
}

// ProjectContent is GET /api/projects/:id/content — fan-out across every
// entity store filtered to one project. Powers the detail page tabs.
// Imports of Action/PRD/Prototype/UseCase/ShapeUpThread/Document/Meeting
// happen via deferred type references — declare-only here, resolved by
// tsc through the rest of this file.
export interface ProjectContent {
  project: Project
  counts: ProjectCounts
  actions: Action[]
  prds: PRD[]
  prototypes: Prototype[]
  usecases: UseCase[]
  threads: ShapeUpThread[]
  documents: Document[]
  meetings: Meeting[]
  warnings?: string[]
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
  stage?: string
  /** Current iteration number. Starts at 1; each successful refine
      bumps it. Past versions are archived server-side. */
  version: number
  spec?: string
  has_html?: boolean
  folder_path?: string
  created: string
  updated?: string
}

export interface PrototypeVersionInfo {
  version: number
  created: string
  hints?: string
  has_html: boolean
}

export interface RefinePrototypeRequest {
  hints?: string
  questions?: Question[]
  answers?: Answer[]
}

export interface RefinePRDRequest {
  hints?: string
  questions?: Question[]
  answers?: Answer[]
}

export interface PRDVersionInfo {
  version: number
  created: string
  hints?: string
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
  id: string // canonical UUID
  slug: string
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
  pipelines?: PipelinesGroup
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

// --- Usage telemetry ---

// Field names use snake_case to match the Go json tags exactly so the
// JSON payload deserializes cleanly without a remapping layer. Mirrors
// agent.Invocation in internal/agent/telemetry.go.
export interface UsageInvocation {
  timestamp: string
  model?: string
  session_id?: string
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  web_search_requests?: number
  cost_usd: number
  duration_ms: number
  duration_api_ms?: number
  num_turns?: number
  subtype?: string
  working_dir?: string
  max_turns?: number
  caller_hint?: string
  prompt_len: number
  result_len: number
  error?: string
}

export interface UsageModelTotals {
  calls: number
  cost_usd: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
}

export interface UsageDayTotals {
  day: string // YYYY-MM-DD (UTC)
  calls: number
  cost_usd: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
}

export interface UsageSummary {
  start: string
  end: string
  total_calls: number
  total_cost_usd: number
  total_input_tokens: number
  total_output_tokens: number
  total_cache_creation_tokens: number
  total_cache_read_tokens: number
  by_model: Record<string, UsageModelTotals>
  by_day: UsageDayTotals[]
}

export type UsageRecentResponse = UsageInvocation[]

// --- Meetings (audio → transcript → note pipeline) ---

// 3-stage pipeline. "audio" entries are raw audio files in
// vault/meetings/audio/ awaiting transcription; "transcript" entries
// are text transcripts (Deepgram-produced or supplied directly);
// "note" entries are Claude-extracted summaries.
export type MeetingStage = "audio" | "transcript" | "note"

export interface Meeting {
  id: string
  title: string
  stage: MeetingStage
  path: string
  source_id?: string
  source_path?: string
  // source_audio links a transcript back to the audio file it was
  // generated from. The backend uses this to suppress the audio entry
  // from List() once the meeting has progressed past the Audio stage.
  source_audio?: string
  projects?: string[]
  created: string
  updated?: string
}

// --- Documents ---

export type DocumentStage = "input" | "note"

export interface Document {
  id: string
  title: string
  path: string
  stage: DocumentStage
  tags?: string[]
  source?: string
  projects?: string[]
  created: string
  updated?: string
}

export interface CreateDocumentRequest {
  title: string
  content: string
}

// --- Unified dashboard row-2 cards ---

export interface PipelinesGroup {
  product: PipelineWidget
  meetings: PipelineWidget
  documents: PipelineWidget
  prototypes: PipelineWidget
}

export interface CardStageCount {
  stage: string
  count: number
}

export interface ItemUsageModelTotals {
  calls: number
  cost_usd: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
}

export interface ItemUsageAggregate {
  calls: number
  cost_usd: number
  input_tokens: number
  output_tokens: number
  cache_creation_tokens: number
  cache_read_tokens: number
  by_model: Record<string, ItemUsageModelTotals>
}

export interface CardItem {
  id: string
  title: string
  stage: string
  updated?: string
  view_url: string
  projects?: string[]
}

export interface CardDetail {
  type: string
  label: string
  stage_counts: CardStageCount[]
  aggregate: ItemUsageAggregate
  items: CardItem[]
}

// --- Onboarding ---

export interface OnboardingItem {
  id: string
  label: string
  configured: boolean
}

export interface OnboardingStatus {
  items: OnboardingItem[]
  configured_count: number
  total: number
}

// --- Persona Status ---

export interface PersonaInfo {
  name: string
  file: string
  configured: boolean
  summary: string
  char_count: number
  budget: number
}

export interface PersonaStatusResponse {
  personas: PersonaInfo[]
}

// --- Persona Chat ---

export interface PersonaChatMessage {
  role: "assistant" | "user"
  content: string
  options?: string[]
}

export interface PersonaChatStartResponse {
  session_id: string
  message: PersonaChatMessage
  turn: number
  max_turns: number
  done: boolean
}

export interface PersonaChatContinueResponse {
  message: PersonaChatMessage
  turn: number
  max_turns: number
  done: boolean
  result?: string
}

export interface PersonaChatFinishResponse {
  content: string
  done: boolean
}

// --- Linear integration ---

export interface LinearOrganization {
  id: string
  name: string
  url_key: string
}

export interface LinearViewer {
  id: string
  name: string
  email: string
}

export interface LinearStatusResponse {
  configured: boolean
  team_key: string
  organization?: LinearOrganization
  viewer?: LinearViewer
  error?: string
  webhook_secret_configured?: boolean
  last_webhook_at?: string
  // "oauth" when connected via the Connect button, "key" when a
  // personal API key is in use, "" when not configured.
  auth_mode?: "oauth" | "key" | ""
  // True when LINEAR_OAUTH_CLIENT_ID and CLIENT_SECRET are present —
  // the prerequisite before the Connect button can do anything.
  oauth_app_configured?: boolean
  // Computed from CORTICAL_BASE_URL — shown in the card so the user
  // can paste it into Linear's OAuth-app redirect-URI field.
  redirect_uri?: string
}

export interface LinearTeam {
  id: string
  name: string
  key: string
}

export interface LinearInitiative {
  id: string
  name: string
  description?: string
  status?: string
}

export interface LinearProject {
  id: string
  name: string
  state?: string
  description?: string
}

export interface LinearTestResponse {
  ok: boolean
  error?: string
  organization?: string
  viewer?: string
  team_name?: string
  team_warning?: string
}

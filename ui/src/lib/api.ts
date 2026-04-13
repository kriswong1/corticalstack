import type {
  DashboardSnapshot,
  StatusResponse,
  IntegrationStatus,
  Job,
  ConfirmPayload,
  Action,
  ReconcileResult,
  Project,
  CreateProjectRequest,
  VaultTreeNode,
  ShapeUpThread,
  Artifact,
  CreateIdeaRequest,
  AdvanceRequest,
  UseCase,
  FromDocRequest,
  FromTextRequest,
  GenerateUseCasesResponse,
  UseCaseFromDocQuestionsRequest,
  UseCaseFromTextQuestionsRequest,
  Prototype,
  CreatePrototypeRequest,
  PrototypeQuestionsRequest,
  PRD,
  CreatePRDRequest,
  PRDQuestionsRequest,
  PersonaResponse,
  PersonaEnhanceRequest,
  QuestionsResponse,
} from "@/types/api"

class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    const text = await res.text()
    throw new ApiError(res.status, text)
  }
  return res.json()
}

function post<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
}

export const api = {
  // Status
  getStatus: () => request<StatusResponse>("/api/status"),
  getIntegrations: () => request<IntegrationStatus[]>("/api/integrations"),

  // Dashboard operating view (single aggregator snapshot)
  getDashboard: () => request<DashboardSnapshot>("/api/dashboard"),

  // Ingest
  ingestText: (body: { text: string; title?: string }) =>
    post<{ job_id: string }>("/api/ingest/text", body),
  ingestURL: (body: { url: string }) =>
    post<{ job_id: string }>("/api/ingest/url", body),
  ingestFile: (formData: FormData) =>
    fetch("/api/ingest/file", { method: "POST", body: formData }).then(
      (res) => {
        if (!res.ok) return res.text().then((t) => Promise.reject(new ApiError(res.status, t)))
        return res.json() as Promise<{ job_id: string }>
      },
    ),

  // Jobs
  listJobs: () => request<Job[]>("/api/jobs"),
  getJob: (id: string) => request<Job>(`/api/jobs/${id}`),
  confirmJob: (id: string, payload: ConfirmPayload) =>
    post<{ status: string }>(`/api/jobs/${id}/confirm`, payload),

  // Vault
  getVaultTree: () => request<VaultTreeNode>("/api/vault/tree"),
  getVaultFile: (path: string) =>
    fetch(`/api/vault/file?path=${encodeURIComponent(path)}`).then((r) => {
      if (!r.ok) return r.text().then((t) => Promise.reject(new ApiError(r.status, t)))
      return r.text()
    }),

  // Projects
  listProjects: () => request<Project[]>("/api/projects"),
  getProject: (id: string) => request<Project>(`/api/projects/${id}`),
  createProject: (body: CreateProjectRequest) =>
    post<Project>("/api/projects", body),
  syncProjects: () =>
    post<{ created: string[]; created_count: number }>("/api/projects/sync", {}),

  // Actions
  listActions: (status?: string) =>
    request<Action[]>(`/api/actions${status ? `?status=${status}` : ""}`),
  getActionCounts: () => request<Record<string, number>>("/api/actions/counts"),
  setActionStatus: (id: string, status: string) =>
    post<Action>(`/api/actions/${id}/status`, { status }),
  updateAction: (id: string, patch: Partial<Action>) =>
    request<Action>(`/api/actions/${id}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(patch),
    }),
  reconcileActions: () =>
    post<ReconcileResult>("/api/actions/reconcile", {}),

  // ShapeUp
  listThreads: () => request<ShapeUpThread[]>("/api/shapeup/threads"),
  getThread: (id: string) =>
    request<ShapeUpThread>(`/api/shapeup/threads/${id}`),
  createIdea: (body: CreateIdeaRequest) =>
    post<Artifact>("/api/shapeup/idea", body),
  advanceThread: (id: string, body: AdvanceRequest) =>
    post<Artifact>(`/api/shapeup/threads/${id}/advance`, body),
  shapeupQuestions: (id: string, targetStage: string) =>
    post<QuestionsResponse>(`/api/shapeup/threads/${id}/questions`, {
      target_stage: targetStage,
    }),

  // Use Cases
  listUseCases: () => request<UseCase[]>("/api/usecases"),
  generateFromDoc: (body: FromDocRequest) =>
    post<GenerateUseCasesResponse>("/api/usecases/from-doc", body),
  generateFromText: (body: FromTextRequest) =>
    post<GenerateUseCasesResponse>("/api/usecases/from-text", body),
  useCaseFromDocQuestions: (body: UseCaseFromDocQuestionsRequest) =>
    post<QuestionsResponse>("/api/usecases/from-doc/questions", body),
  useCaseFromTextQuestions: (body: UseCaseFromTextQuestionsRequest) =>
    post<QuestionsResponse>("/api/usecases/from-text/questions", body),

  // Prototypes
  listPrototypes: () => request<Prototype[]>("/api/prototypes"),
  createPrototype: (body: CreatePrototypeRequest) =>
    post<Prototype>("/api/prototypes", body),
  prototypeQuestions: (body: PrototypeQuestionsRequest) =>
    post<QuestionsResponse>("/api/prototypes/questions", body),
  prototypeHTMLUrl: (id: string) => `/api/prototypes/${id}/html`,

  // PRDs
  listPRDs: () => request<PRD[]>("/api/prds"),
  createPRD: (body: CreatePRDRequest) => post<PRD>("/api/prds", body),
  prdQuestions: (body: PRDQuestionsRequest) =>
    post<QuestionsResponse>("/api/prds/questions", body),

  // Persona
  getPersona: (name: string) =>
    request<PersonaResponse>(`/api/persona/${name}`),
  savePersona: (name: string, content: string) =>
    post<{ status: string; name: string }>(`/api/persona/${name}`, { content }),
  setupPersona: (body: {
    name: string
    role: string
    timezone: string
    context: string
    projects?: string[]
    platforms?: string
  }) => post<{ status: string }>("/api/persona/setup", body),
  enhancePersona: (name: string, body: PersonaEnhanceRequest) =>
    post<{ content: string }>(`/api/persona/${name}/enhance`, body),
  personaEnhanceQuestions: (name: string, content: string) =>
    post<QuestionsResponse>(`/api/persona/${name}/enhance/questions`, {
      content,
    }),
}

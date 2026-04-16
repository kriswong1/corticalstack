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
  UsageRecentResponse,
  UsageSummary,
  Meeting,
  Document,
  CreateDocumentRequest,
  CardDetail,
  CardItem,
  ItemUsageAggregate,
} from "@/types/api"

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.name = "ApiError"
    this.status = status
  }
}

/**
 * getErrorMessage extracts a user-facing string from any thrown value.
 * Preferred input is an `ApiError`, then plain `Error`, then string, then
 * a JSON-stringified fallback for unknown shapes. Used by mutation
 * `onError` handlers across the app so toasts always display something
 * meaningful.
 */
export function getErrorMessage(err: unknown): string {
  if (err instanceof ApiError) return err.message || `Request failed (${err.status})`
  if (err instanceof Error) return err.message
  if (typeof err === "string") return err
  try {
    return JSON.stringify(err)
  } catch {
    return "Unknown error"
  }
}

// Default request timeout. Long-running Claude synthesis calls can pass
// `timeoutMs` to override (see LONG_TIMEOUT_MS below).
const DEFAULT_TIMEOUT_MS = 60_000
// Long timeout for synthesis / LLM-backed calls (5 minutes).
const LONG_TIMEOUT_MS = 5 * 60_000

type RequestInitWithTimeout = RequestInit & {
  signal?: AbortSignal
  timeoutMs?: number
}

/**
 * Low-level fetch wrapper that:
 *   - honors an external AbortSignal (React Query's `signal`) AND a
 *     per-call timeout (default 60s). Whichever fires first aborts.
 *   - surfaces non-JSON error bodies (HTML 500 pages, plain text) as
 *     `ApiError` with a clipped detail string rather than throwing
 *     SyntaxError from `res.json()`.
 *   - handles 204 No Content (and non-JSON successful bodies) by
 *     resolving with `undefined`.
 *   - wraps network errors in `ApiError(0, msg)`.
 */
async function request<T>(path: string, init: RequestInitWithTimeout = {}): Promise<T> {
  const { timeoutMs = DEFAULT_TIMEOUT_MS, signal: externalSignal, ...rest } = init
  const controller = new AbortController()
  const timer = setTimeout(
    () => controller.abort(new DOMException("request timeout", "TimeoutError")),
    timeoutMs,
  )

  // Chain the external signal (React Query's queryFn signal) so both
  // React Query cancellation AND our timeout can abort the same fetch.
  const onExternalAbort = () => controller.abort(externalSignal?.reason)
  if (externalSignal) {
    if (externalSignal.aborted) controller.abort(externalSignal.reason)
    else externalSignal.addEventListener("abort", onExternalAbort)
  }

  try {
    let res: Response
    try {
      res = await fetch(path, { ...rest, signal: controller.signal })
    } catch (e) {
      if (e instanceof DOMException && e.name === "TimeoutError") {
        throw new ApiError(0, "request timeout")
      }
      if (e instanceof DOMException && e.name === "AbortError") {
        // Preserve AbortError so React Query treats it as a cancellation.
        throw e
      }
      throw new ApiError(0, e instanceof Error ? e.message : "network error")
    }

    if (!res.ok) {
      const ct = res.headers.get("content-type") ?? ""
      let detail: string
      if (ct.includes("application/json")) {
        const body = await res
          .json()
          .catch(() => ({})) as { error?: string; message?: string }
        detail = body.error ?? body.message ?? res.statusText
      } else {
        const text = await res.text().catch(() => "")
        detail = text.slice(0, 500) || res.statusText
      }
      throw new ApiError(res.status, detail)
    }

    if (res.status === 204) return undefined as T
    const ct = res.headers.get("content-type") ?? ""
    if (!ct.includes("application/json")) return undefined as T
    return (await res.json()) as T
  } finally {
    clearTimeout(timer)
    if (externalSignal) externalSignal.removeEventListener("abort", onExternalAbort)
  }
}

function post<T>(path: string, body: unknown, init: RequestInitWithTimeout = {}): Promise<T> {
  return request<T>(path, {
    ...init,
    method: "POST",
    headers: { "Content-Type": "application/json", ...(init.headers ?? {}) },
    body: JSON.stringify(body),
  })
}

function put<T>(path: string, body: unknown, init: RequestInitWithTimeout = {}): Promise<T> {
  return request<T>(path, {
    ...init,
    method: "PUT",
    headers: { "Content-Type": "application/json", ...(init.headers ?? {}) },
    body: JSON.stringify(body),
  })
}

// postLong wraps post<> with a higher default timeout for LLM-backed
// synthesis endpoints (PRD, prototype, use-case, persona enhance,
// shapeup advance). Callers can still pass their own `timeoutMs` to
// override further.
function postLong<T>(path: string, body: unknown, init: RequestInitWithTimeout = {}): Promise<T> {
  return post<T>(path, body, { timeoutMs: LONG_TIMEOUT_MS, ...init })
}

export const api = {
  // Status
  getStatus: () => request<StatusResponse>("/api/status"),
  getIntegrations: () => request<IntegrationStatus[]>("/api/integrations"),

  // Dashboard operating view (single aggregator snapshot)
  getDashboard: () => request<DashboardSnapshot>("/api/dashboard"),

  // Usage telemetry — recent invocations and trailing-window aggregates
  getUsageRecent: (limit = 50) =>
    request<UsageRecentResponse>(`/api/usage/recent?limit=${limit}`),
  getUsageSummary: (windowStr = "24h") =>
    request<UsageSummary>(`/api/usage/summary?window=${windowStr}`),

  // Meetings (transcript → summary pipeline)
  listMeetings: () => request<Meeting[]>("/api/meetings"),
  setMeetingStage: (id: string, stage: string) =>
    post<{ id: string; stage: string }>(`/api/meetings/${id}/stage`, { stage }),

  // Card detail (unified dashboard row-2 drill-down)
  getCardDetail: (type: string) => request<CardDetail>(`/api/cards/${type}`),

  // Per-item usage aggregate (selection-driven refetch)
  getItemUsage: (type: string, ids?: string[]) => {
    const params = new URLSearchParams()
    if (ids && ids.length > 0) params.set("ids", ids.join(","))
    const qs = params.toString()
    return request<ItemUsageAggregate>(`/api/items/${type}/usage${qs ? `?${qs}` : ""}`)
  },

  // Documents
  listDocuments: () => request<CardItem[]>("/api/documents"),
  getDocument: (id: string) => request<Document>(`/api/documents/${id}`),
  createDocument: (body: CreateDocumentRequest) =>
    post<Document>("/api/documents", body),
  setDocumentStage: (id: string, stage: string) =>
    post<{ id: string; stage: string }>(`/api/documents/${id}/stage`, { stage }),
  setPrototypeStage: (id: string, stage: string) =>
    post<{ id: string; stage: string }>(`/api/prototypes/${id}/stage`, { stage }),

  // Ingest
  ingestText: (body: { text: string; title?: string }) =>
    post<{ job_id: string }>("/api/ingest/text", body),
  ingestURL: (body: { url: string }) =>
    post<{ job_id: string }>("/api/ingest/url", body),
  ingestFile: (formData: FormData) =>
    // File upload bypasses request<T> because the body is a FormData, not
    // JSON. We still route through AbortController/timeout semantics via
    // a small inline fetch and surface non-JSON errors as ApiError.
    (async () => {
      const controller = new AbortController()
      const timer = setTimeout(
        () => controller.abort(new DOMException("request timeout", "TimeoutError")),
        DEFAULT_TIMEOUT_MS,
      )
      try {
        const res = await fetch("/api/ingest/file", {
          method: "POST",
          body: formData,
          signal: controller.signal,
        })
        if (!res.ok) {
          const text = await res.text().catch(() => "")
          throw new ApiError(res.status, text.slice(0, 500) || res.statusText)
        }
        return (await res.json()) as { job_id: string }
      } finally {
        clearTimeout(timer)
      }
    })(),

  // Jobs
  listJobs: () => request<Job[]>("/api/jobs"),
  getJob: (id: string) => request<Job>(`/api/jobs/${id}`),
  confirmJob: (id: string, payload: ConfirmPayload) =>
    post<{ status: string }>(`/api/jobs/${id}/confirm`, payload),

  // Vault
  getVaultTree: () => request<VaultTreeNode>("/api/vault/tree"),
  getVaultFile: async (path: string) => {
    // Raw text response, not JSON — inline the fetch but still honor
    // DEFAULT_TIMEOUT_MS so hangs are bounded.
    const controller = new AbortController()
    const timer = setTimeout(
      () => controller.abort(new DOMException("request timeout", "TimeoutError")),
      DEFAULT_TIMEOUT_MS,
    )
    try {
      const r = await fetch(`/api/vault/file?path=${encodeURIComponent(path)}`, {
        signal: controller.signal,
      })
      if (!r.ok) {
        const text = await r.text().catch(() => "")
        throw new ApiError(r.status, text.slice(0, 500) || r.statusText)
      }
      return r.text()
    } finally {
      clearTimeout(timer)
    }
  },

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
    put<Action>(`/api/actions/${id}`, patch),
  reconcileActions: () =>
    postLong<ReconcileResult>("/api/actions/reconcile", {}),

  // ShapeUp
  listThreads: () => request<ShapeUpThread[]>("/api/shapeup/threads"),
  getThread: (id: string) =>
    request<ShapeUpThread>(`/api/shapeup/threads/${id}`),
  createIdea: (body: CreateIdeaRequest) =>
    post<Artifact>("/api/shapeup/idea", body),
  advanceThread: (id: string, body: AdvanceRequest) =>
    postLong<Artifact>(`/api/shapeup/threads/${id}/advance`, body),
  shapeupQuestions: (id: string, targetStage: string) =>
    postLong<QuestionsResponse>(`/api/shapeup/threads/${id}/questions`, {
      target_stage: targetStage,
    }),
  getAdvanceProgress: (threadId: string) =>
    request<{ turn: number; max_turns: number; status: string; stage: string }>(
      `/api/shapeup/threads/${threadId}/progress`,
    ),

  // Use Cases
  listUseCases: () => request<UseCase[]>("/api/usecases"),
  generateFromDoc: (body: FromDocRequest) =>
    postLong<GenerateUseCasesResponse>("/api/usecases/from-doc", body),
  generateFromText: (body: FromTextRequest) =>
    postLong<GenerateUseCasesResponse>("/api/usecases/from-text", body),
  useCaseFromDocQuestions: (body: UseCaseFromDocQuestionsRequest) =>
    postLong<QuestionsResponse>("/api/usecases/from-doc/questions", body),
  useCaseFromTextQuestions: (body: UseCaseFromTextQuestionsRequest) =>
    postLong<QuestionsResponse>("/api/usecases/from-text/questions", body),

  // Prototypes
  listPrototypes: () => request<Prototype[]>("/api/prototypes"),
  createPrototype: (body: CreatePrototypeRequest) =>
    postLong<Prototype>("/api/prototypes", body),
  prototypeQuestions: (body: PrototypeQuestionsRequest) =>
    postLong<QuestionsResponse>("/api/prototypes/questions", body),
  prototypeHTMLUrl: (id: string) => `/api/prototypes/${id}/html`,

  // PRDs
  listPRDs: () => request<PRD[]>("/api/prds"),
  createPRD: (body: CreatePRDRequest) => postLong<PRD>("/api/prds", body),
  prdQuestions: (body: PRDQuestionsRequest) =>
    postLong<QuestionsResponse>("/api/prds/questions", body),

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
  }) => postLong<{ status: string }>("/api/persona/setup", body),
  enhancePersona: (name: string, body: PersonaEnhanceRequest) =>
    postLong<{ content: string }>(`/api/persona/${name}/enhance`, body),
  personaEnhanceQuestions: (name: string, content: string) =>
    postLong<QuestionsResponse>(`/api/persona/${name}/enhance/questions`, {
      content,
    }),
}

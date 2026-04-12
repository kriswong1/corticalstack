import { useEffect, useReducer, useRef } from "react"
import type { JobStatus, PreviewResult, Job, JobEvent } from "@/types/api"

interface SSEState {
  status: JobStatus | null
  messages: string[]
  preview: PreviewResult | null
  notePath: string | null
  error: string | null
  connected: boolean
  done: boolean
}

type SSEAction =
  | { type: "snapshot"; job: Job }
  | { type: "status"; event: JobEvent }
  | { type: "progress"; event: JobEvent }
  | { type: "preview"; preview: PreviewResult }
  | { type: "complete"; event: JobEvent }
  | { type: "failed"; event: JobEvent }
  | { type: "connected" }
  | { type: "disconnected" }

const initialState: SSEState = {
  status: null,
  messages: [],
  preview: null,
  notePath: null,
  error: null,
  connected: false,
  done: false,
}

function sseReducer(state: SSEState, action: SSEAction): SSEState {
  switch (action.type) {
    case "connected":
      return { ...state, connected: true }
    case "disconnected":
      return { ...state, connected: false }
    case "snapshot": {
      const job = action.job
      return {
        ...state,
        status: job.status,
        messages: job.messages ?? [],
        preview: job.preview ?? state.preview,
        notePath: job.note_path ?? state.notePath,
        error: job.error ?? null,
        done: job.status === "completed" || job.status === "failed",
      }
    }
    case "status":
      return {
        ...state,
        status: action.event.status,
        messages: action.event.message
          ? [...state.messages, action.event.message]
          : state.messages,
      }
    case "progress":
      return {
        ...state,
        messages: action.event.message
          ? [...state.messages, action.event.message]
          : state.messages,
      }
    case "preview":
      return { ...state, preview: action.preview }
    case "complete":
      return {
        ...state,
        status: "completed",
        done: true,
        messages: action.event.message
          ? [...state.messages, action.event.message]
          : state.messages,
      }
    case "failed":
      return {
        ...state,
        status: "failed",
        done: true,
        error: action.event.message ?? "Unknown error",
        messages: action.event.message
          ? [...state.messages, action.event.message]
          : state.messages,
      }
  }
}

export function useJobStream(jobId: string | null) {
  const [state, dispatch] = useReducer(sseReducer, initialState)
  const esRef = useRef<EventSource | null>(null)

  useEffect(() => {
    if (!jobId) return

    const es = new EventSource(`/api/jobs/${jobId}/stream`)
    esRef.current = es

    es.onopen = () => dispatch({ type: "connected" })
    es.onerror = () => dispatch({ type: "disconnected" })

    es.addEventListener("job_snapshot", (e) => {
      dispatch({ type: "snapshot", job: JSON.parse(e.data) })
    })
    es.addEventListener("job_status", (e) => {
      dispatch({ type: "status", event: JSON.parse(e.data) })
    })
    es.addEventListener("job_progress", (e) => {
      dispatch({ type: "progress", event: JSON.parse(e.data) })
    })
    es.addEventListener("job_preview", (e) => {
      const data = JSON.parse(e.data)
      if (data.preview) dispatch({ type: "preview", preview: data.preview })
    })
    es.addEventListener("job_complete", (e) => {
      dispatch({ type: "complete", event: JSON.parse(e.data) })
      es.close()
    })
    es.addEventListener("job_failed", (e) => {
      dispatch({ type: "failed", event: JSON.parse(e.data) })
      es.close()
    })

    return () => {
      es.close()
    }
  }, [jobId])

  return state
}

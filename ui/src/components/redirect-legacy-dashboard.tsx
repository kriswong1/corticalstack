import { Navigate, useParams } from "react-router-dom"

// RedirectLegacyDashboard maps the old `/dashboard/:type` family to
// the flat, type-first URLs (`/product`, `/meetings`, `/documents`,
// `/prototypes`). Keeps existing bookmarks and external links
// working after the URL restructure.
export function RedirectLegacyDashboard({ hasId = false }: { hasId?: boolean }) {
  const { type, id } = useParams<{ type: string; id: string }>()
  const plural: Record<string, string> = {
    product: "product",
    meeting: "meetings",
    document: "documents",
    prototype: "prototypes",
  }
  const slug = plural[type ?? ""] ?? type ?? ""
  const target = hasId ? `/${slug}/${id ?? ""}` : `/${slug}`
  return <Navigate to={target} replace />
}

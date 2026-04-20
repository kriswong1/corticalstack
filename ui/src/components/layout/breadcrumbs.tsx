import { Link } from "react-router-dom"
import { ArrowLeft, ChevronRight } from "lucide-react"
import { cn } from "@/lib/utils"

export interface BreadcrumbItem {
  label: string
  /** URL for this crumb. Omit for the current (terminal) crumb. */
  to?: string
}

interface BreadcrumbsProps {
  items: BreadcrumbItem[]
  /** Show a back arrow that navigates to the parent crumb (the
      second-to-last item). Hidden when there's no parent. */
  showBack?: boolean
  className?: string
}

// Breadcrumbs renders the page's position in the app hierarchy as a
// clickable trail (`Dashboard / Section / Item`) plus an optional
// back arrow that jumps to the parent crumb. Sits above PageHeader
// on every page so users can orient and step back one level without
// hunting for the sidebar.
export function Breadcrumbs({
  items,
  showBack = true,
  className,
}: BreadcrumbsProps) {
  if (items.length === 0) return null

  const parent = items.length >= 2 ? items[items.length - 2] : null

  return (
    <div className={cn("flex items-center gap-3 mb-4", className)}>
      {showBack && parent?.to && (
        <Link
          to={parent.to}
          aria-label={`Back to ${parent.label}`}
          className="inline-flex h-7 w-7 items-center justify-center rounded-md border border-border text-muted-foreground hover:text-foreground hover:border-foreground/40 transition-colors"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
      )}
      <nav aria-label="Breadcrumb">
        <ol className="flex items-center gap-1.5 text-[13px]">
          {items.map((item, idx) => {
            const isLast = idx === items.length - 1
            return (
              <li key={idx} className="flex items-center gap-1.5 min-w-0">
                {idx > 0 && (
                  <ChevronRight
                    className="h-3 w-3 text-muted-foreground/50 flex-shrink-0"
                    aria-hidden
                  />
                )}
                {item.to && !isLast ? (
                  <Link
                    to={item.to}
                    className="text-muted-foreground hover:text-foreground transition-colors truncate"
                  >
                    {item.label}
                  </Link>
                ) : (
                  <span
                    className={cn(
                      "truncate",
                      isLast
                        ? "text-foreground font-medium"
                        : "text-muted-foreground",
                    )}
                    aria-current={isLast ? "page" : undefined}
                  >
                    {item.label}
                  </span>
                )}
              </li>
            )
          })}
        </ol>
      </nav>
    </div>
  )
}

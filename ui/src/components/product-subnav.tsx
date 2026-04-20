import { NavLink, useLocation } from "react-router-dom"
import { cn } from "@/lib/utils"
import { Box, FileCheck, FileText, Lightbulb } from "lucide-react"

// ProductSubnav renders a tab strip across the top of the four product
// surfaces (Threads, Prototypes, PRDs, Use Cases) so the user can hop
// between them without bouncing back to the sidebar. Each tab stays
// active for both its listing URL and any item-detail URL nested
// beneath it.
const tabs = [
  { to: "/product", label: "Threads", icon: Lightbulb, matches: (p: string) => p === "/product" || p.startsWith("/product/") },
  { to: "/prototypes", label: "Prototypes", icon: Box, matches: (p: string) => p === "/prototypes" || p.startsWith("/prototypes/") },
  { to: "/prds", label: "PRDs", icon: FileCheck, matches: (p: string) => p === "/prds" || p.startsWith("/prds/") },
  { to: "/usecases", label: "Use Cases", icon: FileText, matches: (p: string) => p === "/usecases" || p.startsWith("/usecases/") },
]

export function ProductSubnav() {
  const { pathname } = useLocation()
  return (
    <nav
      aria-label="Product sections"
      className="mb-6 flex items-center gap-1 border-b border-border"
    >
      {tabs.map((t) => {
        const active = t.matches(pathname)
        return (
          <NavLink
            key={t.to}
            to={t.to}
            className={cn(
              "inline-flex items-center gap-1.5 px-3 py-2 text-[13px] font-normal transition-colors border-b-2 -mb-px",
              active
                ? "border-primary text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground hover:border-border",
            )}
          >
            <t.icon className="h-3.5 w-3.5" />
            {t.label}
          </NavLink>
        )
      })}
    </nav>
  )
}

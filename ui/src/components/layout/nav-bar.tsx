import { useState } from "react"
import { NavLink } from "react-router-dom"
import { cn } from "@/lib/utils"
import { useTheme } from "@/hooks/use-theme"
import {
  LayoutDashboard,
  Download,
  Library,
  Settings,
  FolderKanban,
  ListChecks,
  Lightbulb,
  FileText,
  Box,
  FileCheck,
  Brain,
  Sun,
  Moon,
  Menu,
  X,
} from "lucide-react"

const navItems = [
  { to: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
  { to: "/ingest", label: "Ingest", icon: Download },
  { to: "/library", label: "Library", icon: Library },
  { to: "/projects", label: "Projects", icon: FolderKanban },
  { to: "/actions", label: "Actions", icon: ListChecks },
  { to: "/product", label: "Product", icon: Lightbulb },
  { to: "/usecases", label: "Use Cases", icon: FileText },
  { to: "/prototypes", label: "Prototypes", icon: Box },
  { to: "/prds", label: "PRDs", icon: FileCheck },
  { to: "/config", label: "Config", icon: Settings },
]

const personaItems = [
  { to: "/persona/soul", label: "SOUL" },
  { to: "/persona/user", label: "USER" },
  { to: "/persona/memory", label: "MEMORY" },
]

export function NavBar() {
  const { theme, toggleTheme } = useTheme()
  const [mobileOpen, setMobileOpen] = useState(false)

  return (
    <header className="sticky top-0 z-50 w-full border-b border-border bg-background/95 backdrop-blur-md">
      <div className="mx-auto flex h-14 max-w-[1080px] items-center px-4">
        <NavLink
          to="/dashboard"
          className="mr-6 flex items-center gap-2 text-foreground"
        >
          <Brain className="h-5 w-5 text-primary" />
          <span className="text-sm font-normal tracking-wide">
            CORTICAL STACK
          </span>
        </NavLink>

        {/* Desktop nav */}
        <nav className="hidden lg:flex items-center gap-1 overflow-x-auto">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) =>
                cn(
                  "flex items-center gap-1.5 rounded-sm px-2.5 py-1.5 text-[14px] font-normal transition-colors whitespace-nowrap",
                  isActive
                    ? "bg-secondary text-secondary-foreground"
                    : "text-foreground hover:bg-accent hover:text-accent-foreground",
                )
              }
            >
              <item.icon className="h-3.5 w-3.5" />
              {item.label}
            </NavLink>
          ))}
        </nav>

        <div className="ml-auto flex items-center gap-1">
          {/* Persona links — desktop only */}
          <div className="hidden lg:flex items-center gap-1 mr-2">
            {personaItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                className={({ isActive }) =>
                  cn(
                    "rounded-sm px-2 py-1 text-[12px] font-normal transition-colors",
                    isActive
                      ? "bg-primary text-primary-foreground"
                      : "text-muted-foreground hover:text-foreground",
                  )
                }
              >
                {item.label}
              </NavLink>
            ))}
          </div>

          {/* Theme toggle */}
          <button
            onClick={toggleTheme}
            className="rounded-sm p-1.5 text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
            aria-label="Toggle theme"
          >
            {theme === "dark" ? (
              <Sun className="h-4 w-4" />
            ) : (
              <Moon className="h-4 w-4" />
            )}
          </button>

          {/* Mobile hamburger */}
          <button
            onClick={() => setMobileOpen(!mobileOpen)}
            className="lg:hidden rounded-sm p-1.5 text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
            aria-label="Toggle menu"
          >
            {mobileOpen ? (
              <X className="h-4 w-4" />
            ) : (
              <Menu className="h-4 w-4" />
            )}
          </button>
        </div>
      </div>

      {/* Mobile nav dropdown */}
      {mobileOpen && (
        <div className="lg:hidden border-t border-border bg-background px-4 py-3">
          <nav className="flex flex-col gap-1">
            {navItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                onClick={() => setMobileOpen(false)}
                className={({ isActive }) =>
                  cn(
                    "flex items-center gap-2 rounded-sm px-3 py-2 text-[14px] font-normal transition-colors",
                    isActive
                      ? "bg-secondary text-secondary-foreground"
                      : "text-foreground hover:bg-accent",
                  )
                }
              >
                <item.icon className="h-4 w-4" />
                {item.label}
              </NavLink>
            ))}
            <div className="border-t border-border mt-2 pt-2 flex gap-1">
              {personaItems.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  onClick={() => setMobileOpen(false)}
                  className={({ isActive }) =>
                    cn(
                      "rounded-sm px-2 py-1 text-[12px] font-normal transition-colors",
                      isActive
                        ? "bg-primary text-primary-foreground"
                        : "text-muted-foreground hover:text-foreground",
                    )
                  }
                >
                  {item.label}
                </NavLink>
              ))}
            </div>
          </nav>
        </div>
      )}
    </header>
  )
}

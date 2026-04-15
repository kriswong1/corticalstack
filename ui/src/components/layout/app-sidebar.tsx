import { NavLink } from "react-router-dom"
import { cn } from "@/lib/utils"
import { useTheme } from "@/hooks/use-theme"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
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
  Activity,
  Workflow,
} from "lucide-react"

const mainItems = [
  { to: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
  { to: "/ingest", label: "Ingest", icon: Download },
  { to: "/library", label: "Library", icon: Library },
]

const projectItems = [
  { to: "/projects", label: "Projects", icon: FolderKanban },
  { to: "/actions", label: "Actions", icon: ListChecks },
]

const productItems = [
  { to: "/product", label: "Product", icon: Lightbulb },
  { to: "/usecases", label: "Use Cases", icon: FileText },
  { to: "/prototypes", label: "Prototypes", icon: Box },
  { to: "/prds", label: "PRDs", icon: FileCheck },
]

const monitoringItems = [
  { to: "/pipelines", label: "Pipelines", icon: Workflow },
  { to: "/usage", label: "Usage", icon: Activity },
]

const systemItems = [
  { to: "/config", label: "Config", icon: Settings },
]

function NavGroup({
  label,
  items,
}: {
  label: string
  items: { to: string; label: string; icon: React.ComponentType<{ className?: string }> }[]
}) {
  return (
    <SidebarGroup>
      <SidebarGroupLabel>{label}</SidebarGroupLabel>
      <SidebarGroupContent>
        <SidebarMenu>
          {items.map((item) => (
            <SidebarMenuItem key={item.to}>
              <SidebarMenuButton asChild>
                <NavLink
                  to={item.to}
                  className={({ isActive }) =>
                    cn(
                      "flex items-center gap-2",
                      isActive && "bg-sidebar-accent text-sidebar-accent-foreground font-normal",
                    )
                  }
                >
                  <item.icon className="h-4 w-4" />
                  <span>{item.label}</span>
                </NavLink>
              </SidebarMenuButton>
            </SidebarMenuItem>
          ))}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  )
}

export function AppSidebar() {
  const { theme, toggleTheme } = useTheme()

  return (
    <Sidebar>
      <SidebarHeader className="p-4">
        <NavLink
          to="/dashboard"
          className="flex items-center gap-2 text-sidebar-foreground"
        >
          <Brain className="h-5 w-5 text-sidebar-primary" />
          <span className="text-sm font-normal tracking-wide">
            CORTICAL STACK
          </span>
        </NavLink>
      </SidebarHeader>

      <SidebarContent>
        <NavGroup label="Main" items={mainItems} />
        <NavGroup label="Projects" items={projectItems} />
        <NavGroup label="Product" items={productItems} />
        <NavGroup label="System" items={systemItems} />
        <NavGroup label="Monitoring" items={monitoringItems} />
      </SidebarContent>

      <SidebarFooter className="p-2">
        <button
          onClick={toggleTheme}
          className="flex w-full items-center gap-2 rounded-sm px-3 py-2 text-sm text-sidebar-foreground/70 hover:text-sidebar-foreground hover:bg-sidebar-accent transition-colors"
          aria-label="Toggle theme"
        >
          {theme === "dark" ? (
            <>
              <Sun className="h-4 w-4" />
              <span>Light mode</span>
            </>
          ) : (
            <>
              <Moon className="h-4 w-4" />
              <span>Dark mode</span>
            </>
          )}
        </button>
      </SidebarFooter>
    </Sidebar>
  )
}

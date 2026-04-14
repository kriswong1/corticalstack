import { Outlet, useLocation } from "react-router-dom"
import { SidebarProvider, SidebarInset, SidebarTrigger } from "@/components/ui/sidebar"
import { TooltipProvider } from "@/components/ui/tooltip"
import { Toaster } from "@/components/ui/sonner"
import { AppSidebar } from "./app-sidebar"
import { ErrorBoundary } from "@/components/shared/error-boundary"
import { Separator } from "@/components/ui/separator"

export function AppShell() {
  const location = useLocation()
  return (
    <TooltipProvider>
      <SidebarProvider>
        <AppSidebar />
        <SidebarInset>
          <header className="flex h-12 items-center gap-2 border-b border-border px-4">
            <SidebarTrigger className="-ml-1 text-muted-foreground" />
            <Separator orientation="vertical" className="mr-2 h-4" />
          </header>
          <main className="flex-1 p-6">
            <div className="mx-auto max-w-[1080px]">
              {/*
                Key the ErrorBoundary on pathname so navigation after a
                render crash forces a fresh mount. Without the key, the
                Outlet swaps children under a boundary that stays stuck
                in its error state and the user sees a stale error card
                on the new route.
              */}
              <ErrorBoundary key={location.pathname}>
                <Outlet />
              </ErrorBoundary>
            </div>
          </main>
        </SidebarInset>
        <Toaster position="bottom-right" />
      </SidebarProvider>
    </TooltipProvider>
  )
}

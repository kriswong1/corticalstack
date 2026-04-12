import { Outlet } from "react-router-dom"
import { NavBar } from "./nav-bar"
import { ErrorBoundary } from "@/components/shared/error-boundary"

export function AppShell() {
  return (
    <div className="min-h-screen bg-background">
      <NavBar />
      <main className="mx-auto max-w-[1080px] px-4 py-8">
        <ErrorBoundary>
          <Outlet />
        </ErrorBoundary>
      </main>
    </div>
  )
}

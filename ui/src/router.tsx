import { createBrowserRouter, Navigate } from "react-router-dom"
import { AppShell } from "@/components/layout/app-shell"
import { DashboardPage } from "@/pages/dashboard"
import { DashboardCardPage } from "@/pages/dashboard-card"
import { IngestPage } from "@/pages/ingest"
import { LibraryPage } from "@/pages/library"
import { ConfigPage } from "@/pages/config"
import { ProjectsPage } from "@/pages/projects"
import { ActionsPage } from "@/pages/actions"
import { ProductPage } from "@/pages/product"
import { UseCasesPage } from "@/pages/usecases"
import { PrototypesPage } from "@/pages/prototypes"
import { PRDsPage } from "@/pages/prds"
import { ItemPipelinePage } from "@/pages/item-pipeline"

export const router = createBrowserRouter([
  {
    path: "/",
    element: <AppShell />,
    children: [
      { index: true, element: <Navigate to="/dashboard" replace /> },
      { path: "dashboard", element: <DashboardPage /> },
      { path: "dashboard/:type", element: <DashboardCardPage /> },
      { path: "dashboard/:type/:id", element: <ItemPipelinePage /> },
      { path: "ingest", element: <IngestPage /> },
      { path: "library", element: <LibraryPage /> },
      { path: "config", element: <ConfigPage /> },
      { path: "projects", element: <ProjectsPage /> },
      { path: "actions", element: <ActionsPage /> },
      { path: "product", element: <ProductPage /> },
      { path: "usecases", element: <UseCasesPage /> },
      { path: "prototypes", element: <PrototypesPage /> },
      { path: "prds", element: <PRDsPage /> },
      // Persona editors are now inside Config page; redirect old URLs
      { path: "persona/:name", element: <Navigate to="/config" replace /> },
    ],
  },
])

import { createBrowserRouter, Navigate } from "react-router-dom"
import { AppShell } from "@/components/layout/app-shell"
import { DashboardPage } from "@/pages/dashboard"
import { DashboardCardPage } from "@/pages/dashboard-card"
import { IngestPage } from "@/pages/ingest"
import { LibraryPage } from "@/pages/library"
import { ConfigPage } from "@/pages/config"
import { InitiativesPage } from "@/pages/initiatives"
import { InitiativeDetailPage } from "@/pages/initiative-detail"
import { WorkspacesPage } from "@/pages/workspaces"
import { WorkspaceDetailPage } from "@/pages/workspace-detail"
import { ProjectsPage } from "@/pages/projects"
import { ProjectDetailPage } from "@/pages/project-detail"
import { ActionsPage } from "@/pages/actions"
import { UseCasesPage } from "@/pages/usecases"
import { PRDsPage } from "@/pages/prds"
import { PRDDetailPage } from "@/pages/prd-detail"
import { ItemPipelinePage } from "@/pages/item-pipeline"
import { RedirectLegacyDashboard } from "@/components/redirect-legacy-dashboard"

export const router = createBrowserRouter([
  {
    path: "/",
    element: <AppShell />,
    children: [
      { index: true, element: <Navigate to="/dashboard" replace /> },

      // Primary surfaces
      { path: "dashboard", element: <DashboardPage /> },
      { path: "ingest", element: <IngestPage /> },
      { path: "library", element: <LibraryPage /> },
      { path: "config", element: <ConfigPage /> },
      { path: "initiatives", element: <InitiativesPage /> },
      { path: "initiatives/:id", element: <InitiativeDetailPage /> },
      { path: "workspaces", element: <WorkspacesPage /> },
      { path: "workspaces/:id", element: <WorkspaceDetailPage /> },
      { path: "projects", element: <ProjectsPage /> },
      { path: "projects/:id", element: <ProjectDetailPage /> },
      { path: "actions", element: <ActionsPage /> },

      // Pipeline surfaces — one listing + one item route per type.
      // All four types share the DashboardCardPage layout (stage
      // cards + items table); the item detail shares ItemPipelinePage.
      { path: "product", element: <DashboardCardPage type="product" /> },
      { path: "product/:id", element: <ItemPipelinePage type="product" /> },
      { path: "meetings", element: <DashboardCardPage type="meeting" /> },
      { path: "meetings/:id", element: <ItemPipelinePage type="meeting" /> },
      { path: "documents", element: <DashboardCardPage type="document" /> },
      { path: "documents/:id", element: <ItemPipelinePage type="document" /> },
      { path: "prototypes", element: <DashboardCardPage type="prototype" /> },
      { path: "prototypes/:id", element: <ItemPipelinePage type="prototype" /> },

      // PRDs and Use Cases keep their own pages (distinct forms +
      // data shape) but visually align with the pipeline surfaces.
      { path: "prds", element: <PRDsPage /> },
      { path: "prds/:id", element: <PRDDetailPage /> },
      { path: "usecases", element: <UseCasesPage /> },

      // Legacy redirects — keep old /dashboard/:type URLs working.
      { path: "dashboard/:type", element: <RedirectLegacyDashboard /> },
      { path: "dashboard/:type/:id", element: <RedirectLegacyDashboard hasId /> },

      // Persona editors are now inside Config page; redirect old URLs.
      { path: "persona/:name", element: <Navigate to="/config" replace /> },
    ],
  },
])

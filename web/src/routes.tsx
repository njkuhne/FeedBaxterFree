import { createBrowserRouter, Navigate, Outlet } from "react-router-dom";
import { AnnouncerProvider } from "./components/Announcer";
import { ToastProvider } from "./components/Toast";
import { AppShortcuts } from "./feedback/shortcuts";
import { UndoProvider } from "./feedback/undo";
import { AuthProvider } from "./shell/AuthProvider";
import { AdminOnly, RequireAuth } from "./shell/Shell";
import Channels from "./pages/Channels";
import ChangePassword from "./pages/ChangePassword";
import FeedbackDetail from "./pages/FeedbackDetail";
import Inbox from "./pages/Inbox";
import Login from "./pages/Login";
import NotFound from "./pages/NotFound";
import People from "./pages/People";
import PerUser from "./pages/PerUser";
import Profile from "./pages/Profile";
import System from "./pages/System";

function Providers() {
  return (
    <ToastProvider>
      <AnnouncerProvider>
        <UndoProvider>
          <AuthProvider>
            <AppShortcuts>
              <Outlet />
            </AppShortcuts>
          </AuthProvider>
        </UndoProvider>
      </AnnouncerProvider>
    </ToastProvider>
  );
}

export const routes = [
  {
    element: <Providers />,
    children: [
      { path: "/login", element: <Login /> },
      { path: "/change-password", element: <ChangePassword /> },
      {
        element: <RequireAuth />,
        children: [
          { path: "/", element: <Inbox /> },
          { path: "/feedback/:id", element: <FeedbackDetail /> },
          { path: "/user/:userId", element: <PerUser /> },
          { path: "/channels", element: <AdminOnly><Channels /></AdminOnly> },
          { path: "/people", element: <AdminOnly><People /></AdminOnly> },
          { path: "/system", element: <AdminOnly><System /></AdminOnly> },
          { path: "/settings", element: <AdminOnly><Navigate to="/system" replace /></AdminOnly> },
          { path: "/profile", element: <Profile /> },
          { path: "*", element: <NotFound /> },
        ],
      },
    ],
  },
];

export const createRouter = () => createBrowserRouter(routes);

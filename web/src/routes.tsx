import { createBrowserRouter, Outlet } from "react-router-dom";
import { AnnouncerProvider } from "./components/Announcer";
import { ToastProvider } from "./components/Toast";
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
import Settings from "./pages/Settings";

function Providers() {
  return (
    <ToastProvider>
      <AnnouncerProvider>
        <AuthProvider>
          <Outlet />
        </AuthProvider>
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
          { path: "/settings", element: <AdminOnly><Settings /></AdminOnly> },
          { path: "/profile", element: <Profile /> },
          { path: "*", element: <NotFound /> },
        ],
      },
    ],
  },
];

export const createRouter = () => createBrowserRouter(routes);

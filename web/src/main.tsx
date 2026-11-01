import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import "./styles/fonts.css";
import "./styles/tokens.css";
import "./styles/base.css";
import "./styles/components.css";
import "./styles/layout.css";

const root = document.getElementById("app-root");
if (!root) throw new Error("#app-root is missing");
createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

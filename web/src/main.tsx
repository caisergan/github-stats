import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import App from "./App";
import { applyTheme, getTheme } from "./theme";
import "./styles.css";

// Apply the persisted theme before first paint so there is no flash of the
// default theme. The `data-theme` attribute on <html> is what styles.css keys off.
applyTheme(getTheme());

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
);

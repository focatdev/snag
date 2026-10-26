import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "../styles.css";
import "./installer.css";
import { Installer } from "./Installer";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <Installer />
  </StrictMode>,
);

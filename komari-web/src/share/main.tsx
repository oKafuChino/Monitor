import { createRoot } from "react-dom/client";
import "./i18n";
import "./share.css";
import { SharePage } from "./SharePage";
import { ShareDataProvider } from "./ShareDataProvider";

// A dedicated entry intentionally has no main-site providers or routing.
createRoot(document.getElementById("root")!).render(<ShareDataProvider><SharePage /></ShareDataProvider>);

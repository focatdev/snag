import { useEffect } from "react";
import { Sidebar } from "./components/Sidebar";
import { StatusBar } from "./components/StatusBar";
import { TitleBar } from "./components/TitleBar";
import { Shortcuts, WhatsNew } from "./components/Overlays";
import { Toasts } from "./components/Toasts";
import { TooltipProvider } from "./components/ui/tooltip";
import { Home } from "./features/home/Home";
import { Library } from "./features/library/Library";
import { Onboarding } from "./features/onboarding/Onboarding";
import { Settings } from "./features/settings/Settings";
import { Viewer } from "./features/viewer/Viewer";
import { useStore } from "./lib/store";

export function App() {
  const ready = useStore((s) => s.ready);
  const fatal = useStore((s) => s.fatal);
  const view = useStore((s) => s.view);
  const onboarding = useStore((s) => s.ready && s.settings !== null && !s.settings.onboarded);

  useEffect(() => {
    // the default menu has reload and inspect
    const block = (e: MouseEvent) => {
      const t = e.target as HTMLElement;
      if (!t.closest("input, textarea")) e.preventDefault();
    };
    window.addEventListener("contextmenu", block);
    return () => window.removeEventListener("contextmenu", block);
  }, []);

  return (
    <TooltipProvider>
      <div className="flex h-full flex-col">
        <TitleBar />
        {onboarding ? (
          <div className="min-h-0 flex-1">
            <Onboarding />
          </div>
        ) : (
        <div className="flex min-h-0 flex-1">
          <Sidebar />
          <main className="min-w-0 flex-1 overflow-hidden">
            {fatal ? (
              <div className="grid h-full place-items-center p-8 text-center">
                <div>
                  <p className="text-base font-medium">Snag could not reach its backend</p>
                  <p className="mt-1 text-sm text-muted">{fatal}</p>
                </div>
              </div>
            ) : !ready ? null : view.name === "home" ? (
              <Home />
            ) : view.name === "library" ? (
              <Library key={`${view.kind ?? "clips"}-${view.game ?? ""}`} kind={view.kind} game={view.game} />
            ) : view.name === "settings" ? (
              <Settings />
            ) : (
              <Viewer key={view.id} id={view.id} />
            )}
          </main>
        </div>
        )}
        {!onboarding && <StatusBar />}
        <Toasts />
        <Shortcuts />
        <WhatsNew />
      </div>
    </TooltipProvider>
  );
}

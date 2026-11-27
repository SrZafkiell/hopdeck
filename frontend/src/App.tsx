import { useState } from "react";
import { DetailPane } from "./components/DetailPane";
import { EmptyState } from "./components/EmptyState";
import { StatusBar } from "./components/StatusBar";
import { TunnelList } from "./components/TunnelList";
import { sampleTunnels } from "./sample";
import type { Tunnel } from "./types";

// Deck files reach the window from the Go side in M02-04. Until then the list is empty,
// except in development with ?sample in the address, which shows made-up tunnels.
function initialTunnels(): Tunnel[] {
  if (
    import.meta.env.DEV &&
    new URLSearchParams(window.location.search).has("sample")
  ) {
    return sampleTunnels;
  }
  return [];
}

function App() {
  const [tunnels] = useState<Tunnel[]>(initialTunnels);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const selected = tunnels.find((tunnel) => tunnel.id === selectedId) ?? null;

  return (
    <div className="app">
      <div className="panes">
        <nav className="pane list-pane" aria-label="Tunnel list">
          <TunnelList
            tunnels={tunnels}
            selectedId={selectedId}
            onSelect={setSelectedId}
          />
        </nav>
        <main className="pane detail-pane">
          {tunnels.length === 0 ? (
            <EmptyState />
          ) : (
            <DetailPane tunnel={selected} />
          )}
        </main>
      </div>
      <StatusBar tunnels={tunnels} />
    </div>
  );
}

export default App;

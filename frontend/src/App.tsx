import { useEffect, useMemo, useState } from "react";
import { DetailPane } from "./components/DetailPane";
import { EmptyState } from "./components/EmptyState";
import { StatusBar } from "./components/StatusBar";
import { TunnelList } from "./components/TunnelList";
import { goProfiles, watchDecks } from "./decks";
import { sampleProfiles, sampleSnapshot } from "./sample";
import {
  emptySnapshot,
  rowsOf,
  type ProfileSource,
  type Snapshot,
} from "./types";

// In development, ?sample in the address shows a made-up decks folder instead of the
// real one. Production builds drop this branch and the sample data with it.
const usingSample =
  import.meta.env.DEV &&
  new URLSearchParams(window.location.search).has("sample");

function App() {
  const [snapshot, setSnapshot] = useState<Snapshot>(
    usingSample ? sampleSnapshot : emptySnapshot,
  );
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [noticeDismissed, setNoticeDismissed] = useState(false);
  const profiles = useMemo<ProfileSource>(
    () => (usingSample ? sampleProfiles(setSnapshot) : goProfiles),
    [],
  );

  useEffect(() => {
    if (usingSample) {
      return;
    }
    return watchDecks(setSnapshot);
  }, []);

  const rows = rowsOf(snapshot);
  const selected = rows.find((row) => row.id === selectedId) ?? null;

  return (
    <div className="app">
      {snapshot.notice && !noticeDismissed && (
        <div className="notice" role="status">
          <span>{snapshot.notice}</span>
          <button type="button" onClick={() => setNoticeDismissed(true)}>
            Dismiss
          </button>
        </div>
      )}
      <div className="panes">
        <nav className="pane list-pane" aria-label="Tunnel list">
          <TunnelList
            rows={rows}
            selectedId={selectedId}
            onSelect={setSelectedId}
          />
        </nav>
        <main className="pane detail-pane">
          {rows.length === 0 ? (
            <EmptyState folder={snapshot.folder} error={snapshot.error} />
          ) : (
            <DetailPane row={selected} profiles={profiles} />
          )}
        </main>
      </div>
      <StatusBar snapshot={snapshot} />
    </div>
  );
}

export default App;

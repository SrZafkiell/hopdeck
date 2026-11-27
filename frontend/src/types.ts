// What the window knows about the decks folder. The Go side sends it (see decks.ts);
// these types are the window's own, so the components do not depend on generated code.

export type TunnelState =
  "idle" | "checking" | "starting" | "connected" | "reconnecting" | "failed";

export interface Tunnel {
  /** Unique in the window: the deck file and the tunnel name. */
  id: string;
  name: string;
  /** The name of the deck the tunnel comes from. */
  deck: string;
  region: string;
  /** The jump host as the deck file names it: an instance ID or a tag filter. */
  jumpHost: string;
  /** Empty for a port on the jump host itself. */
  remoteHost: string;
  remotePort: number;
  localPort: number;
  /** The local AWS profile mapped to the deck file, or null before one is chosen. */
  profile: string | null;
  state: TunnelState;
  /** Why the tunnel is in its state, in plain words; empty when there is nothing to add. */
  reason: string;
}

/** One problem of a deck file that cannot be used. */
export interface Problem {
  /** The line of the file, counted from 1; 0 when the problem concerns the whole file. */
  line: number;
  /** The path of the field, such as tunnels[2].remote.port; empty for the whole file. */
  field: string;
  text: string;
}

/** One deck file: its tunnels, or the problems that keep it from loading. */
export interface DeckFile {
  file: string;
  name: string;
  problems: Problem[];
  tunnels: Tunnel[];
}

/** What the decks folder holds. */
export interface Snapshot {
  folder: string;
  /** Set when the folder itself cannot be read. */
  error: string;
  decks: DeckFile[];
}

/** A row of the list: a tunnel, or a deck file that cannot be read. */
export type Row =
  | { kind: "tunnel"; id: string; tunnel: Tunnel }
  | { kind: "broken"; id: string; deck: DeckFile };

export const emptySnapshot: Snapshot = { folder: "", error: "", decks: [] };

export const stateLabels: Record<TunnelState, string> = {
  idle: "Not connected",
  checking: "Checking",
  starting: "Starting",
  connected: "Connected",
  reconnecting: "Reconnecting",
  failed: "Failed",
};

/** The rows of the list, in the order of the deck files: each tunnel of a deck file that
 * loaded, and one row for each deck file that cannot be read. */
export function rowsOf(snapshot: Snapshot): Row[] {
  const rows: Row[] = [];
  for (const deck of snapshot.decks) {
    if (deck.problems.length > 0) {
      rows.push({ kind: "broken", id: `file:${deck.file}`, deck });
      continue;
    }
    for (const tunnel of deck.tunnels) {
      rows.push({ kind: "tunnel", id: tunnel.id, tunnel });
    }
  }
  return rows;
}

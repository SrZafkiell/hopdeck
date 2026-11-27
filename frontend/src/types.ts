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
  /** The file name of that deck, which choices such as the profile are kept under. */
  deckFile: string;
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
  /** The local AWS profile chosen for this deck file; empty when none is. */
  profile: string;
  /** The profile the deck file suggests; empty when it suggests none. */
  suggestedProfile: string;
  problems: Problem[];
  tunnels: Tunnel[];
}

/** What the decks folder holds. */
export interface Snapshot {
  folder: string;
  /** Set when the folder itself cannot be read. */
  error: string;
  decks: DeckFile[];
  /** Something Hopdeck did on its own that the person should know; empty if nothing. */
  notice: string;
}

/** A row of the list: a tunnel, or a deck file that cannot be read. */
export type Row =
  | { kind: "tunnel"; id: string; tunnel: Tunnel; deck: DeckFile }
  | { kind: "broken"; id: string; deck: DeckFile };

export const emptySnapshot: Snapshot = {
  folder: "",
  error: "",
  decks: [],
  notice: "",
};

export const stateLabels: Record<TunnelState, string> = {
  idle: "Not connected",
  checking: "Checking",
  starting: "Starting",
  connected: "Connected",
  reconnecting: "Reconnecting",
  failed: "Failed",
};

export type ProfileKind =
  "keys" | "role" | "identity-center" | "process" | "none";

/** One AWS profile of this computer: its name and kind, never a credential. */
export interface ProfileOption {
  name: string;
  kind: ProfileKind;
}

export interface ProfileList {
  profiles: ProfileOption[];
  /** Set when the AWS configuration files cannot be read. */
  error: string;
}

/** Where the profile chooser gets its list and sends a choice: the Go side, or the
 * development sample. */
export interface ProfileSource {
  list(): Promise<ProfileList>;
  choose(deckFile: string, profile: string): Promise<void>;
}

export const kindLabels: Record<ProfileKind, string> = {
  keys: "access keys",
  role: "role",
  "identity-center": "IAM Identity Center",
  process: "credential process",
  none: "no credentials",
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
      rows.push({ kind: "tunnel", id: tunnel.id, tunnel, deck });
    }
  }
  return rows;
}

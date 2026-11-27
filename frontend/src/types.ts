// The window's view of a tunnel. The Go side sends these once deck files load (M02-04);
// until then the list is empty.

export type TunnelState =
  "idle" | "checking" | "starting" | "connected" | "reconnecting" | "failed";

export interface Tunnel {
  /** Unique in the window: the deck file and the tunnel name. */
  id: string;
  name: string;
  /** The deck file the tunnel comes from. */
  deck: string;
  region: string;
  /** The jump host as the deck file names it: an instance ID or a tag filter. */
  jumpHost: string;
  remoteHost: string;
  remotePort: number;
  localPort: number;
  /** The local AWS profile mapped to the deck file, or null before one is chosen. */
  profile: string | null;
  state: TunnelState;
  /** Why the tunnel is in its state, in plain words; empty when there is nothing to add. */
  reason: string;
}

export const stateLabels: Record<TunnelState, string> = {
  idle: "Not connected",
  checking: "Checking",
  starting: "Starting",
  connected: "Connected",
  reconnecting: "Reconnecting",
  failed: "Failed",
};

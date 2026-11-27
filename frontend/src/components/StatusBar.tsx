import type { Snapshot } from "../types";

interface StatusBarProps {
  snapshot: Snapshot;
}

// One line at the bottom of the window: what is loaded, what cannot be read and how much
// of it is connected.
export function StatusBar({ snapshot }: StatusBarProps) {
  const loaded = snapshot.decks.filter((deck) => deck.problems.length === 0);
  const broken = snapshot.decks.length - loaded.length;
  const tunnels = loaded.flatMap((deck) => deck.tunnels);
  const connected = tunnels.filter(
    (tunnel) => tunnel.state === "connected",
  ).length;

  let summary = "No deck files loaded";
  if (snapshot.error) {
    summary = "The decks folder cannot be read";
  } else if (loaded.length > 0) {
    summary = `${count(tunnels.length, "tunnel")} from ${count(loaded.length, "deck file")}`;
  }
  if (broken > 0 && !snapshot.error) {
    summary += `, ${count(broken, "deck file")} cannot be read`;
  }

  return (
    <footer className="status-bar" role="status" aria-live="polite">
      <span>{summary}</span>
      <span>{connected} connected</span>
    </footer>
  );
}

function count(amount: number, word: string): string {
  return `${amount} ${word}${amount === 1 ? "" : "s"}`;
}

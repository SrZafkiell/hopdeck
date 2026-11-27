import type { Tunnel } from "../types";

interface StatusBarProps {
  tunnels: Tunnel[];
}

// One line at the bottom of the window: what is loaded and how much of it is connected.
export function StatusBar({ tunnels }: StatusBarProps) {
  const decks = new Set(tunnels.map((tunnel) => tunnel.deck)).size;
  const connected = tunnels.filter(
    (tunnel) => tunnel.state === "connected",
  ).length;

  return (
    <footer className="status-bar" role="status" aria-live="polite">
      <span>
        {decks === 0
          ? "No deck files loaded"
          : `${count(tunnels.length, "tunnel")} from ${count(decks, "deck file")}`}
      </span>
      <span>{connected} connected</span>
    </footer>
  );
}

function count(amount: number, word: string): string {
  return `${amount} ${word}${amount === 1 ? "" : "s"}`;
}

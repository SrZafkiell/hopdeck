import type { KeyboardEvent } from "react";
import { stateLabels, type Tunnel } from "../types";

interface TunnelListProps {
  tunnels: Tunnel[];
  selectedId: string | null;
  onSelect: (id: string) => void;
}

// A single-select list box. The list itself takes the focus; the arrow keys, Home and
// End move the selection, and the selected row is announced through
// aria-activedescendant, so the list works with the keyboard alone.
export function TunnelList({ tunnels, selectedId, onSelect }: TunnelListProps) {
  if (tunnels.length === 0) {
    return <p className="list-empty">No tunnels yet.</p>;
  }

  const selectedIndex = tunnels.findIndex((tunnel) => tunnel.id === selectedId);
  const last = tunnels.length - 1;

  function handleKeyDown(event: KeyboardEvent<HTMLUListElement>) {
    const targets: Record<string, number> = {
      ArrowDown: Math.min(selectedIndex + 1, last),
      ArrowUp: Math.max(selectedIndex - 1, 0),
      Home: 0,
      End: last,
    };
    const next = targets[event.key];
    if (next === undefined) {
      return;
    }
    event.preventDefault();
    onSelect(tunnels[next].id);
  }

  return (
    <ul
      className="tunnel-list"
      role="listbox"
      aria-label="Tunnels"
      tabIndex={0}
      aria-activedescendant={
        selectedIndex >= 0 ? optionId(selectedIndex) : undefined
      }
      onKeyDown={handleKeyDown}
    >
      {tunnels.map((tunnel, index) => (
        <li
          key={tunnel.id}
          id={optionId(index)}
          className="tunnel-row"
          role="option"
          aria-selected={index === selectedIndex}
          onClick={() => onSelect(tunnel.id)}
        >
          <span className="tunnel-name">{tunnel.name}</span>
          <span className="tunnel-deck">{tunnel.deck}</span>
          <span className={`tunnel-state state-${tunnel.state}`}>
            {stateLabels[tunnel.state]}
          </span>
        </li>
      ))}
    </ul>
  );
}

function optionId(index: number): string {
  return `tunnel-option-${index}`;
}

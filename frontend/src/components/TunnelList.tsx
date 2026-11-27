import type { KeyboardEvent } from "react";
import { stateLabels, type Row } from "../types";

interface TunnelListProps {
  rows: Row[];
  selectedId: string | null;
  onSelect: (id: string) => void;
}

// A single-select list box of tunnels and of deck files that cannot be read. The list
// itself takes the focus; the arrow keys, Home and End move the selection, and the
// selected row is announced through aria-activedescendant, so the list works with the
// keyboard alone.
export function TunnelList({ rows, selectedId, onSelect }: TunnelListProps) {
  if (rows.length === 0) {
    return <p className="list-empty">No tunnels yet.</p>;
  }

  const selectedIndex = rows.findIndex((row) => row.id === selectedId);
  const last = rows.length - 1;

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
    onSelect(rows[next].id);
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
      {rows.map((row, index) => (
        <li
          key={row.id}
          id={optionId(index)}
          className="tunnel-row"
          role="option"
          aria-selected={index === selectedIndex}
          onClick={() => onSelect(row.id)}
        >
          {row.kind === "tunnel" ? (
            <>
              <span className="tunnel-name">{row.tunnel.name}</span>
              <span className="tunnel-deck">{row.tunnel.deck}</span>
              <span className={`tunnel-state state-${row.tunnel.state}`}>
                {stateLabels[row.tunnel.state]}
              </span>
            </>
          ) : (
            <>
              <span className="tunnel-name">{row.deck.file}</span>
              <span className="tunnel-deck">Deck file</span>
              <span className="tunnel-state state-failed">Cannot be read</span>
            </>
          )}
        </li>
      ))}
    </ul>
  );
}

function optionId(index: number): string {
  return `tunnel-option-${index}`;
}

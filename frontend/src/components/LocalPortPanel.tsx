import { useEffect, useState, type FormEvent } from "react";
import { messageOf } from "../text";
import {
  maxLocalPort,
  minLocalPort,
  type PortCheck,
  type PortSource,
  type Tunnel,
} from "../types";

interface LocalPortPanelProps {
  tunnel: Tunnel;
  source: PortSource;
}

// The local address of a tunnel and whether a tunnel could listen on its port now. The
// port is checked when the panel opens, when the port changes and on demand; a port that
// cannot be used is named with the reason, and a free one is offered. A port picked here
// holds for this computer only: Hopdeck keeps it in its settings, never in the deck file.
export function LocalPortPanel({ tunnel, source }: LocalPortPanelProps) {
  const [check, setCheck] = useState<PortCheck | null>(null);
  const [round, setRound] = useState(0);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    setCheck(null);
    source
      .check(tunnel)
      .then((answer) => {
        if (active) {
          setCheck(answer);
        }
      })
      .catch((reason: unknown) => {
        if (active) {
          setCheck({
            port: tunnel.localPort,
            free: false,
            reason: messageOf(reason),
            suggestion: 0,
          });
        }
      });
    return () => {
      active = false;
    };
    // The check depends on where the tunnel listens, not on the snapshot it came in.
  }, [source, tunnel.deckFile, tunnel.name, tunnel.localPort, round]);

  function choose(port: number) {
    setError("");
    source
      .choose(tunnel, port)
      .then(() => setEditing(false))
      .catch((reason: unknown) => setError(messageOf(reason)));
  }

  function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const port = Number(draft);
    if (!Number.isInteger(port) || port < minLocalPort || port > maxLocalPort) {
      setError(`Enter a port from ${minLocalPort} to ${maxLocalPort}.`);
      return;
    }
    choose(port);
  }

  function startEditing() {
    setDraft(String(tunnel.localPort));
    setError("");
    setEditing(true);
  }

  const pickedHere = tunnel.localPort !== tunnel.deckPort;
  const status =
    check === null
      ? { className: "port-checking", text: "Checking the port" }
      : check.free
        ? {
            className: "port-free",
            text: `Port ${check.port} is free on this computer.`,
          }
        : { className: "port-busy", text: check.reason };

  return (
    <div className="local-port">
      <p>
        <code>127.0.0.1:{tunnel.localPort}</code>
      </p>
      {pickedHere && (
        <p className="port-note">
          {`Picked for this computer. The deck file says ${tunnel.deckPort}.`}
        </p>
      )}
      <p className={`port-state ${status.className}`} role="status">
        {status.text}
      </p>
      {editing ? (
        // noValidate: the panel says what is wrong with a port itself, in the same words
        // as the rest of the window, instead of the browser's own bubble.
        <form className="port-form" onSubmit={save} noValidate>
          <label>
            Port on this computer{" "}
            <input
              type="number"
              min={minLocalPort}
              max={maxLocalPort}
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              autoFocus
            />
          </label>
          <button type="submit">Save</button>
          <button
            type="button"
            onClick={() => {
              setEditing(false);
              setError("");
            }}
          >
            Cancel
          </button>
        </form>
      ) : (
        <div className="port-actions">
          {check !== null && !check.free && check.suggestion > 0 && (
            <button type="button" onClick={() => choose(check.suggestion)}>
              Use port {check.suggestion}
            </button>
          )}
          <button type="button" onClick={startEditing}>
            Change port
          </button>
          {pickedHere && (
            <button type="button" onClick={() => choose(0)}>
              Use the deck file's port
            </button>
          )}
          <button type="button" onClick={() => setRound((value) => value + 1)}>
            Check the port again
          </button>
        </div>
      )}
      {error && (
        <p className="port-error" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}

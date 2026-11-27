import { stateLabels, type Tunnel } from "../types";

interface DetailPaneProps {
  tunnel: Tunnel | null;
}

// Where the selected tunnel goes: the local address to use, the remote end, the jump
// host and the AWS profile, with the state and its reason on top.
export function DetailPane({ tunnel }: DetailPaneProps) {
  if (tunnel === null) {
    return (
      <p className="detail-hint">
        Select a tunnel in the list to see where it goes.
      </p>
    );
  }

  return (
    <article className="detail" aria-labelledby="detail-title">
      <h2 id="detail-title">{tunnel.name}</h2>
      <p className={`detail-state state-${tunnel.state}`}>
        {stateLabels[tunnel.state]}
        {tunnel.reason && (
          <span className="detail-reason"> - {tunnel.reason}</span>
        )}
      </p>
      <dl className="detail-fields">
        <dt>Local address</dt>
        <dd>
          <code>127.0.0.1:{tunnel.localPort}</code>
        </dd>
        <dt>Remote</dt>
        <dd>
          <code>
            {tunnel.remoteHost}:{tunnel.remotePort}
          </code>
        </dd>
        <dt>Jump host</dt>
        <dd>
          <code>{tunnel.jumpHost}</code>
        </dd>
        <dt>Region</dt>
        <dd>{tunnel.region}</dd>
        <dt>AWS profile</dt>
        <dd>{tunnel.profile ?? "Not chosen yet"}</dd>
        <dt>Deck file</dt>
        <dd>{tunnel.deck}</dd>
      </dl>
    </article>
  );
}

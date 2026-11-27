import {
  stateLabels,
  type DeckFile,
  type Problem,
  type ProfileSource,
  type Row,
  type Tunnel,
} from "../types";
import { IdentityPanel } from "./IdentityPanel";
import { ProfileChooser } from "./ProfileChooser";

interface DetailPaneProps {
  row: Row | null;
  profiles: ProfileSource;
}

// The selected row in detail: where a tunnel goes, or why a deck file cannot be read.
export function DetailPane({ row, profiles }: DetailPaneProps) {
  if (row === null) {
    return (
      <p className="detail-hint">
        Select a tunnel in the list to see where it goes.
      </p>
    );
  }
  return row.kind === "tunnel" ? (
    <TunnelDetail tunnel={row.tunnel} deck={row.deck} profiles={profiles} />
  ) : (
    <BrokenDeck deck={row.deck} />
  );
}

// Where a tunnel goes: the local address to use, the remote end, the jump host and the
// AWS profile of its deck file, with the state and its reason on top.
function TunnelDetail({
  tunnel,
  deck,
  profiles,
}: {
  tunnel: Tunnel;
  deck: DeckFile;
  profiles: ProfileSource;
}) {
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
          {tunnel.remoteHost ? (
            <code>
              {tunnel.remoteHost}:{tunnel.remotePort}
            </code>
          ) : (
            <>
              Port <code>{tunnel.remotePort}</code> on the jump host
            </>
          )}
        </dd>
        <dt>Jump host</dt>
        <dd>
          <code>{tunnel.jumpHost}</code>
        </dd>
        <dt>Region</dt>
        <dd>{tunnel.region}</dd>
        <dt>AWS profile</dt>
        <dd>
          <ProfileChooser deck={deck} source={profiles} />
        </dd>
        {deck.profile && (
          <>
            <dt>Identity</dt>
            <dd>
              <IdentityPanel
                profile={deck.profile}
                region={tunnel.region}
                source={profiles}
              />
            </dd>
          </>
        )}
        <dt>Deck</dt>
        <dd>{tunnel.deck}</dd>
      </dl>
    </article>
  );
}

// A deck file that cannot be read: its problems, each with its line and field. The Go
// side never puts a value of the file into a problem, so this shows no secret.
function BrokenDeck({ deck }: { deck: DeckFile }) {
  return (
    <article className="detail" aria-labelledby="detail-title">
      <h2 id="detail-title">{deck.file}</h2>
      <p className="detail-state state-failed">Cannot be read</p>
      <p className="detail-note">
        None of the tunnels of this file are shown until these problems are
        fixed. Fix them in the file and save it; the list updates by itself.
      </p>
      <ul className="problem-list">
        {deck.problems.map((problem, index) => (
          <li key={index}>{describeProblem(problem)}</li>
        ))}
      </ul>
    </article>
  );
}

function describeProblem(problem: Problem): string {
  const where = [problem.line > 0 ? `Line ${problem.line}` : "", problem.field]
    .filter(Boolean)
    .join(", ");
  const text = problem.text.charAt(0).toUpperCase() + problem.text.slice(1);
  return where ? `${where}: ${problem.text}` : text;
}

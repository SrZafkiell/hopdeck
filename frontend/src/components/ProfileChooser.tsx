import { useEffect, useState } from "react";
import {
  kindLabels,
  type DeckFile,
  type ProfileList,
  type ProfileSource,
} from "../types";

interface ProfileChooserProps {
  deck: DeckFile;
  source: ProfileSource;
}

// The AWS profile of a deck file, chosen from this computer's profiles. Hopdeck keeps
// the choice in its settings, never in the deck file, and it holds for every tunnel of
// the file.
export function ProfileChooser({ deck, source }: ProfileChooserProps) {
  const [list, setList] = useState<ProfileList | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    let active = true;
    setList(null);
    setError("");
    source
      .list()
      .then((result) => {
        if (active) {
          setList(result);
        }
      })
      .catch((reason: unknown) => {
        if (active) {
          setList({ profiles: [], error: messageOf(reason) });
        }
      });
    return () => {
      active = false;
    };
  }, [source, deck.file]);

  if (list === null) {
    return <span className="chooser-note">Reading your AWS profiles</span>;
  }
  if (list.error) {
    return <span className="chooser-error">{list.error}</span>;
  }
  if (list.profiles.length === 0) {
    return (
      <span className="chooser-note">
        No AWS profiles were found on this computer.
      </span>
    );
  }

  const names = new Set(list.profiles.map((profile) => profile.name));

  function choose(profile: string) {
    setError("");
    source
      .choose(deck.file, profile)
      .catch((reason: unknown) => setError(messageOf(reason)));
  }

  return (
    <div className="profile-chooser">
      <select
        aria-label="AWS profile for this deck file"
        value={deck.profile}
        onChange={(event) => choose(event.target.value)}
      >
        <option value="">Choose a profile</option>
        {deck.profile && !names.has(deck.profile) && (
          <option value={deck.profile}>{deck.profile} (not found)</option>
        )}
        {list.profiles.map((profile) => (
          <option key={profile.name} value={profile.name}>
            {profile.name} - {kindLabels[profile.kind]}
            {profile.name === deck.suggestedProfile ? " (suggested)" : ""}
          </option>
        ))}
      </select>
      <p className="chooser-note">
        Used for every tunnel of this deck file.
        {deck.suggestedProfile &&
          ` The deck file suggests ${deck.suggestedProfile}.`}
      </p>
      {error && (
        <p className="chooser-error" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}

function messageOf(reason: unknown): string {
  return reason instanceof Error ? reason.message : String(reason);
}

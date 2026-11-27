import { Events } from "@wailsio/runtime";
import { DeckService } from "../bindings/github.com/SrZafkiell/hopdeck";
import type * as models from "../bindings/github.com/SrZafkiell/hopdeck/models";
import {
  kindLabels,
  stateLabels,
  type ProfileKind,
  type ProfileSource,
  type Snapshot,
  type TunnelState,
} from "./types";

// The bridge to the Go side: the decks folder as it is now, then every change the Go
// side reports. Returns a function that stops listening.
export function watchDecks(onChange: (snapshot: Snapshot) => void): () => void {
  let active = true;
  DeckService.Snapshot()
    .then((model) => {
      if (active) {
        onChange(toSnapshot(model));
      }
    })
    .catch(() => {
      // Without the Go side, as on the plain dev server, the window stays empty.
    });
  const stop = Events.On("decks", (event) => onChange(toSnapshot(event.data)));
  return () => {
    active = false;
    stop();
  };
}

// toSnapshot turns the generated model into the window's own types. Go sends empty lists
// as null, and an unknown state is shown as idle rather than trusted.
export function toSnapshot(model: models.Snapshot): Snapshot {
  return {
    folder: model.folder,
    error: model.error,
    notice: model.notice,
    decks: (model.decks ?? []).map((deck) => ({
      file: deck.file,
      name: deck.name,
      profile: deck.profile,
      suggestedProfile: deck.suggestedProfile,
      problems: deck.problems ?? [],
      tunnels: (deck.tunnels ?? []).map((tunnel) => ({
        ...tunnel,
        state: toState(tunnel.state),
      })),
    })),
  };
}

// goProfiles lists the AWS profiles of this computer and sends a choice to the Go side,
// which keeps it in the settings and answers with a new snapshot.
export const goProfiles: ProfileSource = {
  async list() {
    const model = await DeckService.Profiles();
    return {
      error: model.error,
      profiles: (model.profiles ?? []).map((profile) => ({
        name: profile.name,
        kind: toKind(profile.kind),
      })),
    };
  },
  async choose(deckFile, profile) {
    await DeckService.ChooseProfile(deckFile, profile);
  },
};

function toKind(kind: string): ProfileKind {
  return Object.prototype.hasOwnProperty.call(kindLabels, kind)
    ? (kind as ProfileKind)
    : "none";
}

function toState(state: string): TunnelState {
  return Object.prototype.hasOwnProperty.call(stateLabels, state)
    ? (state as TunnelState)
    : "idle";
}

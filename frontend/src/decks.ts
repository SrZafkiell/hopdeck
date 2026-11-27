import { Events } from "@wailsio/runtime";
import { DeckService } from "../bindings/github.com/SrZafkiell/hopdeck";
import type * as models from "../bindings/github.com/SrZafkiell/hopdeck/models";
import { stateLabels, type Snapshot, type TunnelState } from "./types";

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
    decks: (model.decks ?? []).map((deck) => ({
      file: deck.file,
      name: deck.name,
      problems: deck.problems ?? [],
      tunnels: (deck.tunnels ?? []).map((tunnel) => ({
        ...tunnel,
        state: toState(tunnel.state),
      })),
    })),
  };
}

function toState(state: string): TunnelState {
  return Object.prototype.hasOwnProperty.call(stateLabels, state)
    ? (state as TunnelState)
    : "idle";
}

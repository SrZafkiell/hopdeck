// Shown while no deck file is loaded: what a deck file is and how to add one, in two
// sentences.
export function EmptyState() {
  return (
    <div className="empty-state">
      <h2>No deck files yet</h2>
      <p>
        Hopdeck opens the tunnels described in a deck file, a small file that
        your administrator gives you. Put the deck file in the decks folder and
        its tunnels appear in the list.
      </p>
    </div>
  );
}

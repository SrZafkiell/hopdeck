interface EmptyStateProps {
  /** The decks folder; empty when it is not known, as on the plain dev server. */
  folder: string;
  /** Why the decks folder cannot be read; empty when it can. */
  error: string;
}

// Shown while no deck file is in the decks folder: what a deck file is and how to add
// one, in two sentences, then where the folder is.
export function EmptyState({ folder, error }: EmptyStateProps) {
  return (
    <div className="empty-state">
      <h2>No deck files yet</h2>
      <p>
        Hopdeck opens the tunnels described in a deck file, a small file that
        your administrator gives you. Put the deck file in the decks folder and
        its tunnels appear in the list.
      </p>
      {folder && (
        <p className="empty-folder">
          Decks folder: <code>{folder}</code>
        </p>
      )}
      {error && (
        <p className="empty-error">The decks folder cannot be read: {error}</p>
      )}
    </div>
  );
}

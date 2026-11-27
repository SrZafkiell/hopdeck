// Small text helpers shared by the components.

/** The message of a failed call, whatever was thrown. */
export function messageOf(reason: unknown): string {
  return reason instanceof Error ? reason.message : String(reason);
}

/** An AWS account ID with all but its last four digits hidden, for screens that may be
 * shared or recorded. */
export function maskAccount(account: string): string {
  if (account.length <= 4) {
    return account;
  }
  return "*".repeat(account.length - 4) + account.slice(-4);
}

/** The text with every occurrence of the account ID masked, such as in an ARN. */
export function maskIn(text: string, account: string): string {
  return account ? text.split(account).join(maskAccount(account)) : text;
}

import { useEffect, useState } from "react";
import { maskAccount, maskIn, messageOf } from "../text";
import type { IdentityResult, IdentityState, ProfileSource } from "../types";

interface IdentityPanelProps {
  profile: string;
  region: string;
  source: ProfileSource;
}

const stateTitles: Record<IdentityState, string> = {
  valid: "Signed in",
  expired: "Expired",
  "needs-mfa": "Needs an MFA code",
  "needs-sign-in": "Needs sign-in",
  error: "Cannot check",
};

// Who the profile's credentials belong to, asked of AWS when the panel opens and on
// demand. The account ID is masked until the person asks to see it, so a shared or
// recorded screen does not show it.
export function IdentityPanel({ profile, region, source }: IdentityPanelProps) {
  const [result, setResult] = useState<IdentityResult | null>(null);
  const [round, setRound] = useState(0);
  const [showAccount, setShowAccount] = useState(false);

  useEffect(() => {
    let active = true;
    setResult(null);
    source
      .check(profile, region)
      .then((answer) => {
        if (active) {
          setResult(answer);
        }
      })
      .catch((reason: unknown) => {
        if (active) {
          setResult({
            state: "error",
            account: "",
            caller: "",
            reason: messageOf(reason),
          });
        }
      });
    return () => {
      active = false;
    };
  }, [source, profile, region, round]);

  if (result === null) {
    return <span className="identity-note">Checking with AWS</span>;
  }

  const account = showAccount ? result.account : maskAccount(result.account);
  const caller = showAccount
    ? result.caller
    : maskIn(result.caller, result.account);

  return (
    <div className={`identity identity-${result.state}`}>
      <p className="identity-state">{stateTitles[result.state]}</p>
      {result.state === "valid" ? (
        <p className="identity-who">
          Account <code>{account}</code> as <code>{caller}</code>
        </p>
      ) : (
        <p className="identity-who">{result.reason}</p>
      )}
      <div className="identity-actions">
        <button type="button" onClick={() => setRound((value) => value + 1)}>
          Check again
        </button>
        {result.state === "valid" && (
          <button
            type="button"
            aria-pressed={showAccount}
            onClick={() => setShowAccount((value) => !value)}
          >
            {showAccount ? "Hide account ID" : "Show account ID"}
          </button>
        )}
      </div>
    </div>
  );
}

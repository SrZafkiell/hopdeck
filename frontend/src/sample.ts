import {
  maxLocalPort,
  minLocalPort,
  type PortSource,
  type ProfileOption,
  type ProfileSource,
  type Snapshot,
  type Tunnel,
} from "./types";

// A made-up decks folder for development previews: open the dev server with ?sample in
// the address to see it. Every value is an example (the documented AWS example instance
// ID, .example.internal host names), and production builds leave this data out because
// only a development-only branch of App uses it.
export const sampleSnapshot: Snapshot = {
  folder: "C:\\Users\\example\\AppData\\Roaming\\Hopdeck\\decks",
  error: "",
  notice: "",
  decks: [
    {
      file: "old-team.hopdeck.yaml",
      name: "old-team.hopdeck.yaml",
      profile: "",
      suggestedProfile: "",
      problems: [
        {
          line: 1,
          field: "version",
          text: "the file uses format version 2 and this Hopdeck reads up to version 1; update Hopdeck to open it",
        },
      ],
      tunnels: [],
    },
    {
      file: "staging.hopdeck.yaml",
      name: "staging",
      profile: "",
      suggestedProfile: "example-admin",
      problems: [],
      tunnels: [
        {
          id: "staging.hopdeck.yaml/admin-api",
          name: "admin-api",
          deck: "staging",
          deckFile: "staging.hopdeck.yaml",
          region: "eu-west-1",
          jumpHost: "tag:Name=staging-jump",
          remoteHost: "admin.api.example.internal",
          remotePort: 8443,
          localPort: 18443,
          deckPort: 18443,
          profile: null,
          state: "failed",
          reason: "No AWS profile is chosen for this deck file yet.",
        },
      ],
    },
    {
      file: "team-databases.hopdeck.yaml",
      name: "team-databases",
      profile: "example-dev",
      suggestedProfile: "example-dev",
      problems: [],
      tunnels: [
        {
          id: "team-databases.hopdeck.yaml/orders-db",
          name: "orders-db",
          deck: "team-databases",
          deckFile: "team-databases.hopdeck.yaml",
          region: "us-east-1",
          jumpHost: "i-1234567890abcdef0",
          remoteHost: "orders.db.example.internal",
          remotePort: 3306,
          localPort: 13306,
          deckPort: 13306,
          profile: "example-dev",
          state: "connected",
          reason: "",
        },
        {
          id: "team-databases.hopdeck.yaml/reports-db",
          name: "reports-db",
          deck: "team-databases",
          deckFile: "team-databases.hopdeck.yaml",
          region: "us-east-1",
          jumpHost: "tag:Role=jump-host",
          remoteHost: "reports.db.example.internal",
          remotePort: 5432,
          localPort: 15432,
          deckPort: 15432,
          profile: "example-dev",
          state: "idle",
          reason: "",
        },
      ],
    },
  ],
};

// Made-up AWS profiles for the sample.
const sampleProfileOptions: ProfileOption[] = [
  { name: "example-admin", kind: "role" },
  { name: "example-dev", kind: "keys" },
  { name: "example-sso", kind: "identity-center" },
];

// A profile source for the sample: it lists the made-up profiles and applies a choice
// to the sample snapshot, as the Go side would after saving it.
export function sampleProfiles(
  apply: (change: (snapshot: Snapshot) => Snapshot) => void,
): ProfileSource {
  return {
    async list() {
      return { profiles: sampleProfileOptions, error: "" };
    },
    async check(profile) {
      const none = { account: "", caller: "" };
      switch (profile) {
        case "example-dev":
          return {
            state: "valid",
            account: "123456789012",
            caller: "arn:aws:iam::123456789012:user/example-user",
            reason: "",
          };
        case "example-admin":
          return {
            state: "needs-mfa",
            ...none,
            reason: "This profile asks for an MFA code before it can be used.",
          };
        case "example-sso":
          return {
            state: "needs-sign-in",
            ...none,
            reason:
              "The IAM Identity Center sign-in of this profile has expired or has not happened yet.",
          };
        default:
          return {
            state: "error",
            ...none,
            reason: `The profile ${profile} is not in your AWS configuration files.`,
          };
      }
    },
    async choose(deckFile, profile) {
      apply((snapshot) => ({
        ...snapshot,
        decks: snapshot.decks.map((deck) =>
          deck.file !== deckFile
            ? deck
            : {
                ...deck,
                profile,
                tunnels: deck.tunnels.map((tunnel): Tunnel => ({
                  ...tunnel,
                  profile: profile || null,
                  state: "idle",
                  reason: profile
                    ? ""
                    : "No AWS profile is chosen for this deck file yet.",
                })),
              },
        ),
      }));
    },
  };
}

// A port source for the sample. On the made-up computer, port 15432 is busy with 15433
// suggested, the ports 49152 to 49251 are kept by the system, and every other port is
// free. A picked port is applied to the sample snapshot, as the Go side would after
// saving it.
export function samplePorts(
  apply: (change: (snapshot: Snapshot) => Snapshot) => void,
): PortSource {
  return {
    async check(tunnel) {
      const port = tunnel.localPort;
      if (port === 15432) {
        return {
          port,
          free: false,
          reason:
            "Port 15432 on this computer is already in use by another program. Pick another port for this tunnel, or close that program.",
          suggestion: 15433,
        };
      }
      if (port >= 49152 && port <= 49251) {
        return {
          port,
          free: false,
          reason: `This computer does not allow listening on port ${port}. On Windows that usually means the port is in a range the system keeps for itself, for example for Hyper-V. Pick another port for this tunnel.`,
          suggestion: 0,
        };
      }
      return { port, free: true, reason: "", suggestion: 0 };
    },
    async choose(tunnel, port) {
      if (port !== 0 && (port < minLocalPort || port > maxLocalPort)) {
        throw new Error(
          `the local port must be between ${minLocalPort} and ${maxLocalPort}`,
        );
      }
      apply((snapshot) => ({
        ...snapshot,
        decks: snapshot.decks.map((deck) => ({
          ...deck,
          tunnels: deck.tunnels.map((each): Tunnel =>
            each.id !== tunnel.id
              ? each
              : { ...each, localPort: port === 0 ? each.deckPort : port },
          ),
        })),
      }));
    },
  };
}

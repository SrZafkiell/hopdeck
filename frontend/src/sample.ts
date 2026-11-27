import type { Tunnel } from "./types";

// Made-up tunnels for development previews: open the dev server with ?sample in the
// address to see them. Every value is an example (the documented AWS example instance ID,
// .example.internal host names), and production builds leave this data out because only
// a development-only branch of App uses it.
export const sampleTunnels: Tunnel[] = [
  {
    id: "team-databases/orders-db",
    name: "orders-db",
    deck: "team-databases",
    region: "us-east-1",
    jumpHost: "i-1234567890abcdef0",
    remoteHost: "orders.db.example.internal",
    remotePort: 3306,
    localPort: 13306,
    profile: "example-dev",
    state: "connected",
    reason: "",
  },
  {
    id: "team-databases/reports-db",
    name: "reports-db",
    deck: "team-databases",
    region: "us-east-1",
    jumpHost: "tag:Role=jump-host",
    remoteHost: "reports.db.example.internal",
    remotePort: 5432,
    localPort: 15432,
    profile: "example-dev",
    state: "idle",
    reason: "",
  },
  {
    id: "staging/admin-api",
    name: "admin-api",
    deck: "staging",
    region: "eu-west-1",
    jumpHost: "tag:Name=staging-jump",
    remoteHost: "admin.api.example.internal",
    remotePort: 8443,
    localPort: 18443,
    profile: null,
    state: "failed",
    reason: "No AWS profile is chosen for this deck file yet.",
  },
];

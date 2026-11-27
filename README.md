# Hopdeck

A desktop app that opens AWS Session Manager tunnels through a jump host to private
services, such as a database, from a shared deck file.

> Status: early development. The window builds and runs; it does not open a tunnel yet.
> Hopdeck is the first deck of
> [Clouddecks](https://github.com/SrZafkiell/clouddecks). The plan, the decisions and the
> risk register live there; work is tracked in issues, milestones and one
> [project board](https://github.com/users/SrZafkiell/projects/3) across the
> repositories.

## What it will do

- An administrator writes a small deck file that lists the tunnels of a team and hands
  it out. The file holds no secret.
- A user drops the file into a folder, picks one of their AWS profiles and clicks
  Connect. One row per tunnel shows the credentials, the jump host, its agent, the
  tunnel and the local port.
- A session that drops comes back by itself. When the credentials expire, the window
  asks for an MFA code or opens the browser sign-in.
- The goal is that there is nothing else to install: no AWS CLI, no plugin.

## How it differs from an SSH tunnel manager

- The jump host has no inbound rule, no public address and no SSH key to hand out.
- Access is an IAM identity and a policy that allows a port forward through that one
  host and nothing else.
- No credential is ever written to disk by the app.

## Where things are

| What | Where |
|---|---|
| The plan of every module | [clouddecks/docs/plan](https://github.com/SrZafkiell/clouddecks/tree/main/docs/plan) |
| The overview and the decisions | [clouddecks/docs/PLAN.md](https://github.com/SrZafkiell/clouddecks/blob/main/docs/PLAN.md) |
| The risk register | [clouddecks/docs/RISKS.md](https://github.com/SrZafkiell/clouddecks/blob/main/docs/RISKS.md) |
| The steps of this repository | [Issues](https://github.com/SrZafkiell/hopdeck/issues) |

## License

Apache-2.0. See [LICENSE](LICENSE).

Hopdeck is an independent project. It is not affiliated with, or endorsed by, Amazon Web
Services.

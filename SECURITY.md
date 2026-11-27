# Security policy

Hopdeck handles AWS credentials and opens network tunnels on the user's computer, so security reports are welcome and are handled before any other work.

## Reporting a vulnerability

Report privately through GitHub's private vulnerability reporting:
https://github.com/SrZafkiell/hopdeck/security/advisories/new

A vulnerability in the shared CI workflows of Clouddecks is reported in
https://github.com/SrZafkiell/clouddecks/security/advisories/new. Please do not open a public issue, a discussion or a pull request for a vulnerability before a
fix is released.

A useful report says:

- which version, and on which operating system;
- what an attacker needs (a deck file they wrote, a process on the same computer,
  a position on the network, a compromised release channel, ...);
- the steps to reproduce it and what happens;
- what you expected instead.

Never put a real AWS account ID, access key, instance ID, host name or ARN in a report.
Use the documented examples instead (account `123456789012`, instance
`i-1234567890abcdef0`, access key `AKIAIOSFODNN7EXAMPLE`).

## What happens next

This is a project with one maintainer. The aim is to acknowledge a report within seven
days, agree on a fix and a disclosure date with the reporter, and publish a GitHub
security advisory with the fixed release. Reporters are credited unless they ask not to
be.

## Supported versions

Hopdeck has not been released yet. After the first release, only the latest release
receives security fixes.

## Scope

In scope: the code in this repository and the release artifacts it publishes, including
how Hopdeck handles credentials, deck files, local listening ports, its window and its
updates. The threat model is in the Clouddecks repository, in
[docs/threat-model](https://github.com/SrZafkiell/clouddecks/tree/main/docs/threat-model).

Out of scope: AWS services themselves (report those to AWS at
https://aws.amazon.com/security/vulnerability-reporting/), a computer that is already
under an attacker's control, and findings that need a deck file the user chose to trust
to contain something the documentation says a deck file may contain.

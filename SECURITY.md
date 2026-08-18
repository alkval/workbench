# Security policy

## Supported version

Security fixes currently target the latest commit on `main`.

## Reporting a vulnerability

Please use GitHub's private vulnerability reporting feature instead of opening
a public issue. Include the affected version, reproduction steps and expected
impact when possible.

## Deployment boundary

Workbench controls local processes and should not be exposed directly to the
public internet. Deploy it behind a private network such as Tailscale or another
authenticated VPN. Do not port-forward the Workbench listener from a router.

The browser API cannot submit arbitrary commands. Workbench only executes the
programs and arguments declared in the local service configuration. Keep that
configuration and the generated password file private.

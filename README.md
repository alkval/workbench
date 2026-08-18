# Workbench

Workbench is a private, self-hosted control panel for starting and stopping resource-heavy development and AI services on a Windows machine. It shows live CPU, memory, process and optional NVIDIA GPU telemetry, captures service output, and keeps an audit trail.

![Go](https://img.shields.io/badge/backend-Go-00ADD8?logo=go&logoColor=white)
![React](https://img.shields.io/badge/frontend-React%20%2B%20TypeScript-3178C6?logo=react&logoColor=white)
![License](https://img.shields.io/badge/license-MIT-green)

## Features

- Start, stop and restart only explicitly allowlisted applications
- Model dependencies such as an API that requires Ollama
- Define quick-start groups for complete stacks
- View service state, recent logs and machine/GPU utilization live
- Stop all managed services with one action
- Authenticate with a password stored locally outside the repository
- Record actions in a local SQLite audit database
- Run at sign-in through a windowless Windows Scheduled Task

Workbench never accepts arbitrary shell commands from the browser. Every executable, argument, working directory and dependency must be declared in the local service configuration.

## Architecture

```text
Browser
  |  HTTPS through your private reverse proxy or VPN
  v
Go HTTP server :8787
  |-- secure cookie authentication
  |-- allowlisted Windows process manager
  |-- Server-Sent Events for live updates
  |-- SQLite audit history
  |-- gopsutil CPU, RAM and process telemetry
  `-- nvidia-smi GPU telemetry (optional)

React + TypeScript + Vite UI
  `-- compiled and embedded in the Go executable
```

The deployment is a single executable plus a local JSON configuration, password file and data directory.

## Requirements

- Windows 10 or 11
- PowerShell 5.1 or newer
- Go and Node.js for building from source
- An NVIDIA driver providing `nvidia-smi` if GPU telemetry is desired

## Configure

Create your private configuration from the example:

```powershell
Copy-Item .\config\services.windows.example.json .\config\services.windows.json
```

Edit `config/services.windows.json` so every path and argument exactly matches the applications on your machine. This file is deliberately ignored by Git because it normally contains personal filesystem paths.

Each service can declare its executable, arguments, working directory, health check, startup timeout, dependencies, detection rules, stop behavior and log location.

## Build and install

Build from a regular PowerShell window:

```powershell
.\scripts\build.ps1
```

Then open PowerShell as Administrator:

```powershell
.\scripts\install-windows.ps1
```

The installer copies the application to `C:\ProgramData\Workbench`, creates a limited scheduled task at user sign-in, generates a dashboard password, adds a private-network firewall rule for TCP 8787, starts Workbench and verifies `/healthz`.

Save the generated password in a password manager. To choose a different password later:

```powershell
.\scripts\rotate-password.ps1
```

Passwords must contain at least 10 characters.

## Private access

This is an administrative control surface. Keep it behind a private VPN such as Tailscale, or a reverse proxy that is itself reachable only over your private network. Do not expose port 8787 directly to the public internet.

A typical private route is:

```text
Private DNS record -> VPN address -> HTTPS reverse proxy -> Windows host:8787
```

Use HTTPS at the browser-facing proxy so Workbench can issue its secure session cookie.

## Development

```powershell
cd web
npm install
npm run build
cd ..
go test .\...
go run .\cmd\workbench
```

The production build is available through `scripts/build.ps1`. Configuration is loaded from `services.json` beside the installed executable, while runtime data stays in its `data` directory.

## Uninstall

Remove the scheduled task and firewall rule while retaining application data:

```powershell
.\scripts\uninstall-windows.ps1
```

To remove the installed files and local data as well:

```powershell
.\scripts\uninstall-windows.ps1 -RemoveData
```

## Security

See [SECURITY.md](SECURITY.md) for the threat model, deployment guidance and vulnerability reporting instructions.

## License

MIT © Alexander Kvalvaag

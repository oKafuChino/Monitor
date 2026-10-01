# Komari

[English](./README.md) | [简体中文](./README_zh-cn.md)

A self-hosted server monitoring panel with real-time metrics, history charts, Ping monitoring and notifications. The frontend is bundled into the Go server, so deployment requires only one service.

This version uses a fixed built-in interface. Third-party themes, plugins, web terminals and remote command execution are removed. Remote file management, including file writes, and SQL administration remain available. The operating system's SSH service is unaffected.

## One-command installation

Obtain this repository's source code and run the installer from its root. It checks dependencies, builds the image, starts the service and waits for the web interface to respond.

Linux (requires Docker, Docker Compose, and curl or wget):

```sh
bash install.sh
```

On Debian/Ubuntu without Docker, use `sudo bash install.sh --install-docker` to install dependencies from Docker's official APT repository.

Windows (start Docker Desktop in Linux container mode first):

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\install.ps1
```

Append `--port 8080` (Linux) or `-Port 8080` (Windows) to choose a port. Use `--check` / `-Check` for a read-only preflight, or `--help` / `-Help` for all options.

Alternatively, deploy manually with `docker compose up -d --build`.

Open `http://<server-ip>:25774` and follow the installation wizard to create an administrator account and configure the site.

- Data is stored in the repository's `data/` directory and persists across container replacement.
- After a successful deployment, the installer saves the port to `.env` for subsequent runs and preserves other settings.
- View logs: `docker compose logs -f monitor`.
- Stop the service: `docker compose down`.

## Build from source

Requires Node.js 24+, `tar`, and Docker. Run all commands from the repository root:

```sh
npm run setup
npm run build -- --docker
npm start -- --docker
```

Docker supplies the Go/CGO toolchain. The resulting Linux binary is `dist/komari`; it includes the frontend. Open `http://127.0.0.1:25774` to complete setup. This startup method binds to localhost by default and stores data in `data/`.

If Go 1.25.0 (as specified in `go.mod`) and a C compiler are installed locally, build and run without Docker:

```sh
npm run setup
npm run build
npm start
```

The native Windows binary is `dist/komari.exe`; other native builds produce `dist/komari`.

## Add monitored servers

1. Sign in to the panel using an address reachable from the servers you want to monitor.
2. Open **Server List**, add a node and copy its installation command for the target operating system.
3. Run the command on that server. Once its Agent connects, monitoring data appears in the panel.

## Update and back up

Download a backup from the administration panel before updating. After updating this repository's source code, rerun the installer. For source-based installations, stop the running service before rebuilding and starting it. Keep the `data/` directory; do not replace it with an empty directory.

See [LICENSE](./LICENSE) for licensing terms.

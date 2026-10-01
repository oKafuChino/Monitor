# Komari

[English](./README.md) | [简体中文](./README_zh-cn.md)

A self-hosted server monitoring panel with real-time metrics, history charts, Ping monitoring and notifications. The frontend is bundled into the Go server, so deployment requires only one service.

This version uses a fixed built-in interface. Third-party themes, plugins, web terminals and remote command execution are removed. Remote file management, including file writes, and SQL administration remain available. The operating system's SSH service is unaffected.

## Install with Docker Compose

Install Docker and Docker Compose, obtain this repository's source code, then run the following from the repository root:

```sh
docker compose up -d --build
```

Open `http://<server-ip>:25774` and follow the installation wizard to create an administrator account and configure the site.

- Data is stored in the repository's `data/` directory and persists across container replacement.
- To change the exposed port, set `MONITOR_PORT=8080` in a `.env` file at the repository root before starting.
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

Download a backup from the administration panel before updating. After updating this repository's source code, rerun the installation/build commands for your chosen method. For source-based installations, stop the running service before rebuilding. Keep the `data/` directory; do not replace it with an empty directory.

See [LICENSE](./LICENSE) for licensing terms.

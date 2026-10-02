# Komari

[English](./README.md) | [简体中文](./README_zh-cn.md)

A self-hosted monitoring panel with real-time metrics, history charts and Ping monitoring. The frontend is embedded in the Go server, so deployment needs only one service.

**The server supports Linux deployments only.** This version uses a fixed built-in interface and removes third-party themes, plugins, web terminals, remote command execution, remote file management and notifications. SQL administration remains available.

Repository: [oKafuChino/Monitor](https://github.com/oKafuChino/Monitor). The default installation branch is `main`.

Optional single-node sharing uses a separate listener and domain, with revocable links. See [configuration and deployment](docs/临时分享节点部署.md). Keep the main listener on loopback/private networking; a different sharing port does not hide a public main port.

Security changes, deployment credentials, trusted proxies, Agent configuration and restore compatibility are documented in [the security implementation record](docs/安全修复实施结果.md). Backend and security regression verification remains pending.

## 1. Requirements

- A Linux server and a root account or an account with sudo access.
- Access to GitHub, Docker registries and build dependency sources. The first installation compiles both frontend and backend inside Docker; host installations of Node.js and Go are not required.
- TCP port `25774` is mapped to loopback by default. Use an SSH tunnel or an access-controlled reverse proxy for remote access; the script does not change firewall rules.
- On Debian/Ubuntu, `--install-docker` can install missing Git, Docker and Compose dependencies. On other Linux distributions, install Git, Docker, Docker Compose, and curl or wget first.

If curl is missing on Debian/Ubuntu:

```sh
sudo apt-get update
sudo apt-get install -y ca-certificates curl
```

## 2. Install using curl

Download and run the installer. It fetches the complete source from this repository's `main` branch:

```sh
curl -fsSL --retry 3 https://raw.githubusercontent.com/oKafuChino/Monitor/main/install.sh -o /tmp/monitor-install.sh &&
sudo bash /tmp/monitor-install.sh --install-docker --dir /opt/monitor
```

Piped execution is also supported:

```sh
curl -fsSL --retry 3 https://raw.githubusercontent.com/oKafuChino/Monitor/main/install.sh |
  sudo bash -s -- --install-docker --dir /opt/monitor
```

Omit `--install-docker` if Git, Docker and Compose are already available. For a new installation, the destination must be absent or empty; unrelated nonempty directories are not overwritten. Save a standalone installer in `/tmp`, outside the intended destination.

The installer checks the environment, downloads source when necessary, builds the integrated image, starts the Compose service, waits for HTTP readiness and then saves the port to `.env`.

Use `ssh -L 25774:127.0.0.1:25774 user@server`, then open `http://127.0.0.1:25774`. Read `data/setup-token` locally on the server and enter it when prompted to create the administrator or import a backup. The token is removed after setup. For a domain/HTTPS reverse proxy, configure the actual proxy peer in `KOMARI_TRUSTED_PROXIES` and ensure it supports WebSocket connections. Set `MONITOR_BIND` only when you need another protected network interface.

## 3. Directory, port and version settings

With `/opt/monitor` as the installation directory:

| Path | Purpose |
| --- | --- |
| `/opt/monitor/` | Git checkout and deployment configuration |
| `/opt/monitor/.env` | Port and other settings; the installer only changes `MONITOR_PORT` |
| `/opt/monitor/data/` | Persistent databases and site files |

Rebuilding the image, replacing containers or running `docker compose down` does not intentionally delete this `data/` directory.

Choose a port during installation:

```sh
sudo bash /tmp/monitor-install.sh --install-docker --dir /opt/monitor --port 8080
```

If already installed and source has not changed, reuse the existing image when changing only the port:

```sh
sudo bash /opt/monitor/install.sh --port 8080 --skip-build
```

Use the chosen port through your tunnel or reverse proxy. Without `--port`, the installer uses the environment/existing `.env` value, falling back to `25774`.

| Option | Purpose |
| --- | --- |
| `--dir /opt/monitor` | Destination. Defaults to an existing local checkout, otherwise `/opt/monitor` for root or `~/monitor` for a regular user |
| `--repo oKafuChino/Monitor` | GitHub repository in `OWNER/REPO` form |
| `--ref main` | Branch or tag; defaults to `main` initially, then reuses the saved ref |
| `--port 8080` | Port to save after successful deployment |
| `--install-docker` | Install missing Git/Docker/Compose on Debian/Ubuntu; conflicting packages are not automatically removed |
| `--update` | Fetch and fast-forward source, then rebuild and deploy |
| `--skip-build` | Use the existing image; unavailable for a new source download |
| `--timeout 300` | HTTP readiness timeout after startup; default 180 seconds, excluding build time |
| `--check` | Read-only checks of existing source, dependencies and configuration; no download or deployment |
| `--help` | Show usage |

The selected branch/tag must contain the integrated frontend, Dockerfile and installer. The repository and ref are recorded in Git configuration for subsequent updates.

## 4. Add monitored nodes

1. Sign in through an address reachable from the servers to be monitored. Do not generate Agent installation commands through an unreachable localhost address.
2. Open **Server List**, add a node and copy its Agent installation command.
3. Run the command on the server to be monitored.
4. Wait for the node to connect and display its metrics.

The probe is integrated in `komari-agent/` and released with this project. Linux probes report block-device read/write rates; other probe platforms report I/O as unsupported. See [probe integration and build instructions](komari-agent/INTEGRATION.md).

## 5. Updates and service management

Download a backup from the administration panel before updating, and retain `.env` and `data/`.

Fetch the selected ref and redeploy:

```sh
sudo bash /opt/monitor/install.sh --update
```

To use the latest installer as well:

```sh
curl -fsSL --retry 3 https://raw.githubusercontent.com/oKafuChino/Monitor/main/install.sh -o /tmp/monitor-install.sh &&
sudo bash /tmp/monitor-install.sh --dir /opt/monitor --update
```

Updates must be fast-forward merges; downgrades are rejected. Uncommitted edits, untracked files or a mismatched origin stop the update; source is never forcibly reset. Commit or back up your changes before retrying. Without `--update`, the script only redeploys the current source and does not fetch a new version.

Run these commands in the installation directory, for example after `cd /opt/monitor`:

| Action | Command |
| --- | --- |
| Status | `sudo docker compose ps` |
| Logs | `sudo docker compose logs --tail 100 -f monitor` |
| Restart | `sudo docker compose restart monitor` |
| Stop | `sudo docker compose stop monitor` |
| Start | `sudo docker compose start monitor` |
| Remove containers, retain the data directory | `sudo docker compose down` |

## 6. Troubleshooting

- **GitHub download fails:** check DNS and connectivity to `raw.githubusercontent.com` and `github.com`. Retry after fixing connectivity; downloads are staged before replacing an installation directory.
- **Docker connection or permission failure:** ensure the daemon is running, check `sudo systemctl status docker`, and run as a Docker-authorized user or with sudo.
- **Port already in use:** choose another `--port`. The script does not stop other services holding a port.
- **Image build fails:** inspect build output, dependency downloads and host resources. A failed build does not proceed to service startup/recreation.
- **Readiness timeout:** inspect `docker compose logs --tail 100 monitor` in the installation directory and increase `--timeout` if needed. A failed deployment does not save the new port; retry with the same arguments after fixing the problem.
- **Deployment lock remains:** verify no installer is running before removing an empty `.deploy.lock` or sibling `.directory-name.install.lock` left by an interrupted process.

## 7. Manual deployment and source builds

With a complete checkout and Docker, run from the repository root:

```sh
docker compose up -d --build
```

Native source builds require Linux, Node.js 24+, `tar`, Go 1.25.0 (see `go.mod`) and a C compiler:

```sh
npm run setup
npm run build
npm start
```

Alternatively, keep Node.js and `tar` on the host and use Docker for Go/CGO:

```sh
npm run setup
npm run build -- --docker
npm start -- --docker
```

The Linux binary is `dist/komari` and includes the frontend. `npm start` binds to `127.0.0.1:25774` by default for local access; use Compose for an exposed service. For frontend development, use `npm run dev -- --docker`.

See [LICENSE](./LICENSE) for licensing terms.

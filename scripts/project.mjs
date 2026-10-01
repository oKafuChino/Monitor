// Root project entry point. The frontend lockfile remains authoritative.
import fs from "node:fs";
import path from "node:path";
import net from "node:net";
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL("../", import.meta.url));
const frontend = path.join(root, "komari-web");
const [command = "help", ...options] = process.argv.slice(2);
if (options.some((option) => option !== "--docker" && !(command === "dev" && option === "--no-build"))) {
  throw new Error("Supported options: --docker; dev also accepts --no-build");
}
const docker = options.includes("--docker");
const skipBuild = options.includes("--no-build");
const port = Number(process.env.MONITOR_PORT || 25774);
const devPort = Number(process.env.MONITOR_DEV_PORT || 5173);
for (const value of [port, devPort]) {
  if (!Number.isInteger(value) || value < 1 || value > 65535) throw new Error("Invalid local port");
}
const goVersion = fs.readFileSync(path.join(root, "go.mod"), "utf8").match(/^go\s+(\S+)/m)[1];
const goImage = `golang:${goVersion}-bookworm`;
const binaryName = "komari";
const backendCommands = new Set(["build", "build-server", "dev", "start", "test", "test-server", "check"]);
if (process.platform !== "linux" && !docker && backendCommands.has(command)) {
  throw new Error("Native server builds and execution require Linux. Use Linux or --docker for Linux containers.");
}
const binary = path.join(root, "dist", binaryName);
const npmCLI = process.env.npm_execpath || path.join(path.dirname(process.execPath), "node_modules/npm/bin/npm-cli.js");

function launch(executable, args, settings = {}) {
  return spawn(executable, args, { cwd: root, stdio: "inherit", windowsHide: true, ...settings });
}
function completed(child, label) {
  return new Promise((resolve, reject) => {
    child.once("error", (error) => reject(new Error(`${label}: ${error.message}`)));
    child.once("exit", (code, signal) => code === 0 ? resolve() : reject(new Error(`${label} exited with ${signal || code}`)));
  });
}
const run = (executable, args, settings) => completed(launch(executable, args, settings), executable);
const npm = (...args) => run(process.execPath, [npmCLI, ...args], { cwd: frontend });

function go(args, project = ".") {
  if (!docker) return run("go", args, { cwd: path.join(root, project) });
  return run("docker", [
    "run", "--rm",
    "--mount", `type=bind,source=${root.replace(/[\\/]$/, "")},target=/workspace`,
    "--mount", "type=volume,source=monitor-go-cache,target=/go",
    "--mount", "type=volume,source=monitor-go-build-cache,target=/root/.cache/go-build",
    "-w", project === "." ? "/workspace" : `/workspace/${project}`, "-e", "CGO_ENABLED=1", goImage, "go", ...args,
  ]);
}
async function buildAgent() {
  const goos = process.env.GOOS || (docker ? "linux" : process.platform === "win32" ? "windows" : process.platform);
  const goarch = process.env.GOARCH || (process.arch === "x64" ? "amd64" : process.arch === "ia32" ? "386" : process.arch);
  const name = `komari-agent-${goos}-${goarch}${goos === "windows" ? ".exe" : ""}`;
  fs.mkdirSync(path.join(root, "dist/agent"), { recursive: true });
  await go(["build", "-trimpath", "-o", `../dist/agent/${name}`, "."], "komari-agent");
}
async function testAgent() {
  await go(["test", "./..."], "komari-agent");
  await go(["test", "-race", "./monitoring/...", "./server/..."], "komari-agent");
}
async function buildWeb() {
  await npm("run", "build");
  await run(process.execPath, [path.join(root, "scripts/embed-frontend.mjs")]);
}
function requireEmbeddedUI() {
  for (const name of ["dist.tar.zst", "komari-theme.json"]) {
    if (!fs.existsSync(path.join(root, "web/public/defaultTheme", name))) {
      throw new Error("Build the embedded UI first: npm run build:web");
    }
  }
}
async function buildServer() {
  requireEmbeddedUI();
  fs.mkdirSync(path.dirname(binary), { recursive: true });
  await go(["build", "-trimpath", "-o", `dist/${binaryName}`, "."]);
}
async function testWeb() {
  await npm("run", "test:ui");
  await run(process.execPath, ["--test", path.join(root, "scripts/integration.test.mjs")]);
}
function serverProcess(runtimeDir, development = false) {
  if (!fs.existsSync(binary)) throw new Error("Build the application first: npm run build" + (docker ? " -- --docker" : ""));
  fs.mkdirSync(runtimeDir, { recursive: true });
  if (!docker) {
    return { child: launch(binary, ["server", "--listen", `127.0.0.1:${port}`], { cwd: runtimeDir }) };
  }
  const containerName = `monitor-${development ? "dev" : "local"}-${process.pid}`;
  const child = launch("docker", [
    "run", "--rm", "--name", containerName,
    "--mount", `type=bind,source=${path.dirname(binary)},target=/artifacts,readonly`,
    "--mount", `type=bind,source=${runtimeDir},target=/app`,
    "-w", "/app", "-p", `127.0.0.1:${port}:25774`, goImage,
    `/artifacts/${binaryName}`, "server", "--listen", "0.0.0.0:25774",
  ]);
  return { child, containerName };
}
async function assertPortAvailable(port) {
  await new Promise((resolve, reject) => {
    const socket = net.createServer();
    socket.once("error", () => reject(new Error(`Port ${port} is already in use; stop the existing local service first.`)));
    socket.listen(port, "127.0.0.1", () => socket.close(resolve));
  });
}
async function waitForBackend(child) {
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    if (child.exitCode !== null) throw new Error("Backend exited before becoming ready");
    try {
      const response = await fetch(`http://127.0.0.1:${port}/`, { signal: AbortSignal.timeout(1000) });
      await response.body?.cancel();
      if (response.ok) return;
    } catch { /* Startup may still be preparing the embedded archive and database. */ }
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
  throw new Error("Backend did not start within 30 seconds");
}
async function serve(development) {
  await assertPortAvailable(port);
  if (development) await assertPortAvailable(devPort);
  const runtimeDir = path.resolve(process.env.MONITOR_RUNTIME_DIR || (development ? path.join(root, "dist/dev") : root));
  const server = serverProcess(runtimeDir, development);
  const children = [server.child];
  let stopping = false;
  const stop = async () => {
    if (stopping) return;
    stopping = true;
    if (server.containerName) {
      await run("docker", ["stop", "--timeout", "5", server.containerName]).catch(() => {});
    }
    for (const child of children) if (child.exitCode === null) child.kill();
  };
  const interrupted = () => { void stop(); };
  process.on("SIGINT", interrupted);
  process.on("SIGTERM", interrupted);
  // Attach exit/error handlers immediately, including during readiness polling.
  const backendDone = completed(server.child, "Backend").catch((error) => {
    if (!stopping) throw error;
  });
  const failure = backendDone.then(() => { if (!stopping) throw new Error("Backend stopped"); });
  try {
    await Promise.race([waitForBackend(server.child), failure]);
    if (stopping) return;
    console.log(`Backend and embedded UI: http://127.0.0.1:${port} (data: ${runtimeDir})`);
    if (development) {
      const vite = launch(process.execPath, [path.join(frontend, "node_modules/vite/bin/vite.js"), "--host", "127.0.0.1", "--port", String(devPort), "--strictPort"], {
        cwd: frontend,
        env: { ...process.env, VITE_API_TARGET: `http://127.0.0.1:${port}` },
      });
      children.push(vite);
      console.log(`Development UI with API/WebSocket proxy: http://127.0.0.1:${devPort}`);
      await Promise.race([backendDone, completed(vite, "Frontend")]);
    } else {
      await backendDone;
    }
  } catch (error) {
    if (!stopping) throw error;
  } finally {
    await stop();
    process.off("SIGINT", interrupted);
    process.off("SIGTERM", interrupted);
  }
}

try {
  switch (command) {
    case "setup": await npm("ci", "--no-audit", "--no-fund"); break;
    case "build-web": await buildWeb(); break;
    case "build-server": await buildServer(); break;
    case "build-agent": await buildAgent(); break;
    case "test-agent": await testAgent(); break;
    case "build": await buildWeb(); await buildServer(); await buildAgent(); break;
    case "dev": if (!skipBuild) { await buildWeb(); await buildServer(); } await serve(true); break;
    case "start": await serve(false); break;
    case "test-web": await buildWeb(); await testWeb(); break;
    case "test-server": requireEmbeddedUI(); await go(["test", "./..."]); break;
    case "lint": await npm("run", "lint"); break;
    case "check":
      await npm("run", "lint");
      // fall through to the full source-to-embedded regression pipeline
    case "test": await buildWeb(); await testWeb(); await go(["test", "./..."]); await testAgent(); break;
    case "help": console.log("npm run setup | dev | build | start | test | check; append -- --docker to use containerized Go/CGO"); break;
    default: throw new Error(`Unknown command: ${command}`);
  }
} catch (error) {
  console.error(error.message);
  if (!docker && /spawn go ENOENT/.test(error.message)) console.error("Go/CGO is unavailable. Retry with -- --docker, or install the Go version from go.mod and a C compiler.");
  process.exitCode = 1;
}

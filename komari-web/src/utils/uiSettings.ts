type UISettings = Record<string, unknown>;

let pending: Promise<unknown> = Promise.resolve();

export async function readUISettings(): Promise<UISettings> {
  const response = await fetch("/api/admin/ui/settings", { cache: "no-store" });
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  const result = await response.json();
  if (!result.data || typeof result.data !== "object" || Array.isArray(result.data)) {
    throw new Error("Invalid UI settings response");
  }
  return result.data;
}

// Send only changed top-level keys; the server merges them in a transaction.
// Cross-tab locking also serializes read/modify/write of the same settings.
export function saveUISettings(
  patch: UISettings | ((current: UISettings) => UISettings),
) {
  const save = async () => {
    const values = typeof patch === "function" ? patch(await readUISettings()) : patch;
    const response = await fetch("/api/admin/ui/settings", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(values),
      keepalive: true,
    });
    if (!response.ok) {
      const result = await response.json().catch(() => null);
      throw new Error(result?.message || `HTTP ${response.status}`);
    }
  };
  const task = pending.catch(() => {}).then(() =>
    navigator.locks ? navigator.locks.request("komari-ui-settings", save) : save(),
  );
  pending = task;
  return task;
}

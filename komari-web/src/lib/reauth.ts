// Secrets exist only for the submitted request. They are never persisted.
let pending: Promise<Record<string, string> | null> | undefined;

function askCredential(message: string): Promise<Record<string, string> | null> {
  if (pending) return pending;
  pending = new Promise((resolve) => {
    const setup = message.includes("Setup token");
    const factor = message.includes("2FA");
    const dialog = document.createElement("dialog");
    dialog.style.cssText = "padding:24px;border:1px solid #888;border-radius:12px;max-width:420px;background:var(--color-panel-solid,Canvas);color:CanvasText";
    const form = document.createElement("form");
    form.method = "dialog";
    const label = document.createElement("label");
    label.textContent = setup ? "部署者验证 / Setup token" : factor ? "二次验证 / Two-factor code" : "重新验证 / Current password";
    const hint = document.createElement("p");
    hint.textContent = setup ? "输入服务器本地 data/setup-token 文件中的一次性凭证。" : "此操作涉及凭证或完整数据，请重新验证身份。";
    const input = document.createElement("input");
    input.type = "password";
    input.autocomplete = "off";
    input.required = true;
    input.style.cssText = "display:block;width:100%;box-sizing:border-box;margin:12px 0;padding:8px";
    label.append(input);
    const cancel = document.createElement("button");
    cancel.type = "button"; cancel.textContent = "取消 / Cancel";
    const submit = document.createElement("button");
    submit.type = "submit"; submit.textContent = "验证 / Verify";
    submit.style.marginLeft = "12px";
    const finish = (result: Record<string, string> | null) => {
      input.value = ""; dialog.remove(); pending = undefined; resolve(result);
    };
    cancel.onclick = () => finish(null);
    dialog.addEventListener("cancel", () => finish(null), { once: true });
    form.onsubmit = (event) => {
      event.preventDefault();
      const result = { [setup ? "X-Setup-Token" : factor ? "X-2FA-Code" : "X-Reauth-Password"]: input.value };
      if (setup) (window as typeof window & { __setupToken?: string }).__setupToken = input.value;
      finish(result);
    };
    form.append(label, hint, cancel, submit); dialog.append(form);
    document.body.append(dialog); dialog.showModal(); input.focus();
  });
  return pending;
}

export function installReauthenticationFetch(): void {
  const original = window.fetch.bind(window);
  window.fetch = async (input, init) => {
    const retryInput = input instanceof Request ? input.clone() : input;
    const response = await original(input, init);
    const rawURL = input instanceof Request ? input.url : String(input);
    const url = new URL(rawURL, window.location.href);
    if (url.origin !== window.location.origin || !url.pathname.startsWith("/api/") || !response.headers.get("Content-Type")?.includes("json")) return response;
    let payload: { message?: string; error?: string | { message?: string } };
    try { payload = await response.clone().json(); } catch { return response; }
    const message = typeof payload.error === "string" ? payload.error : payload.error?.message ?? payload.message ?? "";
    if (!/2FA code is required|Reauthentication password is required|Setup token is required/.test(message)) return response;
    const credentials = await askCredential(message);
    if (!credentials) return response;
    const headers = new Headers(init?.headers ?? (input instanceof Request ? input.headers : undefined));
    for (const [name, value] of Object.entries(credentials)) headers.set(name, value);
    return original(retryInput, { ...init, headers });
  };
}

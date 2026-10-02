import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";

// Deployment credentials are fetched explicitly, never from the redacted node list.
export function useDeploymentToken(uuid: string) {
  const [token, setToken] = useState("");
  const [loading, setLoading] = useState(false);
  const request = useRef<AbortController | null>(null);

  const clear = useCallback(() => {
    request.current?.abort();
    request.current = null;
    setToken("");
    setLoading(false);
  }, []);

  useEffect(() => {
    clear();
    return () => {
      request.current?.abort();
      request.current = null;
    };
  }, [uuid, clear]);

  const reveal = async () => {
    if (request.current) return;
    const controller = new AbortController();
    request.current = controller;
    setToken("");
    setLoading(true);
    try {
      const response = await fetch(`/api/admin/client/${encodeURIComponent(uuid)}/token`, {
        cache: "no-store",
        signal: controller.signal,
      });
      const payload = await response.json().catch(() => null);
      if (!response.ok || payload?.status !== "success") {
        throw new Error(payload?.message || "读取部署凭证失败，请重新验证后重试。");
      }
      const value = payload.token ?? payload.data?.token;
      if (typeof value !== "string" || !value.trim()) {
        throw new Error("部署凭证为空，无法生成安装命令，请检查节点凭证。");
      }
      if (request.current === controller) setToken(value.trim());
    } catch (error) {
      if (request.current === controller) {
        toast.error(error instanceof Error ? error.message : "读取部署凭证失败");
      }
    } finally {
      if (request.current === controller) {
        request.current = null;
        setLoading(false);
      }
    }
  };

  return { token, loading, reveal, clear };
}

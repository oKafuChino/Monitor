import { useCallback, useState } from "react";
import { useAccount } from "@/contexts/AccountContext";

export function useInlineReauthentication() {
  const { account, loading } = useAccount();
  const [value, setValue] = useState("");
  const [error, setError] = useState("");
  const twoFactor = Boolean(account?.["2fa_enabled"]);
  const reset = useCallback(() => { setValue(""); setError(""); }, []);
  const takeHeaders = (): Record<string, string> => {
    setValue("");
    setError("");
    return { [twoFactor ? "X-2FA-Code" : "X-Reauth-Password"]: value };
  };
  return { value, setValue, error, setError, twoFactor, reset, takeHeaders,
    ready: !loading && Boolean(account?.logged_in) && value.length > 0,
    available: !loading && Boolean(account?.logged_in) };
}

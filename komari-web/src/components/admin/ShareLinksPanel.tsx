import { useEffect, useState } from "react";
import { Button, Flex, Select, TextField } from "@radix-ui/themes";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { useRPC2 } from "../../contexts/RPC2Context";

interface Link { id: string; node_name: string; token_mask: string; duration: string; expires_at: string | null; created_at: string; status: string }
export default function ShareLinksPanel() {
  const { t } = useTranslation();
  const { client } = useRPC2();
  const [nodes, setNodes] = useState<{ uuid: string; name: string; hidden: boolean }[]>([]);
  const [links, setLinks] = useState<Link[]>([]);
  const [node, setNode] = useState("");
  const [duration, setDuration] = useState("1d");
  const [url, setUrl] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [publicBase, setPublicBase] = useState("");
  const [savedPublicBase, setSavedPublicBase] = useState("");
  useEffect(() => {
    let mounted = true;
    void Promise.all([client.call<object, { uuid: string; name: string; hidden: boolean }[]>("admin:listClients", {}), client.call<object, Link[]>("admin:listShareLinks", {}), client.call<object, { share_public_base_url?: string }>("admin:getSettings", {})]).then(([n, l, settings]) => {
      if (mounted) { setNodes(n.filter(item => !item.hidden)); setLinks(l); setPublicBase(settings.share_public_base_url ?? ""); setSavedPublicBase(settings.share_public_base_url ?? ""); }
    }).catch(() => { if (mounted) setError(t("share.action_error")); });
    return () => { mounted = false; };
  }, [client, t]);
  const savePublicBase = async () => {
    setBusy(true); setError("");
    try {
      const base = publicBase.trim();
      await client.call("admin:editSettings", { share_public_base_url: base });
      setPublicBase(base); setSavedPublicBase(base); setUrl("");
      toast.success(t("share.saved"));
    } catch (e) { setError(e instanceof Error ? e.message : t("share.action_error")); } finally { setBusy(false); }
  };
  const create = async () => {
    setBusy(true); setUrl(""); setError("");
    try {
      const result = await client.call<object, { url: string }>("admin:createShareLink", { uuid: node, duration });
      setUrl(result.url); setLinks(await client.call<object, Link[]>("admin:listShareLinks", {}));
      toast.success(t("share.created"));
    } catch (e) { setError(e instanceof Error ? e.message : t("share.action_error")); } finally { setBusy(false); }
  };
  const revoke = async (id: string) => {
    setBusy(true);
    try { await client.call("admin:revokeShareLink", { id }); setUrl(""); setLinks(await client.call<object, Link[]>("admin:listShareLinks", {})); toast.success(t("share.revoked")); }
    catch { setError(t("share.action_error")); } finally { setBusy(false); }
  };
	const resetSessions = async (id: string) => {
		setBusy(true);
		try { await client.call("admin:resetShareSessions", { id }); toast.success("访问会话已重置，访客可重新打开分享链接"); }
		catch { setError(t("share.action_error")); } finally { setBusy(false); }
	};
  return <div className="flex flex-col gap-4 w-full">
    <p className="text-sm text-muted-foreground">{t("share.setup")}</p>
    <label htmlFor="share-public-base" className="text-sm font-medium">{t("share.public_base")}</label>
    <Flex gap="2" wrap="wrap">
      <TextField.Root id="share-public-base" value={publicBase} onChange={e => setPublicBase(e.target.value)} placeholder="https://share.example.com" className="flex-1" disabled={busy} />
      <Button disabled={busy || publicBase === savedPublicBase} onClick={() => void savePublicBase()}>{t("share.save")}</Button>
    </Flex>
    <p className="text-sm text-muted-foreground">{t("share.public_base_help")}</p>
    <Flex gap="3" wrap="wrap">
      <Select.Root value={node} onValueChange={setNode}><Select.Trigger placeholder={t("share.select_node")} /><Select.Content>{nodes.map(n => <Select.Item key={n.uuid} value={n.uuid}>{n.name}</Select.Item>)}</Select.Content></Select.Root>
      <Select.Root value={duration} onValueChange={setDuration}><Select.Trigger aria-label={t("share.duration")} /><Select.Content>{["1d", "1w", "1mo", "forever"].map(d => <Select.Item key={d} value={d}>{t(`share.${d}`)}</Select.Item>)}</Select.Content></Select.Root>
      <Button disabled={!node || !savedPublicBase || publicBase.trim() !== savedPublicBase || busy} onClick={() => void create()}>{t("share.create")}</Button>
    </Flex>
    {error && <p role="alert" className="text-red-500 text-sm">{error}</p>}
    {url && <div className="flex flex-col gap-2"><p className="text-sm">{t("share.once")}</p><Flex gap="2"><TextField.Root readOnly value={url} className="flex-1" aria-label={t("share.link")} /><Button onClick={() => { void navigator.clipboard.writeText(url).then(() => toast.success(t("share.copied"))).catch(() => toast.error(t("share.action_error"))); }}>{t("share.copy")}</Button></Flex></div>}
    <div className="overflow-x-auto"><table className="w-full text-sm"><thead><tr>{["node", "duration", "created_at", "expires", "state"].map(key => <th className="p-2 text-left" key={key}>{t(`share.${key}`)}</th>)}<th /></tr></thead><tbody>{links.map(l => <tr key={l.id} className="border-t"><td className="p-2">{l.node_name}<br /><span className="text-xs opacity-60">{l.token_mask}</span></td><td className="p-2">{t(`share.${l.duration}`)}</td><td className="p-2">{new Date(l.created_at).toLocaleString()}</td><td className="p-2">{l.expires_at ? new Date(l.expires_at).toLocaleString() : t("share.forever")}</td><td className="p-2">{t(`share.${l.status}`)}</td><td className="p-2">{l.status === "active" && <Flex gap="2"><Button variant="soft" disabled={busy} onClick={() => void resetSessions(l.id)}>重置访问会话</Button><Button color="red" variant="soft" disabled={busy} onClick={() => void revoke(l.id)}>{t("share.revoke")}</Button></Flex>}</td></tr>)}</tbody></table>{!links.length && <p className="text-sm opacity-60">{t("share.no_links")}</p>}</div>
  </div>;
}

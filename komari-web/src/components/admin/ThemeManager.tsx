import { useCallback, useEffect, useRef, useState } from "react";
import { Badge, Button, Callout, Card, Checkbox, Dialog, Flex, Heading, Select, Text, TextArea } from "@radix-ui/themes";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import ConfigFormTabs, { type ConfigFormItem } from "@/components/admin/ConfigFormTabs";
import UploadDialog from "@/components/UploadDialog";
import { createChunkUploadTask, type ChunkUploadTask } from "@/lib/chunkUpload";
import { usePublicInfo } from "@/contexts/PublicInfoContext";
import { resolveI18nText, type I18nText } from "@/utils/i18nText";

type InstalledTheme = {
  short: string;
  name: I18nText;
  version: string;
  configuration?: { type?: string; data?: unknown };
};
type ThemeList = { active: string; themes: InstalledTheme[] };

async function themeRequest<T>(path: string, method = "GET", body?: unknown): Promise<T> {
  const response = await fetch(`/api/admin/theme/${path}`, {
    method,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const payload = await response.json();
  if (!response.ok || payload.status !== "success") throw new Error(payload.message || `HTTP ${response.status}`);
  return payload.data as T;
}

function ThemeSettings({ theme }: { theme: InstalledTheme }) {
  const { t, i18n } = useTranslation();
  const [values, setValues] = useState<Record<string, unknown>>({});
  const [raw, setRaw] = useState("{}");
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const managed = (!theme.configuration?.type || theme.configuration.type === "managed") && Array.isArray(theme.configuration?.data);
  // Built-in textbox fields render HTML. Custom manifests only get plain text
  // headings inside the administrator interface, never executable HTML panels.
  const items = managed ? (theme.configuration!.data as ConfigFormItem[]).map((item) =>
    item.type === "textbox" ? { ...item, type: "title" } : item,
  ) : [];
  useEffect(() => {
    let disposed = false;
    themeRequest<Record<string, unknown>>(`settings?theme=${encodeURIComponent(theme.short)}`).then((data) => {
      if (disposed) return;
      setValues(data);
      setRaw(JSON.stringify(data, null, 2));
      setReady(true);
    }).catch((err) => { if (!disposed) setError(String(err)); });
    return () => { disposed = true; };
  }, [theme.short]);
  const save = async () => {
    setBusy(true);
    try {
      const data: unknown = managed ? values : JSON.parse(raw);
      if (!data || typeof data !== "object" || Array.isArray(data)) throw new Error(t("localThemes.objectRequired"));
      await themeRequest(`settings?theme=${encodeURIComponent(theme.short)}`, "PUT", data);
      toast.success(t("settings.settings_saved"));
    } catch (err) { toast.error(String(err)); } finally { setBusy(false); }
  };
  return <Flex direction="column" gap="3" style={{ minHeight: 0, height: "100%" }}>
    {error && <Callout.Root color="red"><Callout.Text>{error}</Callout.Text></Callout.Root>}
    <Text size="2" color="gray">{t("localThemes.publicSettings")}</Text>
    {managed && ready ? <ConfigFormTabs items={items} values={values}
      resolveText={(value) => resolveI18nText(value, i18n.resolvedLanguage || i18n.language)}
      onValueChange={(key, value) => setValues((current) => ({ ...current, [key]: value }))}
      className="min-h-0 flex-1" /> :
      <TextArea aria-label={t("localThemes.configuration")} value={raw} onChange={(e) => setRaw(e.target.value)} disabled={!ready} rows={14} />}
    <Flex justify="end" gap="3">
      <Dialog.Close><Button variant="soft">{t("common.close")}</Button></Dialog.Close>
      <Button disabled={!ready || busy} onClick={() => void save()}>{t("common.save")}</Button>
    </Flex>
  </Flex>;
}

export default function ThemeManager() {
  const { t, i18n } = useTranslation();
  const { refresh } = usePublicInfo();
  const [list, setList] = useState<ThemeList>({ active: "default", themes: [] });
  const [selected, setSelected] = useState("default");
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [trusted, setTrusted] = useState(false);
  const [uploadOpen, setUploadOpen] = useState(false);
  const [progress, setProgress] = useState(0);
  const task = useRef<ChunkUploadTask | null>(null);
  const alive = useRef(true);
  const load = useCallback(async (focus?: string) => {
    const data = await themeRequest<ThemeList>("list");
    if (!alive.current) return;
    setList(data);
    setSelected(focus || data.active);
    setReady(true);
    setError("");
  }, []);
  useEffect(() => {
    alive.current = true;
    void load().catch((err) => { if (alive.current) setError(String(err)); });
    return () => { alive.current = false; task.current?.cancel(); };
  }, [load]);

  const activate = async (short: string) => {
    setBusy(true);
    try {
      await themeRequest("set", "POST", { short });
      await load(short);
      await refresh();
      toast.success(t("localThemes.activated"));
    } catch (err) { toast.error(String(err)); } finally { setBusy(false); }
  };
  const remove = async () => {
    if (!window.confirm(t("localThemes.deleteConfirm", { theme: selected }))) return;
    setBusy(true);
    try {
      await themeRequest("delete", "POST", { short: selected });
      await load();
    } catch (err) { toast.error(String(err)); } finally { setBusy(false); }
  };
  const install = async (file: File) => {
    if (!file.name.toLowerCase().endsWith(".zip") || file.size <= 0 || file.size > 64 * 1024 * 1024) { toast.error(t("localThemes.zipLimit")); return; }
    const upload = createChunkUploadTask("/api/admin/upload");
    task.current = upload;
    setBusy(true);
    setProgress(0);
    try {
      const installed = await upload.upload<InstalledTheme>("theme", file, (value) => { if (alive.current) setProgress(value); });
      if (!alive.current) return;
      setUploadOpen(false);
      await load(installed?.short);
      toast.success(t("localThemes.installed"));
    } catch (err) {
      if (alive.current && !(err instanceof DOMException && err.name === "AbortError")) toast.error(String(err));
    } finally { task.current = null; if (alive.current) setBusy(false); }
  };
  const theme = list.themes.find((item) => item.short === selected);
  return <Card className="shrink-0">
    <Flex direction="column" gap="3">
      <Flex justify="between" align="center" gap="2" wrap="wrap">
        <Heading size="4">{t("localThemes.title")}</Heading>
        <Badge>{t("localThemes.active")}: {list.active}</Badge>
      </Flex>
      {error && <Callout.Root color="red"><Callout.Text>{error}</Callout.Text></Callout.Root>}
      <Text size="2" color="gray">{t("localThemes.description")}</Text>
      <Text as="label" size="2"><Flex gap="2" align="start">
        <Checkbox checked={trusted} onCheckedChange={(value) => setTrusted(value === true)} disabled={busy} />
        {t("localThemes.trust")}
      </Flex></Text>
      <Flex gap="2" wrap="wrap" align="center">
        <Select.Root value={selected} onValueChange={setSelected} disabled={!ready || busy}>
          <Select.Trigger aria-label={t("localThemes.title")} />
          <Select.Content>{list.themes.map((item) => <Select.Item key={item.short} value={item.short}>
            {item.short === "default" ? t("localThemes.builtin") : resolveI18nText(item.name, i18n.resolvedLanguage || i18n.language)} {item.version}
          </Select.Item>)}</Select.Content>
        </Select.Root>
        <Button disabled={!ready || busy || selected === list.active || (selected !== "default" && !trusted)} onClick={() => void activate(selected)}>{t("localThemes.activate")}</Button>
        <Button variant="soft" disabled={busy || (ready && list.active === "default")} onClick={() => void activate("default")}>{t("localThemes.restore")}</Button>
        <Button variant="soft" disabled={!ready || busy || !trusted} onClick={() => setUploadOpen(true)}>{t("localThemes.upload")}</Button>
        {selected !== "default" && theme && <>
          <Dialog.Root><Dialog.Trigger><Button variant="soft" disabled={busy}>{t("localThemes.configuration")}</Button></Dialog.Trigger>
            <Dialog.Content style={{ height: "min(80vh, 760px)", display: "flex", flexDirection: "column" }} maxWidth="900px">
              <Dialog.Title>{t("localThemes.configuration")} · {selected}</Dialog.Title>
              <Dialog.Description className="mb-3">{t("localThemes.isolatedSettings")}</Dialog.Description>
              <ThemeSettings key={selected} theme={theme} />
            </Dialog.Content>
          </Dialog.Root>
          <Button color="red" variant="soft" disabled={busy || selected === list.active} onClick={() => void remove()}>{t("common.delete")}</Button>
        </>}
      </Flex>
    </Flex>
    <UploadDialog open={uploadOpen} onOpenChange={(open) => { if (!busy) setUploadOpen(open); }}
      title={t("localThemes.upload")} description={t("localThemes.packageHint")} accept=".zip"
      clickToBrowseText={t("localThemes.chooseZip")} hintText={t("localThemes.zipLimit")}
      uploading={busy} progress={progress} uploadingText={t("localThemes.uploading")}
      closeLabel={t("common.close")} cancelUploadLabel={t("common.cancel")}
      onCancelUpload={() => task.current?.cancel()} onFileSelected={(file) => void install(file)} />
  </Card>;
}

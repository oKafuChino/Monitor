import { useEffect, useState, type ComponentProps } from "react";
import { Button, Callout, Flex, Heading } from "@radix-ui/themes";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import ConfigFormTabs from "@/components/admin/ConfigFormTabs";
import { usePublicInfo } from "@/contexts/PublicInfoContext";
import { readUISettings, saveUISettings } from "@/utils/uiSettings";
import { resolveI18nText } from "@/utils/i18nText";
import manifest from "../../../../komari-theme.json";

const fields = manifest.configuration.data as ComponentProps<typeof ConfigFormTabs>["items"];

export default function AppearanceSettings() {
  const { t, i18n } = useTranslation();
  const { refresh } = usePublicInfo();
  const [values, setValues] = useState<Record<string, unknown>>({});
  const [patch, setPatch] = useState<Record<string, unknown>>({});
  const [ready, setReady] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let disposed = false;
    readUISettings().then((data) => {
      if (!disposed) {
        setValues(data);
        setReady(true);
      }
    }).catch((err) => {
      if (!disposed) setError(String(err));
    });
    return () => { disposed = true; };
  }, []);

  const save = async () => {
    const submitted = patch;
    setSaving(true);
    try {
      await saveUISettings(submitted);
      // Keep edits made while the request was in flight available to save next.
      setPatch((current) => Object.fromEntries(
        Object.entries(current).filter(([key, value]) => value !== submitted[key]),
      ));
      await refresh();
      toast.success(t("settings.settings_saved"));
    } catch (err) {
      toast.error(String(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Flex direction="column" gap="4" className="h-full min-h-0 p-2 md:p-4">
      {error && <Callout.Root color="red"><Callout.Text>{error}</Callout.Text></Callout.Root>}
      <ConfigFormTabs
        items={fields}
        values={values}
        onValueChange={(key, value) => {
          setValues((previous) => ({ ...previous, [key]: value }));
          setPatch((previous) => ({ ...previous, [key]: value }));
        }}
        resolveText={(value) => resolveI18nText(value, i18n.resolvedLanguage || i18n.language)}
        className="min-h-0 flex-1"
        header={
          <Flex justify="between" align="center" gap="3">
            <Heading size="4">{t("settings.appearance.title")}</Heading>
            <Button onClick={() => void save()} disabled={!ready || saving || Object.keys(patch).length === 0}>
              {t("common.save")}
            </Button>
          </Flex>
        }
      />
    </Flex>
  );
}

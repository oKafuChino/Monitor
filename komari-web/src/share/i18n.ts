import i18next from "i18next";
import { initReactI18next } from "react-i18next";
import en from "../i18n/locales/en.json";
import zh from "../i18n/locales/zh_CN.json";
import tw from "../i18n/locales/zh_TW.json";
import ja from "../i18n/locales/ja_JP.json";
import id from "../i18n/locales/id_ID.json";

// Only share copy is bundled; this entry never initializes main-site storage.
void i18next.use(initReactI18next).init({
  lng: navigator.language, fallbackLng: "en", interpolation: { escapeValue: false },
  resources: {
    en: { translation: { share: en.share } },
    zh: { translation: { share: zh.share } },
    "zh-CN": { translation: { share: zh.share } },
    "zh-TW": { translation: { share: tw.share } },
    "zh-HK": { translation: { share: tw.share } },
    ja: { translation: { share: ja.share } }, id: { translation: { share: id.share } },
  },
});
export default i18next;

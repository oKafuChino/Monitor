import type { I18nText } from './i18nText';

// Shared managed configuration shape used by the embedded UI settings.
export interface ThemeConfiguration {
  type?: string;
  icon?: string;
  name?: I18nText;
  data?: unknown;
}

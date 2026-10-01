import type { I18nText } from './i18nText';

// Shared managed configuration shape used by built-in notification forms.
export interface ThemeConfiguration {
  type?: string;
  icon?: string;
  name?: I18nText;
  data?: unknown;
}

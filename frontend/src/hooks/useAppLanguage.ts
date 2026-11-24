import { useTranslation } from 'react-i18next';
import { type AppLanguage, resolveLanguage } from '@/i18n';

/** The UI language narrowed to one of the app's locales (zh / en / ru). */
export function useAppLanguage(): AppLanguage {
    const { i18n } = useTranslation();
    return resolveLanguage(i18n.language);
}

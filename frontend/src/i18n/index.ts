import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import en from './locales/en';
import zh from './locales/zh';
import ru from './locales/ru';

// Single source of truth for the languages the UI ships. Adding a locale means
// adding one row here (plus the locale file) — the language switchers, the MUI
// locale bundle and the browser detector all read from this list rather than
// hard-coding language codes of their own.
export const SUPPORTED_LANGUAGES = [
    // `shortLabel` is the 2-3 char badge under the ActivityBar globe icon, where
    // the full native name does not fit.
    { code: 'en', labelKey: 'system.language.en', shortLabel: 'EN' },
    { code: 'zh', labelKey: 'system.language.zh', shortLabel: '中文' },
    { code: 'ru', labelKey: 'system.language.ru', shortLabel: 'RU' },
] as const;

export type AppLanguage = (typeof SUPPORTED_LANGUAGES)[number]['code'];

export const DEFAULT_LANGUAGE: AppLanguage = 'en';

const SUPPORTED_CODES: readonly AppLanguage[] = SUPPORTED_LANGUAGES.map((l) => l.code);

const isSupported = (value: string | null | undefined): value is AppLanguage =>
    !!value && (SUPPORTED_CODES as readonly string[]).includes(value);

// Narrow an i18next language tag (which may carry a region, e.g. "ru-RU") down
// to one of the languages we actually ship. Callers use this instead of testing
// `i18n.language === 'zh'` so an unknown tag degrades to English rather than to
// whichever branch happened to be the else-case.
export const resolveLanguage = (language?: string | null): AppLanguage => {
    if (!language) return DEFAULT_LANGUAGE;
    const normalized = language.toLowerCase();
    const match = SUPPORTED_CODES.find((code) => normalized === code || normalized.startsWith(`${code}-`));
    return match ?? DEFAULT_LANGUAGE;
};

const resources = {
    en: {
        translation: en,
    },
    zh: {
        translation: zh,
    },
    ru: {
        translation: ru,
    },
};

// Detect a supported language from the browser's configured languages,
// falling back to English when nothing matches.
const detectBrowserLanguage = (): AppLanguage => {
    const browserLangs = navigator.languages && navigator.languages.length > 0 ? navigator.languages : [navigator.language];
    for (const lang of browserLangs) {
        if (!lang) continue;
        const normalized = lang.toLowerCase();
        const match = SUPPORTED_CODES.find((code) => normalized.startsWith(code));
        if (match) return match;
    }
    return DEFAULT_LANGUAGE;
};

// An explicit user choice (persisted to localStorage by i18n.changeLanguage)
// wins; otherwise the browser's languages pick one we ship. The detector
// plugin only runs detectors it knows by name, so this one is registered
// below rather than passed as an option.
const supportedLanguageDetector = {
    name: 'supportedLanguage',
    lookup: (): AppLanguage => {
        let stored: string | null = null;
        try {
            stored = localStorage.getItem('i18nextLng');
        } catch {
            // Storage can be blocked (private windows); fall through to the browser.
        }
        return isSupported(stored) ? stored : detectBrowserLanguage();
    },
};

const languageDetector = new LanguageDetector();
languageDetector.addDetector(supportedLanguageDetector);

const languageDetectorOptions = {
    order: [supportedLanguageDetector.name],
    lookupLocalStorage: 'i18nextLng',
    caches: ['localStorage'],
};

i18n
    .use(languageDetector) // Detect user language
    .use(initReactI18next) // Passes i18n down to react-i18next
    .init({
        resources,
        fallbackLng: DEFAULT_LANGUAGE, // Use English by default
        defaultNS: 'translation',
        debug: false,

        // Configure language detection and storage
        detection: languageDetectorOptions,

        interpolation: {
            escapeValue: false, // React already escapes values
        },
    });

export default i18n;

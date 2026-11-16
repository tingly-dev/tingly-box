import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import type { BackendModule } from 'i18next';
import en from './locales/en';

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

// English is the fallback for every other language, so it ships in the main
// bundle. The other locales are ~0.5 MB of source between them and a user only
// ever reads one, so each loads on demand: before first render for the
// detected language (see `i18nReady`), and through changeLanguage() when the
// user switches. i18next waits for the load before it flips `language`, so
// switching never flashes raw keys.
const resources = {
    en: {
        translation: en,
    },
};

const localeLoaders: Record<Exclude<AppLanguage, 'en'>, () => Promise<{ default: object }>> = {
    zh: () => import('./locales/zh'),
    ru: () => import('./locales/ru'),
};

const lazyLocales: BackendModule = {
    type: 'backend',
    init: () => {},
    read: (language, namespace, callback) => {
        const load = localeLoaders[language as keyof typeof localeLoaders];
        // Only the main `translation` namespace is a locale file; others (the
        // Projects package adds its own with addResourceBundle) have nothing
        // to fetch.
        if (!load || namespace !== 'translation') {
            callback(null, {});
            return;
        }
        load().then((m) => callback(null, m.default), (err) => callback(err, null));
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

// Resolves once the detected language's strings are loaded; main.tsx waits on
// it so the first paint is already translated.
export const i18nReady = i18n
    .use(languageDetector) // Detect user language
    .use(lazyLocales) // Load non-English locales on demand
    .use(initReactI18next) // Passes i18n down to react-i18next
    .init({
        resources,
        partialBundledLanguages: true, // en is bundled, the rest come from the backend above
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

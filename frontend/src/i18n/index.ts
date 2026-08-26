import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import en from './locales/en';

// Single source of truth for the languages the UI ships. Adding a locale means
// adding one row here (plus the locale file) — the language switchers, the MUI
// locale bundle and the browser detector all read from this list rather than
// hard-coding language codes of their own.
export const SUPPORTED_LANGUAGES = [
    // `shortLabel` is the 2-3 char badge under the ActivityBar globe icon, where
    // the full native name does not fit. `dir` drives both the document's `dir`
    // attribute and MUI's `theme.direction` (see ThemeProvider in App.tsx), so a
    // right-to-left locale mirrors the layout rather than only swapping strings.
    { code: 'en', labelKey: 'system.language.en', shortLabel: 'EN', dir: 'ltr' },
    { code: 'zh', labelKey: 'system.language.zh', shortLabel: '中文', dir: 'ltr' },
    { code: 'ru', labelKey: 'system.language.ru', shortLabel: 'RU', dir: 'ltr' },
    { code: 'fa', labelKey: 'system.language.fa', shortLabel: 'FA', dir: 'rtl' },
    { code: 'ar', labelKey: 'system.language.ar', shortLabel: 'AR', dir: 'rtl' },
] as const;

export type AppLanguage = (typeof SUPPORTED_LANGUAGES)[number]['code'];
export type TextDirection = (typeof SUPPORTED_LANGUAGES)[number]['dir'];

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

// Text direction for a language tag, defaulting to LTR for anything unknown.
export const directionOf = (language?: string | null): TextDirection =>
    SUPPORTED_LANGUAGES.find((l) => l.code === resolveLanguage(language))?.dir ?? 'ltr';

// Only English is bundled eagerly: it is the fallback, so it has to be present
// before the first render. Every other locale is a dynamic import, which keeps
// the always-preloaded i18n chunk to one language instead of all five (the
// bundle is in dist/index.html's modulepreload list — see frontend/CLAUDE.md).
const LOCALE_LOADERS: Record<Exclude<AppLanguage, 'en'>, () => Promise<{ default: object }>> = {
    zh: () => import('./locales/zh'),
    ru: () => import('./locales/ru'),
    fa: () => import('./locales/fa'),
    ar: () => import('./locales/ar'),
};

const loaded = new Set<AppLanguage>(['en']);

// Fetch a locale bundle and register it. Safe to call repeatedly: the first
// call per language does the import, later ones resolve immediately.
export const loadLanguage = async (language: AppLanguage): Promise<void> => {
    if (loaded.has(language)) return;
    const loader = LOCALE_LOADERS[language as Exclude<AppLanguage, 'en'>];
    if (!loader) return;
    try {
        const bundle = await loader();
        i18n.addResourceBundle(language, 'translation', bundle.default, true, true);
        loaded.add(language);
    } catch (error) {
        // A failed chunk fetch leaves the UI on the English fallback rather than
        // breaking the page, so log and carry on.
        console.error(`Failed to load the "${language}" locale bundle:`, error);
    }
};

// Switch the UI language: load the bundle first so the change never flashes
// through English, then persist the choice. Every language switcher calls this
// rather than i18n.changeLanguage directly.
export const setAppLanguage = async (language: AppLanguage): Promise<void> => {
    if (resolveLanguage(i18n.language) === language && loaded.has(language)) return;
    await loadLanguage(language);
    await i18n.changeLanguage(language);
    localStorage.setItem('i18nextLng', language);
};

const resources = {
    en: {
        translation: en,
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

// Custom language detector: an explicit user choice (persisted to localStorage
// by i18n.changeLanguage) always wins; otherwise fall back to the browser's language.
const languageDetectorOptions = {
    // Order and sources where to look for language
    order: ['localStorage'],
    // Keys or params to lookup language from
    lookupLocalStorage: 'i18nextLng',
    // Cache user language
    caches: ['localStorage'],
    // Custom detection function - check localStorage first, else detect from the browser
    detection: () => {
        const stored = localStorage.getItem('i18nextLng');
        if (isSupported(stored)) {
            return stored;
        }
        return detectBrowserLanguage();
    },
};

i18n
    .use(LanguageDetector) // Detect user language
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

// Resolves once the detected language's bundle is registered. main.tsx awaits
// this before the first render so a non-English user never sees English text
// swapped out under them.
export const i18nReady: Promise<void> = loadLanguage(resolveLanguage(i18n.language));

export default i18n;

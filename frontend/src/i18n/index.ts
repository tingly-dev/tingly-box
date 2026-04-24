import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import en from './locales/en';
import zh from './locales/zh';

const resources = {
    en: {
        translation: en,
    },
    zh: {
        translation: zh,
    },
};

// Custom language detector that defaults to English when localStorage is empty
const languageDetectorOptions = {
    // Order and sources where to look for language
    order: ['localStorage'],
    // Keys or params to lookup language from
    lookupLocalStorage: 'i18nextLng',
    // Cache user language
    caches: ['localStorage'],
    // Custom detection function - check localStorage first, default to 'en' if empty
    detection: () => {
        const stored = localStorage.getItem('i18nextLng');
        if (stored && (stored === 'en' || stored === 'zh')) {
            return stored;
        }
        return 'en'; // Default to English
    },
};

i18n
    .use(LanguageDetector) // Detect user language
    .use(initReactI18next) // Passes i18n down to react-i18next
    .init({
        resources,
        fallbackLng: 'en', // Use English by default
        defaultNS: 'translation',
        debug: false,

        // Configure language detection and storage
        detection: languageDetectorOptions,

        interpolation: {
            escapeValue: false, // React already escapes values
        },
    });

export default i18n;

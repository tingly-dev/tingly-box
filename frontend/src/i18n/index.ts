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

i18n
    .use(LanguageDetector) // Detect user language
    .use(initReactI18next) // Passes i18n down to react-i18next
    .init({
        resources,
        fallbackLng: 'en', // Use English by default
        defaultNS: 'translation',
        debug: false,

        // Configure language detection and storage
        detection: {
            // Order and sources where to look for language
            order: ['localStorage', 'navigator'],
            // Keys or params to lookup language from
            lookupLocalStorage: 'i18nextLng',
            // Cache user language
            caches: ['localStorage'],
        },

        interpolation: {
            escapeValue: false, // React already escapes values
        },
    });

export default i18n;

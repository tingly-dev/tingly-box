import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import en from './locales/en';

const resources = {
    en,
};

i18n
    .use(LanguageDetector) // Detect user language
    .use(initReactI18next) // Passes i18n down to react-i18next
    .init({
        resources,
        fallbackLng: 'en', // Use English by default
        debug: false,

        interpolation: {
            escapeValue: false, // React already escapes values
        },
    });

export default i18n;

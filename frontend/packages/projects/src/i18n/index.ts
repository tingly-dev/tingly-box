import i18n from 'i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import { initReactI18next } from 'react-i18next';
import { en } from './en';
import { zh } from './zh';

export const LANGUAGES = [
    { code: 'en', label: 'English' },
    { code: 'zh', label: '简体中文' },
] as const;

void i18n
    .use(LanguageDetector)
    .use(initReactI18next)
    .init({
        resources: { en: { translation: en }, zh: { translation: zh } },
        fallbackLng: 'en',
        supportedLngs: ['en', 'zh'],
        nonExplicitSupportedLngs: true,
        interpolation: { escapeValue: false },
        detection: { order: ['localStorage', 'navigator'], lookupLocalStorage: 'tingly-project.lang', caches: ['localStorage'] },
    });

export default i18n;

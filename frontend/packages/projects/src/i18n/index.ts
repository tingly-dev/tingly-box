import i18n from 'i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import { initReactI18next } from 'react-i18next';
import { en } from './en';
import { zh } from './zh';
import transcriptEn from '../transcript/locales/en';
import transcriptZh from '../transcript/locales/zh';

export const LANGUAGES = [
    { code: 'en', label: 'English' },
    { code: 'zh', label: '简体中文' },
] as const;

void i18n
    .use(LanguageDetector)
    .use(initReactI18next)
    .init({
        // The app's own strings live in the `projects` namespace, so a host
        // (Tingly Box) can load them next to its own without key clashes. The
        // transcript shared with Desk reads the host's default namespace
        // (projects.transcript.*), so the standalone app puts it there too.
        resources: {
            en: { projects: en, translation: transcriptEn },
            zh: { projects: zh, translation: transcriptZh },
        },
        ns: ['projects', 'translation'],
        defaultNS: 'projects',
        // The transcript reads the default namespace (it names none).
        fallbackNS: 'translation',
        fallbackLng: 'en',
        supportedLngs: ['en', 'zh'],
        nonExplicitSupportedLngs: true,
        interpolation: { escapeValue: false },
        detection: { order: ['localStorage', 'navigator'], lookupLocalStorage: 'tingly-project.lang', caches: ['localStorage'] },
    });

export default i18n;

import { describe, expect, it } from 'vitest';
import en from './en';
import ru from './ru';
import zh from './zh';
import { en as projectsEn } from '@tingly/projects/locales/en';
import { zh as projectsZh } from '@tingly/projects/locales/zh';

type Bundle = Record<string, unknown>;

const BUNDLES: Array<[string, Bundle]> = [
    ['en', en as unknown as Bundle],
    ['zh', zh as unknown as Bundle],
    ['ru', ru as unknown as Bundle],
];

const PLURAL_SUFFIX = /_(zero|one|two|few|many|other)$/;

const stems = (bundle: Bundle, prefix = '', out = new Set<string>()): Set<string> => {
    for (const [key, value] of Object.entries(bundle)) {
        const path = prefix ? `${prefix}.${key}` : key;
        if (value && typeof value === 'object' && !Array.isArray(value)) {
            stems(value as Bundle, path, out);
        } else {
            out.add(path.replace(PLURAL_SUFFIX, ''));
        }
    }
    return out;
};

// Read the sources through Vite rather than node:fs so the suite needs no
// @types/node (the project deliberately does not depend on it).
// The workspace packages the app renders (packages/*) take their strings
// from these bundles too. Of @tingly/projects only the transcript does: the
// rest is Projects' own app, with its own bundle (packages/projects/src/i18n)
// in the `projects` namespace, checked separately below.
const SOURCES = import.meta.glob([
    '/src/**/*.{ts,tsx}',
    '/packages/!(projects)/src/**/*.{ts,tsx}',
    '/packages/projects/src/transcript/**/*.{ts,tsx}',
], {
    query: '?raw',
    import: 'default',
    eager: true,
}) as Record<string, string>;

// t('some.key', { defaultValue: '…' }) — the inline-English pattern. It is a
// convenient fallback, but a key that exists ONLY as a defaultValue silently
// renders English in every other language, which is how ~130 strings drifted
// out of the locale files. Every literal key the code references has to exist
// in all five bundles.
const T_CALL = /\bt\(\s*['"]([\w.-]+)['"]/g;

describe('t() key coverage', () => {
    const referenced = new Set<string>();
    for (const [path, source] of Object.entries(SOURCES)) {
        if (/\.(test|spec)\.tsx?$/.test(path) || path.includes('/mocks/')) continue;
        for (const match of source.matchAll(T_CALL)) referenced.add(match[1]);
    }

    it('finds t() calls to check', () => {
        expect(referenced.size).toBeGreaterThan(500);
    });

    it.each(BUNDLES)('%s defines every key the code passes to t()', (_name, bundle) => {
        const have = stems(bundle);
        // Keys assembled at runtime (`t(\`prompt.skill.ides.${id}\`)`) are not
        // literals, so only literal call sites are covered here.
        const missing = [...referenced].filter((key) => key.includes('.') && !have.has(key)).sort();
        expect(missing).toEqual([]);
    });
});

// Projects' pages (everything in @tingly/projects but the transcript) read the
// `projects` namespace, which Tingly Box loads from the package's en and zh
// bundles when a Projects route opens (pages/projects/ProjectsRoute.tsx);
// Russian falls back to English there.
const PROJECTS_SOURCES = import.meta.glob([
    '/packages/projects/src/**/*.{ts,tsx}',
    '!/packages/projects/src/transcript/**',
], {
    query: '?raw',
    import: 'default',
    eager: true,
}) as Record<string, string>;

const PROJECTS_BUNDLES: Array<[string, Bundle]> = [
    ['en', projectsEn as unknown as Bundle],
    ['zh', projectsZh as unknown as Bundle],
];

describe('t() key coverage, projects namespace', () => {
    const referenced = new Set<string>();
    const withoutNamespace: string[] = [];
    for (const [path, source] of Object.entries(PROJECTS_SOURCES)) {
        if (/\.(test|spec)\.tsx?$/.test(path)) continue;
        if (/useTranslation\(\)/.test(source)) withoutNamespace.push(path);
        for (const match of source.matchAll(T_CALL)) referenced.add(match[1]);
    }

    // In Tingly Box the default namespace is Tingly Box's own strings.
    it('reads the projects namespace explicitly', () => {
        expect(withoutNamespace).toEqual([]);
    });

    it('finds t() calls to check', () => {
        expect(referenced.size).toBeGreaterThan(100);
    });

    it.each(PROJECTS_BUNDLES)('%s defines every key the Projects pages pass to t()', (_name, bundle) => {
        const have = stems(bundle);
        const missing = [...referenced].filter((key) => key.includes('.') && !have.has(key)).sort();
        expect(missing).toEqual([]);
    });
});

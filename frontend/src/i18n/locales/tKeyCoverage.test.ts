import { describe, expect, it } from 'vitest';
import ar from './ar';
import en from './en';
import fa from './fa';
import ru from './ru';
import zh from './zh';

type Bundle = Record<string, unknown>;

const BUNDLES: Array<[string, Bundle]> = [
    ['en', en as unknown as Bundle],
    ['zh', zh as unknown as Bundle],
    ['ru', ru as unknown as Bundle],
    ['fa', fa as unknown as Bundle],
    ['ar', ar as unknown as Bundle],
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
const SOURCES = import.meta.glob('/src/**/*.{ts,tsx}', {
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

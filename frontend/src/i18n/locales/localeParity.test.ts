import { describe, expect, it } from 'vitest';
import { SUPPORTED_LANGUAGES, directionOf } from '@/i18n';
import ar from './ar';
import en from './en';
import fa from './fa';
import ru from './ru';
import zh from './zh';

type Bundle = Record<string, unknown>;

// i18next resolves a plural key by appending an Intl.PluralRules category
// suffix. English needs two (_one/_other), Russian four, Arabic six and
// Chinese none — so a key like `activeUsers_one` in en.ts has no single
// counterpart in the other bundles. Compare on the stem instead.
const PLURAL_SUFFIX = /_(zero|one|two|few|many|other)$/;

const flatten = (bundle: Bundle, prefix = '', out = new Set<string>()): Set<string> => {
    for (const [key, value] of Object.entries(bundle)) {
        const path = prefix ? `${prefix}.${key}` : key;
        if (value && typeof value === 'object' && !Array.isArray(value)) {
            flatten(value as Bundle, path, out);
        } else {
            out.add(path.replace(PLURAL_SUFFIX, ''));
        }
    }
    return out;
};

const missingFrom = (source: Set<string>, target: Set<string>): string[] =>
    [...source].filter((key) => !target.has(key)).sort();

// Locales expected to cover en.ts and zh.ts in full. zh is excluded: it
// deliberately omits scenarioOverview.descriptions so those product taglines
// fall back to English (see the note in zh.ts).
const TRANSLATIONS: Array<[string, Bundle]> = [
    ['ru', ru as unknown as Bundle],
    ['fa', fa as unknown as Bundle],
    ['ar', ar as unknown as Bundle],
];

describe('locale bundles', () => {
    const enKeys = flatten(en as Bundle);
    const zhKeys = flatten(zh as Bundle);

    // en.ts is the fallback bundle, so every key it defines must exist in the
    // other locales or those users silently drop back to English.
    it.each(TRANSLATIONS)('%s covers every key in en', (_name, bundle) => {
        expect(missingFrom(enKeys, flatten(bundle))).toEqual([]);
    });

    // Several namespaces (remoteControl.*, bots.*, notify.*) carry their English
    // copy inline as t(..., { defaultValue }) and are only overridden in the
    // non-English bundles; keep the other locales from falling behind zh there.
    it.each(TRANSLATIONS)('%s covers every key in zh', (_name, bundle) => {
        expect(missingFrom(zhKeys, flatten(bundle))).toEqual([]);
    });

    it('exposes a native label for every shipped language', () => {
        const all: Array<[string, Bundle]> = [
            ['en', en as unknown as Bundle],
            ['zh', zh as unknown as Bundle],
            ...TRANSLATIONS,
        ];
        for (const [, bundle] of all) {
            const language = (bundle.system as Bundle).language as Record<string, string>;
            expect(language.en).toBe('English');
            expect(language.zh).toBe('中文');
            expect(language.ru).toBe('Русский');
            expect(language.fa).toBe('فارسی');
            expect(language.ar).toBe('العربية');
        }
    });

    // The RTL locales must declare dir: 'rtl' or the layout silently stays LTR.
    it('marks the right-to-left languages as rtl', () => {
        const dirs = Object.fromEntries(SUPPORTED_LANGUAGES.map((l) => [l.code, l.dir]));
        expect(dirs).toEqual({ en: 'ltr', zh: 'ltr', ru: 'ltr', fa: 'rtl', ar: 'rtl' });
        expect(directionOf('ar-SA')).toBe('rtl');
        expect(directionOf('de-DE')).toBe('ltr');
    });
});

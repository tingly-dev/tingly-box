import { afterAll, describe, expect, it } from 'vitest';
import i18n, { i18nReady } from './index';
import en from './locales/en';
import zh from './locales/zh';
import ru from './locales/ru';

const probe = 'common.save';
const lookup = (bundle: object): string => probe.split('.').reduce<any>((o, k) => o?.[k], bundle);

describe('lazy locales', () => {
    afterAll(() => i18n.changeLanguage('en'));

    it('starts in English with only the bundled locale loaded', async () => {
        await i18nReady;
        expect(i18n.language).toBe('en');
        expect(i18n.t(probe)).toBe(lookup(en));
        expect(i18n.hasResourceBundle('zh', 'translation')).toBe(false);
        expect(i18n.hasResourceBundle('ru', 'translation')).toBe(false);
    });

    it.each([['zh', zh], ['ru', ru]] as const)('loads %s on changeLanguage and only then flips the language', async (code, bundle) => {
        expect(lookup(bundle)).not.toBe(lookup(en));
        await i18n.changeLanguage(code);
        expect(i18n.language).toBe(code);
        expect(i18n.t(probe)).toBe(lookup(bundle));
    });

    it('keeps the Projects namespace added by the host (not a locale file)', async () => {
        i18n.addResourceBundle('en', 'projects', { hello: 'hi' }, true, true);
        await i18n.changeLanguage('en');
        expect(i18n.t('hello', { ns: 'projects' })).toBe('hi');
    });
});

import { CacheProvider } from '@emotion/react';
import createCache from '@emotion/cache';
import { prefixer } from 'stylis';
import rtlPlugin from 'stylis-plugin-rtl';
import React, { useEffect } from 'react';
import { directionOf, type AppLanguage } from '@/i18n';

// Created once and reused, so switching language does not throw away every
// computed style. stylis-plugin-rtl mirrors physical CSS (margin-left ⇄
// margin-right, left ⇄ right, …) as it is serialized — that is what lets the
// `sx={{ ml: 2 }}` spread through the app work in Persian and Arabic without
// being rewritten by hand.
//
// LTR deliberately has no cache of its own: it keeps emotion's default, so the
// existing languages render through exactly the path they always did. The key
// must not collide with that default ('mui'), or the two caches fight over the
// same <style> tags.
const rtlCache = createCache({ key: 'mui-rtl', stylisPlugins: [prefixer, rtlPlugin] });

interface DirectionProviderProps {
    language: AppLanguage;
    children: React.ReactNode;
}

/**
 * Applies the active language's text direction to the whole app.
 *
 * `document.documentElement.dir` is what makes the browser mirror text
 * alignment, flex/grid flow and scrollbars; the emotion cache handles the
 * physical CSS our own components write. MUI's own components read direction
 * from `theme.direction` instead (see theme/index.ts), so all three have to
 * agree — hence a single provider that owns the switch. It also keeps
 * `documentElement.lang` in sync, which browsers use for font fallback and
 * hyphenation and screen readers use to pick a voice.
 */
export const DirectionProvider: React.FC<DirectionProviderProps> = ({ language, children }) => {
    const direction = directionOf(language);

    useEffect(() => {
        const root = document.documentElement;
        const previousDir = root.dir;
        const previousLang = root.lang;
        root.dir = direction;
        root.lang = language;
        return () => {
            root.dir = previousDir;
            root.lang = previousLang;
        };
    }, [direction, language]);

    if (direction === 'ltr') return <>{children}</>;
    return <CacheProvider value={rtlCache}>{children}</CacheProvider>;
};

export default DirectionProvider;

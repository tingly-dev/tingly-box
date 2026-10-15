import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { CssBaseline, ThemeProvider, useMediaQuery } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { createAppTheme, type ResolvedThemeMode, type ThemeMode } from '../theme';
import { isThemeMode, resolveThemeMode } from '../theme/modes';
import { readLocal, writeLocal } from '../utils/storage';

export type { ThemeMode };

interface ThemeModeValue {
    mode: ThemeMode;
    resolved: ResolvedThemeMode;
    setMode: (m: ThemeMode) => void;
}

const Ctx = createContext<ThemeModeValue | null>(null);

export function ThemeModeProvider({ children }: { children: ReactNode }) {
    const prefersDark = useMediaQuery('(prefers-color-scheme: dark)');
    const [mode, setModeState] = useState<ThemeMode>(() => {
        const stored = readLocal('themeMode');
        return isThemeMode(stored) ? stored : 'system';
    });
    const { i18n } = useTranslation();
    const resolved = resolveThemeMode(mode, prefersDark);
    const theme = useMemo(() => createAppTheme(resolved, i18n.language), [resolved, i18n.language]);

    useEffect(() => {
        document.documentElement.lang = i18n.language;
    }, [i18n.language]);

    const value = useMemo<ThemeModeValue>(() => ({
        mode,
        resolved,
        setMode: (m) => {
            setModeState(m);
            writeLocal('themeMode', m);
        },
    }), [mode, resolved]);

    return (
        <Ctx.Provider value={value}>
            <ThemeProvider theme={theme}>
                <CssBaseline />
                {children}
            </ThemeProvider>
        </Ctx.Provider>
    );
}

export function useThemeMode() {
    const v = useContext(Ctx);
    if (!v) throw new Error('useThemeMode outside ThemeModeProvider');
    return v;
}

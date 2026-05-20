import React, { createContext, useContext, useState, useEffect, useMemo } from 'react';
import type { ResolvedThemeMode, ThemeMode } from '@/theme';
import { isThemeMode, SYSTEM_THEME_MODE_VALUES, THEME_MODE_VALUES } from '@/theme/options';

interface ThemeContextType {
  mode: ThemeMode;
  effectiveMode: ResolvedThemeMode;
  toggleTheme: () => void;
  setTheme: (mode: ThemeMode) => void;
}

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

export const useThemeMode = (): ThemeContextType => {
  const context = useContext(ThemeContext);
  if (!context) {
    throw new Error('useThemeMode must be used within a ThemeModeProvider');
  }
  return context;
};

interface ThemeModeProviderProps {
  children: React.ReactNode;
}

const STORAGE_KEY = 'tingly-theme-mode';

const getSystemMode = (): ResolvedThemeMode => {
  if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) {
    return 'dark';
  }
  return 'light';
};

export const ThemeModeProvider: React.FC<ThemeModeProviderProps> = ({ children }) => {
  const [mode, setMode] = useState<ThemeMode>(() => {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (isThemeMode(stored)) {
      return stored;
    }
    return 'system';
  });
  const [systemMode, setSystemMode] = useState<ResolvedThemeMode>(getSystemMode);

  const toggleTheme = () => {
    setMode((prev) => {
      const currentIndex = THEME_MODE_VALUES.indexOf(prev);
      return THEME_MODE_VALUES[(currentIndex + 1) % THEME_MODE_VALUES.length];
    });
  };

  const setTheme = (newMode: ThemeMode) => {
    setMode(newMode);
  };

  useEffect(() => {
    if (!window.matchMedia) return;
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    const handleSystemModeChange = (event: MediaQueryListEvent) => {
      setSystemMode(event.matches ? SYSTEM_THEME_MODE_VALUES[1] : SYSTEM_THEME_MODE_VALUES[0]);
    };

    setSystemMode(mediaQuery.matches ? SYSTEM_THEME_MODE_VALUES[1] : SYSTEM_THEME_MODE_VALUES[0]);
    mediaQuery.addEventListener('change', handleSystemModeChange);
    return () => mediaQuery.removeEventListener('change', handleSystemModeChange);
  }, []);

  const effectiveMode = mode === 'system' ? systemMode : mode;

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, mode);
  }, [mode]);

  useEffect(() => {
    document.documentElement.classList.remove('light', 'dark', 'system', 'sunlit', 'claude');
    document.documentElement.classList.add(effectiveMode);
    if (mode === 'system') {
      document.documentElement.classList.add('system');
    }
  }, [effectiveMode, mode]);

  const value = useMemo(() => ({ mode, effectiveMode, toggleTheme, setTheme }), [mode, effectiveMode]);

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
};

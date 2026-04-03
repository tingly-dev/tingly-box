import React, { createContext, useContext, useState, useEffect, useMemo } from 'react';

type ThemeMode = 'light' | 'dark' | 'sunlit';

interface ThemeContextType {
  mode: ThemeMode;
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

export const ThemeModeProvider: React.FC<ThemeModeProviderProps> = ({ children }) => {
  const [mode, setMode] = useState<ThemeMode>(() => {
    // Check localStorage first
    const stored = localStorage.getItem(STORAGE_KEY) as ThemeMode | null;
    if (stored === 'light' || stored === 'dark' || stored === 'sunlit') {
      return stored;
    }
    // Check system preference
    if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) {
      return 'dark';
    }
    return 'light';
  });

  const toggleTheme = () => {
    setMode((prev) => {
      if (prev === 'light') return 'dark';
      if (prev === 'dark') return 'sunlit';
      return 'light';
    });
  };

  const setTheme = (newMode: ThemeMode) => {
    setMode(newMode);
  };

  useEffect(() => {
    // Persist to localStorage
    localStorage.setItem(STORAGE_KEY, mode);
    // Update document class for any CSS that depends on it
    document.documentElement.classList.remove('light', 'dark', 'sunlit');
    document.documentElement.classList.add(mode);
  }, [mode]);

  const value = useMemo(() => ({ mode, toggleTheme, setTheme }), [mode]);

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
};

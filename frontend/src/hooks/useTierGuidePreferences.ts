import { useEffect, useState } from 'react';

const TIER_GUIDE_PREFERENCES_KEY = 'tingly.tierGuide.preferences';

export interface TierGuidePreferences {
    seen: boolean;
    dismissedAt: number | null;
    lastVersion: string;
    context: {
        firstProviderShown: boolean;
        tierHoverShown: boolean;
        manualShown: boolean;
    };
}

const DEFAULT_PREFERENCES: TierGuidePreferences = {
    seen: false,
    dismissedAt: null,
    lastVersion: '1.0.0',
    context: {
        firstProviderShown: false,
        tierHoverShown: false,
        manualShown: false,
    },
};

/**
 * Load tier guide preferences from localStorage
 */
export const loadTierGuidePreferences = (): TierGuidePreferences => {
    if (typeof window === 'undefined') {
        return DEFAULT_PREFERENCES;
    }

    try {
        const stored = localStorage.getItem(TIER_GUIDE_PREFERENCES_KEY);
        if (stored) {
            return { ...DEFAULT_PREFERENCES, ...JSON.parse(stored) };
        }
    } catch (error) {
        console.warn('Failed to load tier guide preferences:', error);
    }

    return DEFAULT_PREFERENCES;
};

/**
 * Save tier guide preferences to localStorage
 */
export const saveTierGuidePreferences = (preferences: TierGuidePreferences): void => {
    if (typeof window === 'undefined') {
        return;
    }

    try {
        localStorage.setItem(TIER_GUIDE_PREFERENCES_KEY, JSON.stringify(preferences));
    } catch (error) {
        console.warn('Failed to save tier guide preferences:', error);
    }
};

/**
 * Hook to manage tier guide preferences
 *
 * Provides:
 * - Current preferences state
 * - Function to mark guide as seen
 * - Function to update context flags
 * - Function to reset preferences (for testing/settings)
 */
export const useTierGuidePreferences = () => {
    const [preferences, setPreferences] = useState<TierGuidePreferences>(DEFAULT_PREFERENCES);
    const [loaded, setLoaded] = useState(false);

    // Load preferences on mount
    useEffect(() => {
        setPreferences(loadTierGuidePreferences());
        setLoaded(true);
    }, []);

    const markAsSeen = (context: 'first-provider' | 'tier-hover' | 'manual') => {
        const updated = {
            ...preferences,
            seen: true,
            dismissedAt: Date.now(),
            context: {
                ...preferences.context,
                [context === 'first-provider' ? 'firstProviderShown' : context === 'tier-hover' ? 'tierHoverShown' : 'manualShown']: true,
            },
        };
        setPreferences(updated);
        saveTierGuidePreferences(updated);
    };

    const updateContext = (updates: Partial<TierGuidePreferences['context']>) => {
        const updated = {
            ...preferences,
            context: {
                ...preferences.context,
                ...updates,
            },
        };
        setPreferences(updated);
        saveTierGuidePreferences(updated);
    };

    const resetPreferences = () => {
        setPreferences(DEFAULT_PREFERENCES);
        saveTierGuidePreferences(DEFAULT_PREFERENCES);
    };

    const shouldShowFirstRunHint = loaded && !preferences.context.firstProviderShown;

    return {
        preferences,
        loaded,
        markAsSeen,
        updateContext,
        resetPreferences,
        shouldShowFirstRunHint,
    };
};

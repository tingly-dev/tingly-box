/**
 * Analytics Context
 *
 * Provides analytics state and controls throughout the app
 */

import React, { createContext, useContext, useState, useEffect, ReactNode } from 'react';
import { analytics } from '@/utils/analytics';

export interface AnalyticsContextType {
    // Whether analytics is enabled in config
    enabled: boolean;
    // Whether user has given consent
    hasConsent: boolean;
    // Grant consent
    grantConsent: () => void;
    // Revoke consent
    revokeConsent: () => void;
    // Get data preview
    getDataPreview: () => string;
    // Set application version
    setVersion: (version: string) => void;
}

const AnalyticsContext = createContext<AnalyticsContextType | undefined>(undefined);

interface AnalyticsProviderProps {
    children: ReactNode;
    measurementId?: string;
    enabled?: boolean;
    debug?: boolean;
    version?: string;
}

export const AnalyticsProvider: React.FC<AnalyticsProviderProps> = ({
    children,
    measurementId = '',
    enabled = true,
    debug = true,
    version = '',
}) => {
    const [hasConsent, setHasConsent] = useState<boolean>(() => {
        // Initialize from localStorage
        return localStorage.getItem('analytics_consent') === 'true';
    });

    useEffect(() => {
        // Initialize analytics on mount
        if (measurementId) {
            analytics.initialize({
                measurementId,
                enabled,
                debug,
            });
        }

        // Set version if provided
        if (version) {
            analytics.setVersion(version);
        }
    }, [measurementId, enabled, debug, version]);

    const grantConsent = () => {
        analytics.grantConsent();
        setHasConsent(true);
    };

    const revokeConsent = () => {
        analytics.revokeConsent();
        setHasConsent(false);
    };

    const getDataPreview = () => {
        return analytics.getDataPreview();
    };

    const setVersion = (version: string) => {
        analytics.setVersion(version);
    };

    const value: AnalyticsContextType = {
        enabled,
        hasConsent,
        grantConsent,
        revokeConsent,
        getDataPreview,
        setVersion,
    };

    return (
        <AnalyticsContext.Provider value={value}>
            {children}
        </AnalyticsContext.Provider>
    );
};

export const useAnalytics = (): AnalyticsContextType => {
    const context = useContext(AnalyticsContext);
    if (context === undefined) {
        throw new Error('useAnalytics must be used within an AnalyticsProvider');
    }
    return context;
};

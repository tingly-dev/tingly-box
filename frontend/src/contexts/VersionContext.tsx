import React, { createContext, useContext, useEffect, useState, useCallback, type ReactNode } from 'react';
import { api } from '../services/api';

interface VersionContextType {
    currentVersion: string;
    latestVersion: string | null;
    hasUpdate: boolean;
    shouldNotify: boolean;
    showNotification: boolean;
    releaseURL: string | null;
    checking: boolean;
    checkForUpdates: (manual?: boolean) => Promise<void>;
    updateTrigger: number;
}

const VersionContext = createContext<VersionContextType | undefined>(undefined);

export const useVersion = () => {
    const context = useContext(VersionContext);
    if (context === undefined) {
        throw new Error('useVersion must be used within a VersionProvider');
    }
    return context;
};

interface VersionProviderProps {
    children: ReactNode;
}

export const VersionProvider: React.FC<VersionProviderProps> = ({ children }) => {
    const [currentVersion, setCurrentVersion] = useState<string>('Unknown');
    const [latestVersion, setLatestVersion] = useState<string | null>(null);
    const [hasUpdate, setHasUpdate] = useState(false);
    const [shouldNotify, setShouldNotify] = useState(false);
    const [releaseURL, setReleaseURL] = useState<string | null>(null);
    const [checking, setChecking] = useState(false);
    const [manualTrigger, setManualTrigger] = useState(false);
    const [updateTrigger, setUpdateTrigger] = useState(0);

    const checkForUpdates = useCallback(async (manual = false) => {
        setChecking(true);
        if (manual) {
            setManualTrigger(true);
            setUpdateTrigger(prev => prev + 1);
        }
        try {
            const result = await api.getLatestVersion();
            if (result.success) {
                setCurrentVersion(result.data.current_version);
                setLatestVersion(result.data.latest_version);
                setHasUpdate(result.data.has_update);
                setShouldNotify(result.data.should_notify);
                setReleaseURL(result.data.release_url);
            }
        } catch (error) {
            console.error('Failed to check for updates:', error);
        } finally {
            setChecking(false);
        }
    }, []);

    useEffect(() => {
        // Check on mount (non-manual)
        checkForUpdates(false);

        // Check every 24 hours (non-manual)
        const interval = setInterval(() => checkForUpdates(false), 24 * 60 * 60 * 1000);
        return () => clearInterval(interval);
    }, [checkForUpdates]);

    // Determine if notification should show
    const showNotification = hasUpdate && (shouldNotify || manualTrigger);

    return (
        <VersionContext.Provider
            value={{
                currentVersion,
                latestVersion,
                hasUpdate,
                shouldNotify,
                releaseURL,
                checking,
                checkForUpdates,
                showNotification,
                updateTrigger,
            }}
        >
            {children}
        </VersionContext.Provider>
    );
};

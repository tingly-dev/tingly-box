import React, { createContext, useContext, useEffect, useState, useCallback, type ReactNode } from 'react';
import { api } from '../services/api';

interface VersionContextType {
    currentVersion: string;
    latestVersion: string | null;
    hasUpdate: boolean;
    shouldNotify: boolean;
    releaseURL: string | null;
    checking: boolean;
    checkForUpdates: (manual?: boolean) => Promise<void>;
    showUpdateDialog: () => void;
    openUpdateDialog: boolean;
    closeUpdateDialog: () => void;
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
    const [openUpdateDialog, setOpenUpdateDialog] = useState(false);

    const checkForUpdates = useCallback(async (manual = false) => {
        setChecking(true);
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
        // Check on mount
        checkForUpdates(false);

        // Check every 24 hours
        const interval = setInterval(() => checkForUpdates(false), 24 * 60 * 60 * 1000);
        return () => clearInterval(interval);
    }, [checkForUpdates]);

    const showUpdateDialog = useCallback(() => {
        setOpenUpdateDialog(true);
    }, []);

    const closeUpdateDialog = useCallback(() => {
        setOpenUpdateDialog(false);
    }, []);

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
                showUpdateDialog,
                openUpdateDialog,
                closeUpdateDialog,
            }}
        >
            {children}
        </VersionContext.Provider>
    );
};

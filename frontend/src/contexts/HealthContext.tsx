import React, { createContext, useContext, useEffect, useState, useCallback, useMemo, useRef, type ReactNode } from 'react';
import { api } from '../services/api';

interface HealthContextType {
    isHealthy: boolean;
    lastCheck: Date | null;
    checking: boolean;
    checkHealth: () => Promise<void>;
    disconnectDialogOpen: boolean;
    showDisconnectDialog: () => void;
    closeDisconnectDialog: () => void;
}

const HealthContext = createContext<HealthContextType | undefined>(undefined);

export const useHealth = () => {
    const context = useContext(HealthContext);
    if (context === undefined) {
        throw new Error('useHealth must be used within a HealthProvider');
    }
    return context;
};

interface HealthProviderProps {
    children: ReactNode;
}

export const HealthProvider: React.FC<HealthProviderProps> = ({ children }) => {
    const [isHealthy, setIsHealthy] = useState(true);
    const [lastCheck, setLastCheck] = useState<Date | null>(null);
    const [checking, setChecking] = useState(false);
    const [disconnectDialogOpen, setDisconnectDialogOpen] = useState(false);
    const disconnectDialogOpenRef = useRef(false);

    const checkHealth = useCallback(async () => {
        setChecking(true);
        try {
            const healthy = await api.healthCheck();
            setIsHealthy(healthy);
            setLastCheck(new Date());
            // Auto close disconnect dialog if health is restored
            if (healthy && disconnectDialogOpenRef.current) {
                setDisconnectDialogOpen(false);
            }
        } catch (error) {
            console.error('Health check failed:', error);
            setIsHealthy(false);
            setLastCheck(new Date());
        } finally {
            setChecking(false);
        }
    }, []); // Empty deps - checkHealth never needs to be recreated

    // Keep ref in sync with state
    useEffect(() => {
        disconnectDialogOpenRef.current = disconnectDialogOpen;
    }, [disconnectDialogOpen]);

    const showDisconnectDialog = useCallback(() => {
        setDisconnectDialogOpen(true);
    }, []);

    const closeDisconnectDialog = useCallback(() => {
        setDisconnectDialogOpen(false);
    }, []);

    useEffect(() => {
        // Check on mount
        checkHealth();

        // Check every 30 seconds — but only while the tab is on screen: a
        // background tab has nobody to show the disconnect dialog to. Coming
        // back re-checks at once, so a server that died meanwhile is noticed
        // on return rather than up to 30s later.
        const interval = setInterval(() => {
            if (document.visibilityState !== 'hidden') void checkHealth();
        }, 30 * 1000);
        const onVisible = () => {
            if (document.visibilityState === 'visible') void checkHealth();
        };
        document.addEventListener('visibilitychange', onVisible);
        return () => {
            clearInterval(interval);
            document.removeEventListener('visibilitychange', onVisible);
        };
    }, [checkHealth]);

    // Memoized so the 30s health poll (which toggles `checking`) does not
    // hand every useHealth consumer a fresh value object twice per cycle.
    const value = useMemo(() => ({
        isHealthy, lastCheck, checking, checkHealth, disconnectDialogOpen, showDisconnectDialog, closeDisconnectDialog,
    }), [isHealthy, lastCheck, checking, checkHealth, disconnectDialogOpen, showDisconnectDialog, closeDisconnectDialog]);

    return (
        <HealthContext.Provider value={value}>
            {children}
        </HealthContext.Provider>
    );
};

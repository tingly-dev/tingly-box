import React, { createContext, useContext, useEffect, useState, useCallback, type ReactNode } from 'react';
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

    const checkHealth = useCallback(async () => {
        setChecking(true);
        try {
            const healthy = await api.healthCheck();
            setIsHealthy(healthy);
            setLastCheck(new Date());
            // Auto close disconnect dialog if health is restored
            if (healthy && disconnectDialogOpen) {
                setDisconnectDialogOpen(false);
            }
        } catch (error) {
            console.error('Health check failed:', error);
            setIsHealthy(false);
            setLastCheck(new Date());
        } finally {
            setChecking(false);
        }
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

        // Check every 30 seconds
        const interval = setInterval(checkHealth, 30 * 1000);
        return () => clearInterval(interval);
    }, [checkHealth]);

    return (
        <HealthContext.Provider value={{ isHealthy, lastCheck, checking, checkHealth, disconnectDialogOpen, showDisconnectDialog, closeDisconnectDialog }}>
            {children}
        </HealthContext.Provider>
    );
};

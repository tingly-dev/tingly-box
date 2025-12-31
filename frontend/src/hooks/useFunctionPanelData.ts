import { useState, useEffect, useCallback } from 'react';
import { api } from '../services/api';
import type { Provider } from '../types/provider';

export interface NotificationState {
    open: boolean;
    message?: string;
    severity?: 'success' | 'info' | 'warning' | 'error';
    autoHideDuration?: number;
    onClose?: () => void;
}

export const useFunctionPanelData = () => {
    const [showTokenModal, setShowTokenModal] = useState(false);
    const [generatedToken, setGeneratedToken] = useState<string>('');
    const [apiKey, setApiKey] = useState<string>('');
    const [notification, setNotification] = useState<NotificationState>({ open: false });
    const [providers, setProviders] = useState<Provider[]>([]);
    const [loading, setLoading] = useState(true);

    const showNotification = useCallback((
        message: string,
        severity: 'success' | 'info' | 'warning' | 'error' = 'info',
        autoHideDuration: number = 6000
    ) => {
        setNotification({
            open: true,
            message,
            severity,
            autoHideDuration,
            onClose: () => setNotification(prev => ({ ...prev, open: false }))
        });
    }, []);

    const copyToClipboard = useCallback(async (text: string, label: string) => {
        try {
            await navigator.clipboard.writeText(text);
            showNotification(`${label} copied to clipboard!`, 'success');
        } catch (err) {
            showNotification('Failed to copy to clipboard', 'error');
        }
    }, [showNotification]);

    const loadToken = useCallback(async () => {
        const result = await api.getToken();
        if (result.token) {
            setApiKey(result.token);
        }
    }, []);

    const loadProviders = useCallback(async () => {
        const result = await api.getProviders();
        if (result.success) {
            setProviders(result.data);
        }
        setLoading(false);
    }, []);

    const loadData = useCallback(async () => {
        await Promise.all([loadToken(), loadProviders()]);
    }, [loadToken, loadProviders]);

    const generateToken = useCallback(async (clientId: string = 'web') => {
        const result = await api.generateToken(clientId);
        if (result.success) {
            setGeneratedToken(result.data.token);
            copyToClipboard(result.data.token, 'Token');
            return result.data.token;
        } else {
            showNotification(`Failed to generate token: ${result.error}`, 'error');
            return null;
        }
    }, [copyToClipboard, showNotification]);

    useEffect(() => {
        loadData();
    }, [loadData]);

    const token = generatedToken || apiKey;
    const hasProviders = providers.length > 0;

    return {
        showTokenModal,
        setShowTokenModal,
        token,
        generateToken,
        showNotification,
        copyToClipboard,
        providers,
        loading,
        hasProviders,
        notification,
        loadProviders,
    };
};

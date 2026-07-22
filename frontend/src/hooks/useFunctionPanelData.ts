import { useState, useEffect, useCallback } from 'react';
import { api } from '../services/api';
import type { Provider } from '../types/provider';
import { notify } from '@/utils/notify';
import { providersDataCache } from '@/services/scenarioDataCache';

export interface NotificationState {
    open: boolean;
    message?: string;
    severity?: 'success' | 'info' | 'warning' | 'error';
    autoHideDuration?: number;
    onClose?: () => void;
}

// Notifications now render through the global NotificationProvider; this stub is
// kept so consumers that still pass `notification` to PageLayout keep compiling.
const CLOSED_NOTIFICATION: NotificationState = { open: false };

export const useFunctionPanelData = () => {
    const [showTokenModal, setShowTokenModal] = useState(false);
    const [generatedToken, setGeneratedToken] = useState<string>('');
    const [apiKey, setApiKey] = useState<string>('');
    // Seed synchronously from the shared cache when another mount (this
    // page renders up to 3 instances of this hook — modal provider, page,
    // TemplatePage) already has fresh providers, so this instance paints
    // immediately instead of re-showing a loading spinner.
    const [providers, setProviders] = useState<Provider[]>(() => providersDataCache.getCached() ?? []);
    const [loading, setLoading] = useState(() => providersDataCache.getCached() === undefined);

    const showNotification = useCallback((
        message: string,
        severity: 'success' | 'info' | 'warning' | 'error' = 'info',
        autoHideDuration: number = 6000
    ) => {
        notify.show(severity, message, { duration: autoHideDuration });
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

    // Always fetches fresh (correct after mutations like add/import
    // provider), but concurrent mounts of this hook on the same page
    // (ScenarioPageModalProvider + the page + TemplatePage each call it)
    // share one in-flight request instead of firing one apiece. Combined
    // with the cache-seeded initial state above, navigating to a page
    // that already has warm data paints instantly and revalidates quietly.
    const loadProviders = useCallback(async () => {
        const data = await providersDataCache.refresh();
        setProviders(data);
        setLoading(false);
    }, []);

    // Stay in sync with fetches triggered by other concurrent mounts of
    // this hook so a revalidation started elsewhere still updates this
    // instance's state.
    useEffect(() => {
        return providersDataCache.subscribe(setProviders);
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
        notification: CLOSED_NOTIFICATION,
        loadProviders,
    };
};

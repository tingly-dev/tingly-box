import React, { createContext, useContext, useState, useCallback, type ReactNode } from 'react';
import type { Provider } from '@/types/provider';
import { notify } from '@/utils/notify';

export interface SnackbarState {
    open: boolean;
    message: string;
    severity: 'success' | 'error';
}

// Snackbars now render through the global NotificationProvider; this stub keeps
// the context shape stable for consumers that still read `snackbar`.
const CLOSED_SNACKBAR: SnackbarState = { open: false, message: '', severity: 'error' };

export interface CustomModelDialogState {
    open: boolean;
    provider: Provider | null;
    value: string;
    originalValue?: string;
}

export interface ModelSelectContextValue {
    // Tab state
    internalCurrentTab: string | undefined;
    setInternalCurrentTab: (tab: string | undefined) => void;
    isInitialized: boolean;
    setIsInitialized: (initialized: boolean) => void;

    // Probing state
    probingModels: Set<string>;
    addProbingModel: (key: string) => void;
    removeProbingModel: (key: string) => void;
    isModelProbing: (key: string) => boolean;

    // Snackbar state
    snackbar: SnackbarState;
    showSnackbar: (message: string, severity: 'success' | 'error') => void;
    hideSnackbar: () => void;

    // Custom model dialog state
    customModelDialog: CustomModelDialogState;
    openCustomModelDialog: (provider: Provider, value?: string) => void;
    closeCustomModelDialog: () => void;
    updateCustomModelDialogValue: (value: string) => void;

    // Refresh trigger to force UI update after custom model changes
    refreshTrigger: number;
    triggerRefresh: () => void;
}

const ModelSelectContext = createContext<ModelSelectContextValue | undefined>(undefined);

export interface ModelSelectProviderProps {
    children: ReactNode;
}

// ModelSelectProvider is remounted by its caller's own React `key` whenever a
// dialog session genuinely restarts (ModelSelectDialog.tsx keys it on
// `selectedProvider`; useModelSelectDialog.tsx keys the whole dialog on
// "closed" vs "providerUuid-modelName") — a fresh mount already gives every
// piece of state below its initial value, so there is nothing left for this
// component to reset on its own. An earlier version tried to *also* detect
// "new session" internally by reading a `key` prop, but `key` is a reserved
// React prop that is never actually passed down (React warns and the read
// always returns undefined) — and even if it were readable, the remount
// above already guarantees the "reset" case, making the comparison it drove
// permanently unreachable. Removed rather than rewired.
export function ModelSelectProvider({ children }: ModelSelectProviderProps) {
    const [internalCurrentTab, setInternalCurrentTab] = useState<string | undefined>(undefined);
    const [isInitialized, setIsInitialized] = useState(false);
    const [probingModels, setProbingModels] = useState<Set<string>>(new Set());
    const [customModelDialog, setCustomModelDialog] = useState<CustomModelDialogState>({
        open: false,
        provider: null,
        value: ''
    });
    const [refreshTrigger, setRefreshTrigger] = useState(0);

    const triggerRefresh = useCallback(() => {
        setRefreshTrigger(prev => prev + 1);
    }, []);

    const addProbingModel = useCallback((key: string) => {
        setProbingModels(prev => new Set(prev).add(key));
    }, []);

    const removeProbingModel = useCallback((key: string) => {
        setProbingModels(prev => {
            const next = new Set(prev);
            next.delete(key);
            return next;
        });
    }, []);

    const isModelProbing = useCallback((key: string) => {
        return probingModels.has(key);
    }, [probingModels]);

    const showSnackbar = useCallback((message: string, severity: 'success' | 'error') => {
        notify.show(severity, message);
    }, []);

    const hideSnackbar = useCallback(() => {
        // No-op: notifications are dismissed by the global NotificationProvider.
    }, []);

    const openCustomModelDialog = useCallback((provider: Provider, value?: string) => {
        setCustomModelDialog({
            open: true,
            provider,
            value: value || '',
            originalValue: value
        });
    }, []);

    const closeCustomModelDialog = useCallback(() => {
        setCustomModelDialog({ open: false, provider: null, value: '', originalValue: undefined });
    }, []);

    const updateCustomModelDialogValue = useCallback((value: string) => {
        setCustomModelDialog(prev => ({ ...prev, value }));
    }, []);

    const value: ModelSelectContextValue = {
        internalCurrentTab,
        setInternalCurrentTab,
        isInitialized,
        setIsInitialized,
        probingModels,
        addProbingModel,
        removeProbingModel,
        isModelProbing,
        snackbar: CLOSED_SNACKBAR,
        showSnackbar,
        hideSnackbar,
        customModelDialog,
        openCustomModelDialog,
        closeCustomModelDialog,
        updateCustomModelDialogValue,
        refreshTrigger,
        triggerRefresh,
    };

    return (
        <ModelSelectContext.Provider value={value}>
            {children}
        </ModelSelectContext.Provider>
    );
}

export function useModelSelectContext(): ModelSelectContextValue {
    const context = useContext(ModelSelectContext);
    if (!context) {
        throw new Error('useModelSelectContext must be used within a ModelSelectProvider');
    }
    return context;
}

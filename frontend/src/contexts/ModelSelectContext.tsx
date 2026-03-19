import React, { createContext, useContext, useState, useCallback, useEffect, useRef, type ReactNode } from 'react';
import type { Provider } from '@/types/provider';

export interface SnackbarState {
    open: boolean;
    message: string;
    severity: 'success' | 'error';
}

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
    key?: string; // Key to force reset state when changed
}

export function ModelSelectProvider({ children, key: providerKey }: ModelSelectProviderProps) {
    const [internalCurrentTab, setInternalCurrentTab] = useState<string | undefined>(undefined);
    const [isInitialized, setIsInitialized] = useState(false);
    const [probingModels, setProbingModels] = useState<Set<string>>(new Set());
    const [snackbar, setSnackbar] = useState<SnackbarState>({
        open: false,
        message: '',
        severity: 'error'
    });
    const [customModelDialog, setCustomModelDialog] = useState<CustomModelDialogState>({
        open: false,
        provider: null,
        value: ''
    });
    const [refreshTrigger, setRefreshTrigger] = useState(0);

    // Track the previous key to detect if this is a new dialog session vs provider change
    const previousKeyRef = useRef<string | undefined>(undefined);

    useEffect(() => {
        const prevKey = previousKeyRef.current;
        previousKeyRef.current = providerKey;

        // Check if this is a completely new session (dialog reopened)
        // providerKey format is "closed" or "providerUuid-modelName"
        if (prevKey === undefined || prevKey === 'closed' || providerKey === 'closed') {
            // New session - reset initialization state
            setInternalCurrentTab(undefined);
            setIsInitialized(false);
        }
        // Otherwise it's just a provider change within the same session - preserve all state
    }, [providerKey]);

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
        setSnackbar({ open: true, message, severity });
    }, []);

    const hideSnackbar = useCallback(() => {
        setSnackbar(prev => ({ ...prev, open: false }));
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
        snackbar,
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

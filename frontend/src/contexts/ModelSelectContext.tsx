import React, { createContext, useContext, useState, useCallback, ReactNode } from 'react';
import type { Provider } from '../types/provider';

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
    internalCurrentTab: number;
    setInternalCurrentTab: (tab: number) => void;
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
}

const ModelSelectContext = createContext<ModelSelectContextValue | undefined>(undefined);

export interface ModelSelectProviderProps {
    children: ReactNode;
}

export function ModelSelectProvider({ children }: ModelSelectProviderProps) {
    const [internalCurrentTab, setInternalCurrentTab] = useState(0);
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

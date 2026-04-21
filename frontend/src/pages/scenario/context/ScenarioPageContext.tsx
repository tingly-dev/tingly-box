import React, { createContext, useContext, useState, useCallback, type ReactNode } from 'react';
import { useFunctionPanelData } from '@/hooks/useFunctionPanelData';

/**
 * Context for sharing modal state across scenario page components.
 *
 * Problem: ProviderConfigCard (in scenario page) and TemplatePage both need
 * to show the ApiKeyModal. Without context, they would have separate modal states.
 *
 * Solution: This context provides a single source of truth for modal state.
 */

interface ScenarioPageContextValue {
    showTokenModal: boolean;
    setShowTokenModal: (show: boolean) => void;
    token: string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
}

const ScenarioPageContext = createContext<ScenarioPageContextValue | undefined>(undefined);

export const useScenarioPageModal = () => {
    const context = useContext(ScenarioPageContext);
    if (!context) {
        throw new Error('useScenarioPageModal must be used within ScenarioPageModalProvider');
    }
    return context;
};

interface ScenarioPageModalProviderProps {
    children: ReactNode;
}

/**
 * Provider that wraps scenario pages to share modal state.
 * Uses useFunctionPanelData internally to get token and copy functionality.
 */
export const ScenarioPageModalProvider: React.FC<ScenarioPageModalProviderProps> = ({ children }) => {
    // Get modal state from useFunctionPanelData
    const { showTokenModal, setShowTokenModal, token, copyToClipboard } = useFunctionPanelData();

    const value: ScenarioPageContextValue = {
        showTokenModal,
        setShowTokenModal,
        token,
        copyToClipboard,
    };

    return (
        <ScenarioPageContext.Provider value={value}>
            {children}
        </ScenarioPageContext.Provider>
    );
};

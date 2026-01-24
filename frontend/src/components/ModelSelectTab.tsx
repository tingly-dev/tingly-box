import { Box } from '@mui/material';
import React, { useEffect, useCallback } from 'react';
import { dispatchCustomModelUpdate, listenForCustomModelUpdates, useCustomModels } from '../hooks/useCustomModels';
import { useGridLayout } from '../hooks/useGridLayout';
import { useProviderGroups } from '../hooks/useProviderGroups';
import { useModelSelection } from '../hooks/useModelSelection';
import { ModelSelectProvider, useModelSelectContext } from '../contexts/ModelSelectContext';
import type { Provider, ProviderModelsDataByUuid } from '../types/provider';
import { getModelTypeInfo, navigateToModelPage } from '../utils/modelUtils';
import { ProviderSidebar, ModelsPanel, CustomModelDialog } from './model-select';
import { Alert, Snackbar } from '@mui/material';

export interface ProviderSelectTabOption {
    provider: Provider;
    model?: string;
}

interface ModelSelectTabProps {
    providers: Provider[];
    providerModels?: ProviderModelsDataByUuid;
    selectedProvider?: string; // This is now UUID
    selectedModel?: string;
    activeTab?: number;
    onSelected?: (option: ProviderSelectTabOption) => void;
    onProviderChange?: (provider: Provider) => void; // Called when switching to a provider tab
    onRefresh?: (provider: Provider) => void;
    onCustomModelSave?: (provider: Provider, customModel: string) => void;
    refreshingProviders?: string[]; // These are UUIDs
    // Single provider mode props
    singleProvider?: Provider | null; // If provided, only show this provider
    onTest?: (model: string) => void; // Callback for Test button
    testing?: boolean; // Whether a test is in progress
}

function ModelSelectTabInner({
    providers,
    providerModels,
    selectedProvider,
    selectedModel,
    activeTab: externalActiveTab,
    onSelected,
    onProviderChange,
    onRefresh,
    onCustomModelSave,
    refreshingProviders = [],
    singleProvider,
    onTest,
    testing = false,
}: ModelSelectTabProps) {
    const { customModels, removeCustomModel } = useCustomModels();
    const gridLayout = useGridLayout();
    const {
        internalCurrentTab,
        setInternalCurrentTab,
        isInitialized,
        setIsInitialized,
        snackbar,
        hideSnackbar,
    } = useModelSelectContext();

    const { handleModelSelect } = useModelSelection({ onSelected });

    const {
        groupedProviders,
        flattenedProviders,
        isSingleProviderMode,
        displayProviders,
    } = useProviderGroups(providers, singleProvider);

    // Use external activeTab if provided, otherwise use internal state
    const currentTab = externalActiveTab !== undefined ? externalActiveTab : internalCurrentTab;

    // Listen for custom model updates from other components
    useEffect(() => {
        const cleanup = listenForCustomModelUpdates(() => {
            // The hook will automatically handle state updates
        });
        return cleanup;
    }, []);

    const handleTabChange = useCallback((newValue: number) => {
        if (externalActiveTab === undefined) {
            setInternalCurrentTab(newValue);
        }

        // Get the target provider from flattened list
        const targetProvider = flattenedProviders[newValue];
        if (!targetProvider) return;

        // Notify parent component about provider change
        if (onProviderChange) {
            onProviderChange(targetProvider);
        }

        // Auto-navigate to page containing selected model when switching tabs
        if (selectedProvider === targetProvider.uuid && selectedModel) {
            const modelTypeInfo = getModelTypeInfo(targetProvider, providerModels, customModels);
            const { isCustomModel, allModelsForSearch } = modelTypeInfo;

            // Only navigate to page for standard models, not custom models
            if (!isCustomModel(selectedModel)) {
                const standardModels = allModelsForSearch.filter(model => !isCustomModel(model));
                // Need to import or create setCurrentPage function
                // For now, we'll handle this differently
            }
        }
    }, [externalActiveTab, flattenedProviders, onProviderChange, selectedProvider, selectedModel, providerModels, customModels, setInternalCurrentTab]);

    const handleDeleteCustomModel = useCallback((provider: Provider, customModel: string) => {
        removeCustomModel(provider.uuid, customModel);
        dispatchCustomModelUpdate(provider.uuid, customModel);
    }, [removeCustomModel]);

    const handleCustomModelSave = useCallback((providerUuid: string, customModel: string) => {
        if (onCustomModelSave) {
            const provider = providers.find(p => p.uuid === providerUuid);
            if (provider) {
                onCustomModelSave(provider, customModel);
            }
        }
    }, [onCustomModelSave, providers]);

    // Auto-switch to selected provider tab and navigate to selected model on component mount (only once)
    useEffect(() => {
        if (!isInitialized && selectedProvider) {
            const targetProviderIndex = flattenedProviders.findIndex(provider => provider.uuid === selectedProvider);

            // Auto-switch to the selected provider's tab
            if (targetProviderIndex !== -1) {
                if (externalActiveTab === undefined) {
                    setInternalCurrentTab(targetProviderIndex);
                }

                // Fetch models for the selected provider on initial load
                const targetProvider = flattenedProviders[targetProviderIndex];
                if (onProviderChange) {
                    onProviderChange(targetProvider);
                }
            }

            // Mark as initialized to prevent further automatic switching
            setIsInitialized(true);
        }
    }, [isInitialized, selectedProvider, flattenedProviders, externalActiveTab, onProviderChange, setInternalCurrentTab, setIsInitialized]);

    return (
        <Box sx={{ display: 'flex', flexDirection: 'row', height: '100%', width: '100%' }}>
            {/* Left Sidebar - Vertical Tabs */}
            <ProviderSidebar
                groupedProviders={groupedProviders}
                currentTab={currentTab}
                selectedProvider={selectedProvider}
                onTabChange={handleTabChange}
            />

            {/* Right Panel - Tab Content */}
            <ModelsPanel
                flattenedProviders={flattenedProviders}
                providerModels={providerModels}
                selectedProvider={selectedProvider}
                selectedModel={selectedModel}
                currentTab={currentTab}
                refreshingProviders={refreshingProviders}
                columns={gridLayout.columns}
                modelsPerPage={gridLayout.modelsPerPage}
                onModelSelect={handleModelSelect}
                onRefresh={onRefresh}
                onCustomModelEdit={() => {/* Handled by context */}}
                onCustomModelDelete={handleDeleteCustomModel}
                onTest={onTest}
                testing={testing}
            />

            {/* Custom Model Dialog */}
            <CustomModelDialog onCustomModelSave={handleCustomModelSave} />

            {/* Snackbar for notifications */}
            <Snackbar
                open={snackbar.open}
                autoHideDuration={6000}
                onClose={hideSnackbar}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
            >
                <Alert
                    onClose={hideSnackbar}
                    severity={snackbar.severity}
                    sx={{ width: '100%' }}
                >
                    {snackbar.message}
                </Alert>
            </Snackbar>
        </Box>
    );
}

export default function ModelSelectTab(props: ModelSelectTabProps) {
    return (
        <ModelSelectProvider>
            <ModelSelectTabInner {...props} />
        </ModelSelectProvider>
    );
}

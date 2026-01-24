import { Box } from '@mui/material';
import React, { useEffect, useCallback } from 'react';
import { useCustomModels } from '../hooks/useCustomModels';
import { useProviderModels } from '../hooks/useProviderModels';
import { useGridLayout } from '../hooks/useGridLayout';
import { useProviderGroups } from '../hooks/useProviderGroups';
import { useModelSelection } from '../hooks/useModelSelection';
import { ModelSelectProvider, useModelSelectContext } from '../contexts/ModelSelectContext';
import type { Provider } from '../types/provider';
import { getModelTypeInfo } from '../utils/modelUtils';
import { ProviderSidebar, ModelsPanel, CustomModelDialog } from './model-select';
import { Alert, Snackbar } from '@mui/material';

export interface ProviderSelectTabOption {
    provider: Provider;
    model: string;
}

interface ModelSelectTabProps {
    providers: Provider[];
    selectedProvider?: string; // This is now UUID
    selectedModel?: string;
    activeTab?: string; // Provider UUID
    onSelected?: (option: ProviderSelectTabOption) => void;
    onProviderChange?: (provider: Provider) => void; // Called when switching to a provider tab
    onCustomModelSave?: (provider: Provider, customModel: string) => void;
    // Single provider mode props
    singleProvider?: Provider | null; // If provided, only show this provider
    onTest?: (model: string) => void; // Callback for Test button
    testing?: boolean; // Whether a test is in progress
}

function ModelSelectTabInner({
    providers,
    selectedProvider,
    selectedModel,
    activeTab: externalActiveTab,
    onSelected,
    onProviderChange,
    onCustomModelSave,
    singleProvider,
    onTest,
    testing = false,
}: ModelSelectTabProps) {
    const { customModels, removeCustomModel, saveCustomModel, updateCustomModel } = useCustomModels();
    const { providerModels, refreshingProviders, fetchModels, refreshModels } = useProviderModels();
    const gridLayout = useGridLayout();
    const {
        internalCurrentTab,
        setInternalCurrentTab,
        isInitialized,
        setIsInitialized,
        snackbar,
        hideSnackbar,
        openCustomModelDialog,
        closeCustomModelDialog,
        customModelDialog,
    } = useModelSelectContext();

    const { handleModelSelect } = useModelSelection({ onSelected });

    const {
        groupedProviders,
        flattenedProviders,
    } = useProviderGroups(providers, singleProvider);

    // Use external activeTab if provided, otherwise use internal state
    const currentTab = externalActiveTab !== undefined ? externalActiveTab : internalCurrentTab;

    const handleTabChange = useCallback(async (providerUuid: string) => {
        if (externalActiveTab === undefined) {
            setInternalCurrentTab(providerUuid);
        }

        // Get the target provider from flattened list
        const targetProvider = flattenedProviders.find(p => p.uuid === providerUuid);
        if (!targetProvider) return;

        // Fetch models for this provider
        await fetchModels(providerUuid);

        // Notify parent component about provider change
        if (onProviderChange) {
            onProviderChange(targetProvider);
        }
    }, [externalActiveTab, flattenedProviders, onProviderChange, setInternalCurrentTab, fetchModels]);

    const handleDeleteCustomModel = useCallback((provider: Provider, customModel: string) => {
        removeCustomModel(provider.uuid, customModel);
    }, [removeCustomModel]);

    const handleCustomModelEdit = useCallback((provider: Provider, currentValue?: string) => {
        openCustomModelDialog(provider, currentValue);
    }, [openCustomModelDialog]);

    const handleCustomModelSave = useCallback(() => {
        const customModel = customModelDialog.value?.trim();
        if (customModel && customModelDialog.provider) {
            if (customModelDialog.originalValue) {
                // Editing: use updateCustomModel to atomically replace old value with new value
                updateCustomModel(customModelDialog.provider.uuid, customModelDialog.originalValue, customModel);
            } else {
                // Adding new: use saveCustomModel
                saveCustomModel(customModelDialog.provider.uuid, customModel);
            }

            // Then save to persistence through parent component
            if (onCustomModelSave) {
                onCustomModelSave(customModelDialog.provider, customModel);
            }
        }
        closeCustomModelDialog();
    }, [customModelDialog, saveCustomModel, updateCustomModel, onCustomModelSave, closeCustomModelDialog]);

    // Auto-switch to selected provider tab and navigate to selected model on component mount (only once)
    useEffect(() => {
        if (!isInitialized && selectedProvider) {
            const targetProviderIndex = flattenedProviders.findIndex(provider => provider.uuid === selectedProvider);

            // Auto-switch to the selected provider's tab
            if (targetProviderIndex !== -1) {
                if (externalActiveTab === undefined) {
                    setInternalCurrentTab(selectedProvider);
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
            {currentTab && (() => {
                const currentProvider = flattenedProviders.find(p => p.uuid === currentTab);
                if (!currentProvider) return null;

                return (
                    <ModelsPanel
                        provider={currentProvider}
                        selectedProvider={selectedProvider}
                        selectedModel={selectedModel}
                        columns={gridLayout.columns}
                        modelsPerPage={gridLayout.modelsPerPage}
                        onModelSelect={handleModelSelect}
                        onCustomModelEdit={handleCustomModelEdit}
                        onCustomModelDelete={handleDeleteCustomModel}
                        onTest={onTest}
                        testing={testing}
                    />
                );
            })()}

            {/* Custom Model Dialog */}
            <CustomModelDialog onSave={handleCustomModelSave} />

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

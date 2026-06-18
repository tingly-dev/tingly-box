import { Box } from '@mui/material';
import React, { useEffect, useCallback, useRef } from 'react';
import { useCustomModels } from '@/hooks/useCustomModels';
import { useProviderModels } from '@/hooks/useProviderModels';
import { useGridLayout } from '@/hooks/useGridLayout';
import { useProviderGroups } from '@/hooks/useProviderGroups';
import { useModelSelection } from '@/hooks/useModelSelection';
import { useRecentModels } from '@/hooks/useRecentModels';
import { ModelSelectProvider, useModelSelectContext } from '@/contexts/ModelSelectContext';
import type { Provider } from '@/types/provider';
import { getModelTypeInfo } from '@/utils/modelUtils';
import { ProviderSidebar, ModelsPanel, CustomModelDialog } from './model-select';

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
    onSelectionClear?: () => void; // Called when selection should be cleared (e.g., after deleting selected model)
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
    onSelectionClear,
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
        openCustomModelDialog,
        closeCustomModelDialog,
        customModelDialog,
        triggerRefresh,
    } = useModelSelectContext();

    const { handleModelSelect } = useModelSelection({ onSelected });
    const { recentModels, lastProvider } = useRecentModels();

    const {
        groupedProviders,
        flattenedProviders,
    } = useProviderGroups(providers, singleProvider);

    // Use external activeTab if provided, otherwise use internal state.
    // Fallback chain to prevent flickering:
    //   1. externalActiveTab — parent-controlled tab
    //   2. internalCurrentTab — user's in-session tab switch
    //   3. selectedProvider — lock onto the selected provider if a model is chosen
    //   4. lastProvider — remember the last chosen provider when nothing is selected
    //   5. first available provider — final default
    const currentTab = externalActiveTab
        ?? internalCurrentTab
        ?? (selectedProvider && selectedModel ? selectedProvider : undefined)
        ?? lastProvider
        ?? flattenedProviders[0]?.uuid;

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

        // If the deleted model is currently selected, clear the selection
        // Use onSelectionClear to avoid triggering the parent's save/close logic
        if (selectedProvider === provider.uuid && selectedModel === customModel && onSelectionClear) {
            onSelectionClear();
        }

        // Trigger refresh to update UI
        triggerRefresh();
    }, [removeCustomModel, selectedProvider, selectedModel, onSelectionClear, triggerRefresh]);

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

    // Auto-switch to selected provider tab and navigate to selected model on component mount
    // Use ref to track which provider we've initialized for to prevent duplicate fetches
    const initializedProviderRef = useRef<string | null>(null);

    useEffect(() => {
        if (selectedProvider) {
            // Skip if already initialized for this provider
            if (initializedProviderRef.current === selectedProvider) {
                return;
            }

            const targetProviderIndex = flattenedProviders.findIndex(provider => provider.uuid === selectedProvider);

            // Auto-switch to the selected provider's tab
            if (targetProviderIndex !== -1) {
                if (externalActiveTab === undefined) {
                    setInternalCurrentTab(selectedProvider);
                }

                // Fetch models for the selected provider
                fetchModels(selectedProvider);

                // Notify parent component about provider change
                const targetProvider = flattenedProviders[targetProviderIndex];
                if (onProviderChange) {
                    onProviderChange(targetProvider);
                }

                // Mark this provider as initialized
                initializedProviderRef.current = selectedProvider;
            }
        } else if (lastProvider && initializedProviderRef.current !== lastProvider) {
            // No selection yet: open on the most recently used provider.
            // Fetch its models so the right panel is populated on first open.
            if (externalActiveTab === undefined) {
                setInternalCurrentTab(lastProvider);
            }
            fetchModels(lastProvider);
            const targetProvider = flattenedProviders.find(p => p.uuid === lastProvider);
            if (targetProvider && onProviderChange) {
                onProviderChange(targetProvider);
            }
            initializedProviderRef.current = lastProvider;
        }
    }, [selectedProvider, lastProvider, flattenedProviders, externalActiveTab, onProviderChange, setInternalCurrentTab, fetchModels]);

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
        </Box>
    );
}

export default function ModelSelectDialog(props: ModelSelectTabProps) {
    // Create a unique key based on selected provider and model to force context reset when selection changes
    const providerKey = `${props.selectedProvider || ''}-${props.selectedModel || ''}`;
    return (
        <ModelSelectProvider key={providerKey}>
            <ModelSelectTabInner {...props} />
        </ModelSelectProvider>
    );
}

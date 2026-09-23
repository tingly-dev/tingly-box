import { Box } from '@mui/material';
import React, { useEffect, useCallback, useMemo, useRef, useState } from 'react';
import { api } from '@/services/api';
import { useCustomModels } from '@/hooks/useCustomModels';
import { useProviderModels } from '@/hooks/useProviderModels';
import { useGridLayout } from '@/hooks/useGridLayout';
import { useProviderGroups } from '@/hooks/useProviderGroups';
import { useRecentModels } from '@/hooks/useRecentModels';
import { useProviderEditDialog } from '@/hooks/useProviderEditDialog';
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
        showSnackbar,
    } = useModelSelectContext();

    const { recentModels, lastProvider, addRecentModel } = useRecentModels();

    const handleModelSelect = useCallback(async (provider: Provider, model: string) => {
        onSelected?.({ provider, model });
        // Track recent model
        addRecentModel(provider.uuid, model);
    }, [onSelected, addRecentModel]);

    // Providers edited through the in-dialog "Edit Provider" button. Callers
    // own the providers prop and only refetch it on their own surfaces, so an
    // edit (e.g. a rename) wouldn't show up here until the dialog reopens.
    // Overlay the freshly-fetched record on the prop until the next refresh.
    const [providerOverrides, setProviderOverrides] = useState<Record<string, Provider>>({});
    const effectiveProviders = useMemo(
        () => providers.map(p => providerOverrides[p.uuid] ?? p),
        [providers, providerOverrides]
    );

    const handleProviderUpdated = useCallback(async (providerUuid: string) => {
        const result = await api.getProvider(providerUuid);
        if (result.success) {
            setProviderOverrides(prev => ({ ...prev, [providerUuid]: result.data as Provider }));
        }
        // The edit may change api style/endpoint — refetch models for the tab.
        fetchModels(providerUuid);
        triggerRefresh();
    }, [fetchModels, triggerRefresh]);

    const { editProvider, providerEditDialogs } = useProviderEditDialog({
        onUpdated: handleProviderUpdated,
        showNotification: showSnackbar,
    });

    const {
        groupedProviders,
        flattenedProviders,
    } = useProviderGroups(effectiveProviders, singleProvider);

    // Use external activeTab if provided, otherwise use internal state.
    // Fallback chain to prevent flickering:
    //   1. externalActiveTab — parent-controlled tab
    //   2. internalCurrentTab — user's in-session tab switch
    //   3. selectedProvider — lock onto the selected provider if a model is chosen
    //   4. lastProvider — remember the last chosen provider when nothing is selected
    //   5. first available provider — final default
    // The winner must resolve to an existing provider: an absent (empty-string)
    // or stale (deleted provider) reference would otherwise leave the right
    // panel blank, so fall through to the first provider instead.
    const requestedTab = externalActiveTab
        ?? internalCurrentTab
        ?? (selectedProvider && selectedModel ? selectedProvider : undefined)
        ?? (lastProvider || undefined);
    const currentTab = flattenedProviders.some(p => p.uuid === requestedTab)
        ? requestedTab
        : flattenedProviders[0]?.uuid;

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
                        onProviderEdit={(provider) => editProvider(provider.uuid)}
                    />
                );
            })()}

            {/* Custom Model Dialog */}
            <CustomModelDialog onSave={handleCustomModelSave} />

            {/* Provider edit dialogs (API-key form / OAuth detail) */}
            {providerEditDialogs}
        </Box>
    );
}

export default function ModelSelectDialog(props: ModelSelectTabProps) {
    // Reset internal tab/dialog state only when the underlying provider selection
    // changes (a genuinely new session) — NOT on every model pick within the same
    // session. selectedModel changes on every card click (e.g. while browsing/
    // testing in ModelListDialog), and remounting on that would blow away
    // per-card state like a model's persistent test-result badge.
    const providerKey = props.selectedProvider || '';
    return (
        <ModelSelectProvider key={providerKey}>
            <ModelSelectTabInner {...props} />
        </ModelSelectProvider>
    );
}

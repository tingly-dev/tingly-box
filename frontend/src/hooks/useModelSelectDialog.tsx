import { Dialog, DialogContent, DialogTitle } from '@mui/material';
import React, { useCallback, useRef, useState } from 'react';
import { api } from '../services/api';
import type { Provider, ProviderModelsDataByUuid } from '../types/provider';
import ModelSelectTab, { type ProviderSelectTabOption } from '../components/ModelSelectTab';
import type { ConfigRecord, Rule } from '../components/RoutingGraphTypes.ts';
import { v4 as uuidv4 } from 'uuid';

export interface ModelSelectOptions {
    ruleUuid: string;
    configRecord: ConfigRecord;
    providerUuid?: string; // The uuid of the service to edit, or "smart:${index}" for adding to smart rule
    mode?: 'edit' | 'add';
}

export interface UseModelSelectDialogOptions {
    providers: Provider[];
    providerModels?: ProviderModelsDataByUuid;
    onProviderModelsChange?: (providerUuid: string, models: any) => void;
    rules: Rule[];
    onRuleChange?: (updatedRule: Rule) => void;
    showNotification: (message: string, severity: 'success' | 'error') => void;
    onRefreshProvider?: (providerUuid: string) => void;
    refreshingProviders?: string[];
}

interface EditingServiceContext {
    isSmartRouting: boolean;
    smartRuleIndex?: number;
}

interface ModelSelectDialogProps {
    open: boolean;
    onClose: () => void;
}

export const useModelSelectDialog = (options: UseModelSelectDialogOptions) => {
    const {
        providers,
        providerModels = {},
        onProviderModelsChange,
        rules,
        onRuleChange,
        showNotification,
        onRefreshProvider,
        refreshingProviders = []
    } = options;

    // Dialog state
    const [open, setOpen] = useState(false);
    const [mode, setMode] = useState<'edit' | 'add'>('add');
    const [editingProviderUuid, setEditingProviderUuid] = useState<string | null>(null);
    const [currentRuleUuid, setCurrentRuleUuid] = useState<string | null>(null);
    const [currentConfigRecord, setCurrentConfigRecord] = useState<ConfigRecord | null>(null);

    // Refs for tracking context
    const currentSmartRuleIndexRef = useRef<number | null>(null);
    const editingServiceContextRef = useRef<EditingServiceContext | null>(null);

    // Find the service in both providers and smartRouting
    const findService = useCallback((configRecord: ConfigRecord, serviceUuid: string) => {
        // First check providers
        const providerService = configRecord.providers.find(p => p.uuid === serviceUuid);
        if (providerService) {
            return { service: providerService, isSmartRouting: false };
        }

        // Then check smartRouting services
        if (configRecord.smartRouting) {
            for (let i = 0; i < configRecord.smartRouting.length; i++) {
                const rule = configRecord.smartRouting[i];
                if (rule.services) {
                    const service = rule.services.find(s => s.uuid === serviceUuid);
                    if (service) {
                        return { service, isSmartRouting: true, smartRuleIndex: i };
                    }
                }
            }
        }

        return null;
    }, []);

    // Fetch models for a provider - updates parent state
    const handleFetchModels = useCallback(async (providerUuid: string) => {
        if (!providerUuid) return;

        // If already have models, skip
        if (providerModels[providerUuid]) {
            return;
        }

        try {
            const result = await api.getProviderModelsByUUID(providerUuid);
            if (result.success && result.data) {
                // If GET returns empty list, auto-fetch from Provider API
                if (!result.data.models || result.data.models.length === 0) {
                    const refreshResult = await api.updateProviderModelsByUUID(providerUuid);
                    if (refreshResult.success && refreshResult.data && onProviderModelsChange) {
                        onProviderModelsChange(providerUuid, refreshResult.data);
                    }
                } else if (onProviderModelsChange) {
                    onProviderModelsChange(providerUuid, result.data);
                }
            }
        } catch (error) {
            console.error(`Failed to fetch models for provider ${providerUuid}:`, error);
        }
    }, [providerModels, onProviderModelsChange]);

    // Open the dialog
    const openModelSelect = useCallback((options: ModelSelectOptions) => {
        const { ruleUuid, configRecord, providerUuid, mode: newMode = 'edit' } = options;

        setCurrentRuleUuid(ruleUuid);
        setCurrentConfigRecord(configRecord);
        setMode(newMode);

        // Check if providerUuid is a smart rule reference (format: "smart:${index}")
        if (providerUuid?.startsWith('smart:')) {
            const index = parseInt(providerUuid.substring(6), 10);
            currentSmartRuleIndexRef.current = index;
            setEditingProviderUuid(null);
            editingServiceContextRef.current = null;
        } else {
            currentSmartRuleIndexRef.current = null;
            setEditingProviderUuid(providerUuid || null);

            // In edit mode, determine if providerUuid refers to a service in smartRouting or providers
            if (newMode === 'edit' && providerUuid) {
                const found = findService(configRecord, providerUuid);
                if (found) {
                    editingServiceContextRef.current = {
                        isSmartRouting: found.isSmartRouting,
                        smartRuleIndex: found.smartRuleIndex,
                    };
                } else {
                    editingServiceContextRef.current = null;
                }
            } else {
                editingServiceContextRef.current = null;
            }
        }

        setOpen(true);
    }, [findService]);

    // Handle model selection
    const handleModelSelect = useCallback((option: ProviderSelectTabOption) => {
        if (!currentConfigRecord || !currentRuleUuid) return;

        const smartRuleIndex = currentSmartRuleIndexRef.current;
        const editingContext = editingServiceContextRef.current;

        let updated: ConfigRecord;

        // Check if we're adding to a smart rule by index
        if (smartRuleIndex !== null && smartRuleIndex >= 0 && mode === 'add') {
            // Add service to the specific smart rule by index
            updated = {
                ...currentConfigRecord,
                smartRouting: (currentConfigRecord.smartRouting || []).map((rule, index) => {
                    if (index === smartRuleIndex) {
                        const newService = { uuid: uuidv4(), provider: option.provider.uuid, model: option.model || '' };
                        return {
                            ...rule,
                            services: [
                                ...(rule.services || []),
                                newService,
                            ],
                        };
                    }
                    return rule;
                }),
            };
        } else if (mode === 'add') {
            // Add to default providers
            updated = {
                ...currentConfigRecord,
                providers: [
                    ...currentConfigRecord.providers,
                    { uuid: uuidv4(), provider: option.provider.uuid, model: option.model || '', isManualInput: false },
                ],
            };
        } else if (mode === 'edit' && editingProviderUuid) {
            // Edit existing provider or smart routing service
            if (editingContext?.isSmartRouting && editingContext.smartRuleIndex !== undefined) {
                // Edit service in smart routing
                updated = {
                    ...currentConfigRecord,
                    smartRouting: (currentConfigRecord.smartRouting || []).map((rule, index) => {
                        if (index === editingContext.smartRuleIndex) {
                            return {
                                ...rule,
                                services: (rule.services || []).map(service => {
                                    if (service.uuid === editingProviderUuid) {
                                        return { ...service, provider: option.provider.uuid, model: option.model || '' };
                                    }
                                    return service;
                                }),
                            };
                        }
                        return rule;
                    }),
                };
            } else {
                // Edit in default providers
                updated = {
                    ...currentConfigRecord,
                    providers: currentConfigRecord.providers.map(p => {
                        if (p.uuid === editingProviderUuid) {
                            return { ...p, provider: option.provider.uuid, model: option.model || '' };
                        }
                        return p;
                    }),
                };
            }
        } else {
            updated = currentConfigRecord;
        }

        // Save to backend
        const rule = rules.find(r => r.uuid === currentRuleUuid);
        if (rule && updated) {
            const ruleData = {
                uuid: rule.uuid,
                scenario: rule.scenario,
                request_model: updated.requestModel,
                response_model: updated.responseModel,
                active: updated.active,
                description: updated.description,
                services: updated.providers
                    .filter(p => p.provider && p.model)
                    .map(provider => ({
                        provider: provider.provider,
                        model: provider.model,
                        weight: provider.weight || 0,
                        active: provider.active !== undefined ? provider.active : true,
                        time_window: provider.time_window || 0,
                    })),
                smart_enabled: updated.smartEnabled || false,
                smart_routing: updated.smartRouting || [],
            };

            api.updateRule(rule.uuid, ruleData).then((result) => {
                if (result.success) {
                    showNotification(`Configuration saved successfully`, 'success');
                    if (onRuleChange) {
                        onRuleChange({
                            ...rule,
                            scenario: ruleData.scenario,
                            request_model: ruleData.request_model,
                            response_model: ruleData.response_model,
                            active: ruleData.active,
                            description: ruleData.description,
                            services: ruleData.services,
                            smart_enabled: ruleData.smart_enabled,
                            smart_routing: ruleData.smart_routing,
                        });
                    }
                } else {
                    showNotification(`Failed to save: ${result.error || 'Unknown error'}`, 'error');
                }
            });
        }

        // Close dialog and clean up
        setOpen(false);
        setCurrentRuleUuid(null);
        setCurrentConfigRecord(null);
        setEditingProviderUuid(null);
        currentSmartRuleIndexRef.current = null;
        editingServiceContextRef.current = null;
    }, [currentConfigRecord, currentRuleUuid, mode, editingProviderUuid, rules, onRuleChange, showNotification]);

    // Get selected provider and model for pre-selection
    const getSelectedProvider = useCallback(() => {
        if (mode === 'edit' && editingProviderUuid && currentConfigRecord) {
            const found = findService(currentConfigRecord, editingProviderUuid);
            return found?.service.provider;
        }
        return undefined;
    }, [mode, editingProviderUuid, currentConfigRecord, findService]);

    const getSelectedModel = useCallback(() => {
        if (mode === 'edit' && editingProviderUuid && currentConfigRecord) {
            const found = findService(currentConfigRecord, editingProviderUuid);
            return found?.service.model;
        }
        return undefined;
    }, [mode, editingProviderUuid, currentConfigRecord, findService]);

    // Get a unique key for ModelSelectTab to force remount when selection changes
    const dialogKey = open ? `${getSelectedProvider() || ''}-${getSelectedModel() || ''}` : 'closed';

    // Close dialog
    const closeModelSelect = useCallback(() => {
        setOpen(false);
        setCurrentRuleUuid(null);
        setCurrentConfigRecord(null);
        setEditingProviderUuid(null);
        currentSmartRuleIndexRef.current = null;
        editingServiceContextRef.current = null;
    }, []);

    // Dialog component
    const ModelSelectDialog: React.FC<ModelSelectDialogProps> = ({ open: dialogOpen, onClose }) => (
        <Dialog
            open={dialogOpen}
            onClose={() => {
                closeModelSelect();
                onClose?.();
            }}
            maxWidth="lg"
            fullWidth
            PaperProps={{
                sx: { height: '80vh' }
            }}
        >
            <DialogTitle sx={{ textAlign: 'center' }}>
                {mode === 'add' ? 'Add API Key' : 'Choose Model'}
            </DialogTitle>
            <DialogContent>
                <ModelSelectTab
                    key={dialogKey}
                    providers={providers}
                    providerModels={providerModels}
                    selectedProvider={getSelectedProvider()}
                    selectedModel={getSelectedModel()}
                    onSelected={handleModelSelect}
                    onProviderChange={(p) => handleFetchModels(p.uuid)}
                    onRefresh={(p) => onRefreshProvider?.(p.uuid)}
                    refreshingProviders={refreshingProviders}
                />
            </DialogContent>
        </Dialog>
    );

    return {
        openModelSelect,
        closeModelSelect,
        ModelSelectDialog,
        isOpen: open,
    };
};

import { Dialog, DialogContent, DialogTitle } from '@mui/material';
import React, { useCallback, useRef, useState } from 'react';
import type { Provider } from '../types/provider';
import ModelSelectDialog, { type ProviderSelectTabOption } from '@/components/ModelSelectDialog.tsx';
import type { ConfigRecord, Rule } from '@/components/RoutingGraphTypes.ts';
import { v4 as uuidv4 } from 'uuid';
import api from "@/services/api.ts";
import { buildRuleUpdatePayload } from '@/components/rule-card/ruleUpdatePayload';

export interface ModelSelectOptions {
    ruleUuid: string;
    configRecord: ConfigRecord;
    providerUuid?: string; // The uuid of the service to edit, or "smart:${index}" for adding to smart rule
    mode?: 'edit' | 'add';
    addTier?: number; // Tier to assign when mode='add' (for tier-based adds)
}

export interface UseModelSelectDialogOptions {
    providers: Provider[];
    rules: Rule[];
    onRuleChange?: (updatedRule: Rule) => void;
    showNotification: (message: string, severity: 'success' | 'error') => void;
    onCreateFromModel?: (option: ProviderSelectTabOption) => void;
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
        rules,
        onRuleChange,
        showNotification,
        onCreateFromModel,
    } = options;

    // Dialog state
    const [open, setOpen] = useState(false);
    const [mode, setMode] = useState<'edit' | 'add' | 'create-rule'>('add');
    const [editingProviderUuid, setEditingProviderUuid] = useState<string | null>(null);
    const [currentRuleUuid, setCurrentRuleUuid] = useState<string | null>(null);
    const [currentConfigRecord, setCurrentConfigRecord] = useState<ConfigRecord | null>(null);
    const [modelSelectionCleared, setModelSelectionCleared] = useState(false);
    const [currentAddTier, setCurrentAddTier] = useState<number | undefined>(undefined);

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

    // Open the dialog
    const openModelSelect = useCallback((options: ModelSelectOptions) => {
        const { ruleUuid, configRecord, providerUuid, mode: newMode = 'edit', addTier } = options;

        setCurrentRuleUuid(ruleUuid);
        setCurrentConfigRecord(configRecord);
        setMode(newMode);
        setModelSelectionCleared(false);
        setCurrentAddTier(addTier);

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

    const openModelSelectForCreate = useCallback(() => {
        setMode('create-rule');
        setCurrentRuleUuid(null);
        setCurrentConfigRecord(null);
        setEditingProviderUuid(null);
        setModelSelectionCleared(false);
        setCurrentAddTier(undefined);
        currentSmartRuleIndexRef.current = null;
        editingServiceContextRef.current = null;
        setOpen(true);
    }, []);

    // Handle model selection
    const handleModelSelect = useCallback((option: ProviderSelectTabOption) => {
        if (mode === 'create-rule') {
            setOpen(false);
            onCreateFromModel?.(option);
            return;
        }
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
                        const newService = { uuid: uuidv4(), provider: option.provider.uuid, model: option.model || '', active: true };
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
            // Add to default providers, preserving tier priority if specified
            updated = {
                ...currentConfigRecord,
                providers: [
                    ...currentConfigRecord.providers,
                    { uuid: uuidv4(), provider: option.provider.uuid, model: option.model || '', isManualInput: false, tier: currentAddTier ?? 0 },
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
            const ruleData = buildRuleUpdatePayload(rule, updated);

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
                            flags: ruleData.flags,
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
        setCurrentAddTier(undefined);
        currentSmartRuleIndexRef.current = null;
        editingServiceContextRef.current = null;
    }, [currentConfigRecord, currentAddTier, currentRuleUuid, mode, editingProviderUuid, rules, onRuleChange, showNotification, onCreateFromModel]);

    // Get selected provider and model for pre-selection
    const getSelectedProvider = useCallback(() => {
        if (mode === 'edit' && editingProviderUuid && currentConfigRecord) {
            const found = findService(currentConfigRecord, editingProviderUuid);
            return found?.service.provider;
        }
        return undefined;
    }, [mode, editingProviderUuid, currentConfigRecord, findService]);

    const getSelectedModel = useCallback(() => {
        // If model selection was cleared (e.g., after deleting a custom model), return undefined
        if (modelSelectionCleared) {
            return undefined;
        }
        if (mode === 'edit' && editingProviderUuid && currentConfigRecord) {
            const found = findService(currentConfigRecord, editingProviderUuid);
            return found?.service.model;
        }
        return undefined;
    }, [mode, editingProviderUuid, currentConfigRecord, findService, modelSelectionCleared]);

    // Get a unique key for ModelSelectTab to force remount when selection changes
    const dialogKey = open ? `${getSelectedProvider() || ''}-${getSelectedModel() || ''}` : 'closed';

    // Close dialog
    const closeModelSelect = useCallback(() => {
        setOpen(false);
        setCurrentRuleUuid(null);
        setCurrentConfigRecord(null);
        setEditingProviderUuid(null);
        setCurrentAddTier(undefined);
        currentSmartRuleIndexRef.current = null;
        editingServiceContextRef.current = null;
    }, []);

    // Dialog component
    const WrappedModelSelectDialog: React.FC<ModelSelectDialogProps> = ({ open: dialogOpen, onClose }) => (
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
                {mode === 'create-rule'
                    ? 'Select a model for your new rule'
                    : mode === 'add'
                        ? 'Connect AI'
                        : 'Choose Model'}
            </DialogTitle>
            <DialogContent>
                <ModelSelectDialog
                    key={dialogKey}
                    providers={providers}
                    selectedProvider={getSelectedProvider()}
                    selectedModel={getSelectedModel()}
                    onSelected={handleModelSelect}
                    onSelectionClear={() => setModelSelectionCleared(true)}
                />
            </DialogContent>
        </Dialog>
    );

    return {
        openModelSelect,
        openModelSelectForCreate,
        closeModelSelect,
        ModelSelectDialog: WrappedModelSelectDialog,
        isOpen: open,
    };
};

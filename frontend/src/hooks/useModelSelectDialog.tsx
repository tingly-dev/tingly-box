import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, FormControlLabel, Switch, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import React, { useCallback, useRef, useState } from 'react';
import type { Provider } from '../types/provider';
import ModelSelectDialog, { type ProviderSelectTabOption } from '@/components/ModelSelectDialog.tsx';
import type { ConfigRecord, Rule } from '@/components/RoutingGraphTypes.ts';
import { v4 as uuidv4 } from 'uuid';
import api from "@/services/api.ts";
import { useStableComponent } from '@/hooks/useStableComponent';
import { buildRuleUpdatePayload } from '@/components/rule-card/ruleUpdatePayload';

export interface ModelSelectOptions {
    ruleUuid: string;
    configRecord: ConfigRecord;
    serviceUuid?: string; // The uuid of the service to edit, or "smart:${index}" for adding to smart rule
    mode?: 'edit' | 'add';
    addTier?: number; // Tier to assign when mode='add' (for tier-based adds)
}

export interface UseModelSelectDialogOptions {
    providers: Provider[];
    rules: Rule[];
    onRuleChange?: (updatedRule: Rule) => void;
    showNotification: (message: string, severity: 'success' | 'error') => void;
    // Called from New Model with every picked model; the caller creates one rule each.
    onCreateFromModels?: (options: ProviderSelectTabOption[]) => void;
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
        onCreateFromModels,
    } = options;
    const { t } = useTranslation();

    // Dialog state
    const [open, setOpen] = useState(false);
    const [mode, setMode] = useState<'edit' | 'add' | 'create-rule'>('add');
    const [editingServiceUuid, setEditingServiceUuid] = useState<string | null>(null);
    const [currentRuleUuid, setCurrentRuleUuid] = useState<string | null>(null);
    const [currentConfigRecord, setCurrentConfigRecord] = useState<ConfigRecord | null>(null);
    const [modelSelectionCleared, setModelSelectionCleared] = useState(false);
    const [currentAddTier, setCurrentAddTier] = useState<number | undefined>(undefined);
    // New Model starts single-select (a click creates the rule); the toolbar's
    // switch turns on multi-select, where clicks collect models in pick order.
    const [multiSelect, setMultiSelect] = useState(false);
    const [picked, setPicked] = useState<ProviderSelectTabOption[]>([]);

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
        const { ruleUuid, configRecord, serviceUuid, mode: newMode = 'edit', addTier } = options;

        setCurrentRuleUuid(ruleUuid);
        setCurrentConfigRecord(configRecord);
        setMode(newMode);
        setModelSelectionCleared(false);
        setCurrentAddTier(addTier);

        // Check if serviceUuid is a smart rule reference (format: "smart:${index}")
        if (serviceUuid?.startsWith('smart:')) {
            const index = parseInt(serviceUuid.substring(6), 10);
            currentSmartRuleIndexRef.current = index;
            setEditingServiceUuid(null);
            editingServiceContextRef.current = null;
        } else {
            currentSmartRuleIndexRef.current = null;
            setEditingServiceUuid(serviceUuid || null);

            // In edit mode, determine if serviceUuid refers to a service in smartRouting or providers
            if (newMode === 'edit' && serviceUuid) {
                const found = findService(configRecord, serviceUuid);
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
        setEditingServiceUuid(null);
        setModelSelectionCleared(false);
        setCurrentAddTier(undefined);
        currentSmartRuleIndexRef.current = null;
        editingServiceContextRef.current = null;
        setMultiSelect(false);
        setPicked([]);
        setOpen(true);
    }, []);

    // Handle model selection
    const handleModelSelect = useCallback((option: ProviderSelectTabOption) => {
        if (mode === 'create-rule') {
            if (!multiSelect) {
                setOpen(false);
                onCreateFromModels?.([option]);
                return;
            }
            setPicked(prev => prev.some(p => p.provider.uuid === option.provider.uuid && p.model === option.model)
                ? prev.filter(p => !(p.provider.uuid === option.provider.uuid && p.model === option.model))
                : [...prev, option]);
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
        } else if (mode === 'edit' && editingServiceUuid) {
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
                                    if (service.uuid === editingServiceUuid) {
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
                        if (p.uuid === editingServiceUuid) {
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
        setEditingServiceUuid(null);
        setCurrentAddTier(undefined);
        currentSmartRuleIndexRef.current = null;
        editingServiceContextRef.current = null;
    }, [currentConfigRecord, currentAddTier, currentRuleUuid, mode, editingServiceUuid, rules, onRuleChange, showNotification, multiSelect, onCreateFromModels]);

    // Get selected provider and model for pre-selection
    const getSelectedProvider = useCallback(() => {
        if (mode === 'edit' && editingServiceUuid && currentConfigRecord) {
            const found = findService(currentConfigRecord, editingServiceUuid);
            return found?.service.provider;
        }
        return undefined;
    }, [mode, editingServiceUuid, currentConfigRecord, findService]);

    const getSelectedModel = useCallback(() => {
        // If model selection was cleared (e.g., after deleting a custom model), return undefined
        if (modelSelectionCleared) {
            return undefined;
        }
        if (mode === 'edit' && editingServiceUuid && currentConfigRecord) {
            const found = findService(currentConfigRecord, editingServiceUuid);
            return found?.service.model;
        }
        return undefined;
    }, [mode, editingServiceUuid, currentConfigRecord, findService, modelSelectionCleared]);

    // Get a unique key for ModelSelectTab to force remount when selection changes
    const dialogKey = open ? `${getSelectedProvider() || ''}-${getSelectedModel() || ''}` : 'closed';

    const isPicked = useCallback((providerUuid: string, model: string) =>
        picked.some(p => p.provider.uuid === providerUuid && p.model === model), [picked]);

    // Close dialog
    const closeModelSelect = useCallback(() => {
        setOpen(false);
        setPicked([]);
        setCurrentRuleUuid(null);
        setCurrentConfigRecord(null);
        setEditingServiceUuid(null);
        setCurrentAddTier(undefined);
        currentSmartRuleIndexRef.current = null;
        editingServiceContextRef.current = null;
    }, []);

    const submitPicked = useCallback(() => {
        const options = picked;
        closeModelSelect();
        if (options.length > 0) onCreateFromModels?.(options);
    }, [picked, closeModelSelect, onCreateFromModels]);

    // Dialog component. Kept as one stable component type: an inline React.FC
    // would be a new type on every render of the host page, remounting the
    // dialog (and emptying its model list) whenever the page re-renders.
    const WrappedModelSelectDialog = useStableComponent<ModelSelectDialogProps>(({ open: dialogOpen, onClose }) => (
        <Dialog
            open={dialogOpen}
            onClose={() => {
                closeModelSelect();
                onClose?.();
            }}
            maxWidth="lg"
            fullWidth
            slotProps={{
                paper: {
                    sx: { height: '80vh' }
                }
            }}
        >
            <DialogTitle sx={{ textAlign: 'center' }}>
                {mode === 'create-rule'
                    ? (multiSelect ? t('templateActions.multiSelectTitle') : 'Select a model for your new rule')
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
                    isModelSelected={mode === 'create-rule' && multiSelect ? isPicked : undefined}
                    // The multi-select switch sits on the models toolbar row; the
                    // footer only exists while selecting, the count beside its action.
                    toolbarEnd={mode === 'create-rule' ? (
                        <FormControlLabel
                            sx={{ ml: 0 }}
                            slotProps={{ typography: { variant: 'body2' } }}
                            control={
                                <Switch
                                    size="small"
                                    checked={multiSelect}
                                    onChange={(e) => {
                                        setMultiSelect(e.target.checked);
                                        setPicked([]);
                                    }}
                                />
                            }
                            label={t('templateActions.selectMultiple')}
                        />
                    ) : undefined}
                    onSelectionClear={() => setModelSelectionCleared(true)}
                />
            </DialogContent>
            {mode === 'create-rule' && multiSelect && (
                <DialogActions sx={{ px: 3, py: 1.5, gap: 1, borderTop: 1, borderColor: 'divider' }}>
                    {picked.length > 0 && (
                        <Button size="small" onClick={() => setPicked([])}>
                            {t('templateActions.clearSelection')}
                        </Button>
                    )}
                    <Typography variant="body2" color="text.secondary">
                        {t('templateActions.selectedModels', { count: picked.length })}
                    </Typography>
                    <Button
                        variant="contained"
                        disabled={picked.length === 0}
                        onClick={submitPicked}
                    >
                        {t('templateActions.addModels', { count: picked.length })}
                    </Button>
                </DialogActions>
            )}
        </Dialog>
    ));

    return {
        openModelSelect,
        openModelSelectForCreate,
        closeModelSelect,
        ModelSelectDialog: WrappedModelSelectDialog,
        isOpen: open,
    };
};

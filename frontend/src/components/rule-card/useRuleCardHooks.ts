import { useState, useCallback, useEffect } from 'react';
import { api } from '@/services/api';
import type { ProbeResponse } from '@/client';
import type { ConfigRecord, Rule, SmartRouting } from '@/components/RoutingGraphTypes';
import {
    ruleToConfigRecord,
    isConfigRecordReadyForSave,
    cloneSmartRouting,
    createEmptySmartRouting,
    exportRuleWithProviders,
} from './utils';

// ============================================================================
// Types
// ============================================================================

export interface RuleCardStateProps {
    collapsible?: boolean;
    initiallyExpanded?: boolean;
    onToggleExpanded?: () => void;
}

export interface UseRuleCardDataProps {
    rule: Rule;
    providers: any[];
}

export interface UseRuleAutoSaveProps {
    rule: Rule;
    onRuleChange?: (updatedRule: Rule) => void;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
}

export interface UseRuleExportProps {
    rule: Rule;
    showNotification: (message: string, severity: 'success' | 'error') => void;
}

export interface SmartRoutingHandlersProps {
    configRecord: ConfigRecord | null;
    setConfigRecord: (record: ConfigRecord | null) => void;
    autoSave: (record: ConfigRecord) => Promise<boolean>;
    ruleUuid: string;
    onModelSelectOpen: (ruleUuid: string, configRecord: ConfigRecord, mode: 'edit' | 'add', providerUuid?: string) => void;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
}

export interface SmartRoutingDialogState {
    open: boolean;
    editingRule: SmartRouting | null;
}

// ============================================================================
// Hooks
// ============================================================================

/**
 * Manages the expansion state of the rule card
 */
export function useRuleCardExpanded({
    collapsible = false,
    initiallyExpanded = !collapsible,
    onToggleExpanded,
}: RuleCardStateProps) {
    const [expanded, setExpanded] = useState(initiallyExpanded);

    useEffect(() => {
        setExpanded(initiallyExpanded);
    }, [initiallyExpanded]);

    const handleToggleExpanded = useCallback(() => {
        setExpanded((prev) => !prev);
        onToggleExpanded?.();
    }, [onToggleExpanded]);

    return { expanded, handleToggleExpanded };
}

/**
 * Manages the ConfigRecord state and keeps it in sync with the rule prop
 */
export function useRuleCardData({ rule, providers }: UseRuleCardDataProps) {
    const [configRecord, setConfigRecord] = useState<ConfigRecord | null>(null);

    useEffect(() => {
        if (rule && providers.length > 0) {
            setConfigRecord(ruleToConfigRecord(rule));
        }
    }, [rule, providers]);

    return { configRecord, setConfigRecord };
}

/**
 * Handles auto-save logic with rollback on error
 */
export function useRuleAutoSave({ rule, onRuleChange, showNotification }: UseRuleAutoSaveProps) {
    const autoSave = useCallback(
        async (newConfigRecord: ConfigRecord): Promise<boolean> => {
            if (!isConfigRecordReadyForSave(newConfigRecord)) {
                return false;
            }

            try {
                const ruleData = {
                    uuid: rule.uuid,
                    scenario: rule.scenario,
                    request_model: newConfigRecord.requestModel,
                    response_model: newConfigRecord.responseModel,
                    active: newConfigRecord.active,
                    description: newConfigRecord.description,
                    services: newConfigRecord.providers
                        .filter((p) => p.provider && p.model)
                        .map((provider) => ({
                            provider: provider.provider,
                            model: provider.model,
                            weight: provider.weight || 0,
                            active: provider.active !== undefined ? provider.active : true,
                            time_window: provider.time_window || 0,
                        })),
                    smart_enabled: newConfigRecord.smartEnabled || false,
                    smart_routing: newConfigRecord.smartRouting || [],
                };

                const result = await api.updateRule(rule.uuid, ruleData);
                if (result.success) {
                    onRuleChange?.({
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
                    showNotification('Configuration saved successfully', 'success');
                    return true;
                } else {
                    showNotification(`Failed to save: ${result.error || 'Unknown error'}`, 'error');
                    return false;
                }
            } catch (error) {
                console.error('Error saving rule:', error);
                showNotification('Error saving configuration', 'error');
                return false;
            }
        },
        [rule, onRuleChange, showNotification]
    );

    const updateField = useCallback(
        async (
            configRecord: ConfigRecord | null,
            setConfigRecord: (record: ConfigRecord | null) => void,
            field: keyof ConfigRecord,
            value: any
        ): Promise<boolean> => {
            if (!configRecord) return false;

            const previousRecord = { ...configRecord };
            const updated = { ...configRecord, [field]: value };
            setConfigRecord(updated);

            const success = await autoSave(updated);
            if (!success) {
                setConfigRecord(previousRecord);
            }
            return success;
        },
        [autoSave]
    );

    const updateRecord = useCallback(
        async (
            configRecord: ConfigRecord | null,
            setConfigRecord: (record: ConfigRecord | null) => void,
            newConfigRecord: ConfigRecord
        ): Promise<boolean> => {
            if (!configRecord) return false;

            const previousRecord = { ...configRecord };
            setConfigRecord(newConfigRecord);

            const success = await autoSave(newConfigRecord);
            if (!success) {
                setConfigRecord(previousRecord);
            }
            return success;
        },
        [autoSave]
    );

    return { autoSave, updateField, updateRecord };
}

/**
 * Manages the probe functionality for testing model connectivity
 */
export function useRuleProbe(configRecord: ConfigRecord | null) {
    const [isProbing, setIsProbing] = useState(false);
    const [probeResult, setProbeResult] = useState<ProbeResponse | null>(null);
    const [detailsExpanded, setDetailsExpanded] = useState(false);
    const [probeDialogOpen, setProbeDialogOpen] = useState(false);
    const [providerName, setProviderName] = useState<string>('');

    useEffect(() => {
        if (probeDialogOpen && configRecord?.providers[0]?.provider) {
            const fetchProviderName = async () => {
                try {
                    const providerUuid = configRecord.providers[0].provider;
                    const result = await api.getProvider(providerUuid);
                    setProviderName(result.data?.name || 'Unknown Provider');
                } catch {
                    setProviderName('Unknown Provider');
                }
            };
            fetchProviderName();
        }
    }, [probeDialogOpen, configRecord]);

    const handleProbe = useCallback(async () => {
        if (!configRecord?.providers.length || !configRecord.providers[0].provider || !configRecord.providers[0].model) {
            return;
        }

        const providerUuid = configRecord.providers[0].provider;
        const model = configRecord.providers[0].model;

        setIsProbing(true);
        setProbeResult(null);
        setProbeDialogOpen(true);

        try {
            const result = await api.probeModel(providerUuid, model);
            setProbeResult(result);
        } catch (error) {
            setProbeResult({
                success: false,
                error: {
                    message: (error as Error).message,
                    type: 'client_error',
                },
            });
        } finally {
            setIsProbing(false);
        }
    }, [configRecord]);

    const handleToggleDetails = useCallback(() => {
        setDetailsExpanded((prev) => !prev);
    }, []);

    const handleCloseDialog = useCallback(() => {
        setProbeDialogOpen(false);
    }, []);

    return {
        isProbing,
        probeResult,
        detailsExpanded,
        dialogOpen: probeDialogOpen,
        providerName,
        handleProbe,
        handleToggleDetails,
        handleCloseDialog,
    };
}

/**
 * Handles rule export functionality with providers
 */
export function useRuleExport({ rule, showNotification }: UseRuleExportProps) {
    const handleExport = useCallback(async () => {
        await exportRuleWithProviders(rule, showNotification);
    }, [rule, showNotification]);

    return { handleExport };
}

/**
 * Manages all smart routing operations (add, edit, delete, service management)
 */
export function useSmartRoutingHandlers({
    configRecord,
    setConfigRecord,
    autoSave,
    ruleUuid,
    onModelSelectOpen,
    showNotification,
}: SmartRoutingHandlersProps) {
    const [smartRuleDialogOpen, setSmartRuleDialogOpen] = useState(false);
    const [editingSmartRule, setEditingSmartRule] = useState<SmartRouting | null>(null);

    const handleAddSmartRule = useCallback(async () => {
        if (!configRecord) return;

        const newSmartRouting = createEmptySmartRouting();
        const updated: ConfigRecord = {
            ...configRecord,
            smartRouting: [...(configRecord.smartRouting || []), newSmartRouting],
        };

        const previousRecord = { ...configRecord };
        setConfigRecord(updated);

        const success = await autoSave(updated);
        if (!success) {
            setConfigRecord(previousRecord);
        }
    }, [configRecord, setConfigRecord, autoSave]);

    const handleEditSmartRule = useCallback(async (ruleUuid: string) => {
        if (!configRecord) return;

        const smartRule = (configRecord.smartRouting || []).find((r) => r.uuid === ruleUuid);
        if (smartRule) {
            setEditingSmartRule(cloneSmartRouting(smartRule));
            setSmartRuleDialogOpen(true);
        }
    }, [configRecord]);

    const handleSaveSmartRule = useCallback(async (updatedRule: SmartRouting) => {
        if (!configRecord) return;

        const updatedSmartRouting = (configRecord.smartRouting || []).map((r) =>
            r.uuid === updatedRule.uuid ? updatedRule : r
        );

        const updated: ConfigRecord = {
            ...configRecord,
            smartRouting: updatedSmartRouting,
        };

        const previousRecord = { ...configRecord };
        setConfigRecord(updated);

        const success = await autoSave(updated);
        if (!success) {
            setConfigRecord(previousRecord);
        } else {
            setSmartRuleDialogOpen(false);
            showNotification('Smart rule updated successfully', 'success');
        }
    }, [configRecord, setConfigRecord, autoSave, showNotification]);

    const handleCancelSmartRuleEdit = useCallback(() => {
        setSmartRuleDialogOpen(false);
        setEditingSmartRule(null);
    }, []);

    const handleDeleteSmartRule = useCallback(async (ruleUuid: string) => {
        if (!configRecord) return;

        const updated: ConfigRecord = {
            ...configRecord,
            smartRouting: (configRecord.smartRouting || []).filter((r) => r.uuid !== ruleUuid),
        };

        const previousRecord = { ...configRecord };
        setConfigRecord(updated);

        const success = await autoSave(updated);
        if (!success) {
            setConfigRecord(previousRecord);
        } else {
            showNotification('Smart rule deleted successfully', 'success');
        }
    }, [configRecord, setConfigRecord, autoSave, showNotification]);

    const handleAddServiceToSmartRule = useCallback((smartRuleIndex: number) => {
        if (!configRecord) return;
        onModelSelectOpen(ruleUuid, configRecord, 'add', `smart:${smartRuleIndex}`);
    }, [configRecord, ruleUuid, onModelSelectOpen]);

    const handleDeleteServiceFromSmartRule = useCallback(async (ruleUuid: string, serviceUuid: string) => {
        if (!configRecord) return;

        const updatedSmartRouting = (configRecord.smartRouting || []).map((rule) => {
            if (rule.uuid === ruleUuid && rule.services) {
                return {
                    ...rule,
                    services: rule.services.filter((s) => s.uuid !== serviceUuid),
                };
            }
            return rule;
        });

        const updated: ConfigRecord = {
            ...configRecord,
            smartRouting: updatedSmartRouting,
        };

        const previousRecord = { ...configRecord };
        setConfigRecord(updated);

        const success = await autoSave(updated);
        if (!success) {
            setConfigRecord(previousRecord);
        } else {
            showNotification('Service deleted successfully', 'success');
        }
    }, [configRecord, setConfigRecord, autoSave, showNotification]);

    const handleDeleteDefaultProvider = useCallback(async (providerUuid: string) => {
        if (!configRecord) return;

        const updated: ConfigRecord = {
            ...configRecord,
            providers: configRecord.providers.filter((p) => p.uuid !== providerUuid),
        };

        const previousRecord = { ...configRecord };
        setConfigRecord(updated);

        const success = await autoSave(updated);
        if (!success) {
            setConfigRecord(previousRecord);
        } else {
            showNotification('Provider deleted successfully', 'success');
        }
    }, [configRecord, setConfigRecord, autoSave, showNotification]);

    return {
        dialogState: {
            open: smartRuleDialogOpen,
            editingRule: editingSmartRule,
        },
        handlers: {
            handleAddSmartRule,
            handleEditSmartRule,
            handleSaveSmartRule,
            handleCancelSmartRuleEdit,
            handleDeleteSmartRule,
            handleAddServiceToSmartRule,
            handleDeleteServiceFromSmartRule,
            handleDeleteDefaultProvider,
        },
    };
}

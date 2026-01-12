import { Delete as DeleteIcon, PlayArrow as ProbeIcon } from '@mui/icons-material';
import { Box, Button, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle, Tooltip, Typography } from '@mui/material';
import React, { useCallback, useState } from 'react';
import type { ProbeResponse } from '../client';
import Probe from './ProbeModal.tsx';
import RoutingGraph from './RoutingGraph';
import SmartRoutingGraph from './SmartRoutingGraph';
import { api } from '../services/api';
import type { Provider, ProviderModelsDataByUuid } from '../types/provider';
import type { ConfigRecord, Rule } from './RoutingGraphTypes.ts';
import { v4 as uuidv4 } from 'uuid';

export interface RuleCardProps {
    rule: Rule;
    providers: Provider[];
    providerModelsByUuid: ProviderModelsDataByUuid;
    providerUuidToName: { [uuid: string]: string };
    saving: boolean;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
    onRuleChange?: (updatedRule: Rule) => void;
    onProviderModelsChange?: (providerUuid: string, models: any) => void;
    onRefreshProvider?: (providerUuid: string) => void;
    onModelSelectOpen: (ruleUuid: string, configRecord: ConfigRecord, mode: 'edit' | 'add', providerUuid?: string) => void;
    collapsible?: boolean;
    initiallyExpanded?: boolean;
    allowDeleteRule?: boolean;
    onRuleDelete?: (ruleUuid: string) => void;
    allowToggleRule?: boolean;
}

export const RuleCard: React.FC<RuleCardProps> = ({
    rule,
    providers,
    providerModelsByUuid,
    providerUuidToName,
    saving,
    showNotification,
    onRuleChange,
    onProviderModelsChange,
    onRefreshProvider,
    onModelSelectOpen,
    collapsible = false,
    initiallyExpanded = !collapsible,
    allowDeleteRule = false,
    onRuleDelete,
    allowToggleRule = true,
}) => {
    const [configRecord, setConfigRecord] = useState<ConfigRecord | null>(null);
    const [expanded, setExpanded] = useState(initiallyExpanded);

    // Probe state
    const [isProbing, setIsProbing] = useState(false);
    const [probeResult, setProbeResult] = useState<ProbeResponse | null>(null);
    const [detailsExpanded, setDetailsExpanded] = useState(false);
    const [probeDialogOpen, setProbeDialogOpen] = useState(false);

    // Delete confirmation state
    const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

    // Convert rule to ConfigRecord format
    React.useEffect(() => {
        if (rule && providers.length > 0) {
            const services = rule.services || [];
            const providersList = services.map((service: any) => ({
                uuid: service.id || service.uuid || uuidv4(),
                provider: service.provider || '',
                model: service.model || '',
                isManualInput: false,
                weight: service.weight || 0,
                active: service.active !== undefined ? service.active : true,
                time_window: service.time_window || 0,
            }));

            if (providersList.length === 0) {
                providersList.push({
                    uuid: uuidv4(),
                    provider: '',
                    model: '',
                });
            }

            const newConfigRecord: ConfigRecord = {
                uuid: rule.uuid || uuidv4(),
                requestModel: rule.request_model || '',
                responseModel: rule.response_model || '',
                active: rule.active !== undefined ? rule.active : true,
                providers: providersList,
                description: rule.description,
                smartEnabled: rule.smart_enabled || false,
                smartRouting: rule.smart_routing || [],
            };

            setConfigRecord(newConfigRecord);
        }
    }, [rule, providers]);

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
            console.error('Probe error:', error);
            setProbeResult({
                success: false,
                error: {
                    message: (error as Error).message,
                    type: 'client_error'
                }
            });
        } finally {
            setIsProbing(false);
        }
    }, [configRecord]);

    const handleRefreshModels = useCallback(async (providerUuid: string) => {
        if (!providerUuid) return;

        if (onRefreshProvider) {
            onRefreshProvider(providerUuid);
            return;
        }

        try {
            const result = await api.updateProviderModelsByUUID(providerUuid);
            if (result.success && result.data && onProviderModelsChange) {
                onProviderModelsChange(providerUuid, result.data);
                showNotification(`Models refreshed successfully!`, 'success');
            } else {
                showNotification(`Failed to refresh models`, 'error');
            }
        } catch (error) {
            console.error('Error refreshing models:', error);
            showNotification(`Error refreshing models`, 'error');
        }
    }, [onRefreshProvider, onProviderModelsChange, showNotification]);

    const handleFetchModels = useCallback(async (providerUuid: string) => {
        if (!providerUuid || providerModelsByUuid[providerUuid]) return;

        try {
            const result = await api.getProviderModelsByUUID(providerUuid);
            if (result.success && result.data && onProviderModelsChange) {
                onProviderModelsChange(providerUuid, result.data);
            }
        } catch (error) {
            console.error(`Failed to fetch models for provider ${providerUuid}:`, error);
        }
    }, [providerModelsByUuid, onProviderModelsChange]);

    const autoSave = useCallback(async (newConfigRecord: ConfigRecord) => {
        if (!newConfigRecord.requestModel) return false;

        for (const provider of newConfigRecord.providers) {
            if (provider.provider && !provider.model) {
                return false;
            }
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
                    .filter(p => p.provider && p.model)
                    .map(provider => ({
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
            console.log("update rule: ", result)
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
                return true;
            } else {
                showNotification(`Failed to save: ${result.error || 'Unknown error'}`, 'error');
                return false;
            }
        } catch (error) {
            console.error('Error saving rule:', error);
            showNotification(`Error saving configuration`, 'error');
            return false;
        }
    }, [rule, onRuleChange, showNotification]);

    const handleUpdateRecord = useCallback(async (field: keyof ConfigRecord, value: any) => {
        if (configRecord) {
            const previousRecord = { ...configRecord };
            const updated = { ...configRecord, [field]: value };
            setConfigRecord(updated);

            const success = await autoSave(updated);
            if (!success) {
                // Rollback on error
                setConfigRecord(previousRecord);
            }
        }
    }, [configRecord, autoSave]);

    const handleDeleteProvider = useCallback(async (_recordId: string, providerId: string) => {
        if (configRecord) {
            const previousRecord = { ...configRecord };
            const updated = {
                ...configRecord,
                providers: configRecord.providers.filter(p => p.uuid !== providerId),
            };
            setConfigRecord(updated);

            const success = await autoSave(updated);
            if (!success) {
                // Rollback on error
                setConfigRecord(previousRecord);
            }
        }
    }, [configRecord, autoSave]);

    const handleProviderNodeClick = useCallback((providerUuid: string) => {
        if (configRecord) {
            onModelSelectOpen(rule.uuid, configRecord, 'edit', providerUuid);
        }
    }, [configRecord, rule.uuid, onModelSelectOpen]);

    const handleAddProviderButtonClick = useCallback(() => {
        if (configRecord) {
            onModelSelectOpen(rule.uuid, configRecord, 'add');
        }
    }, [configRecord, rule.uuid, onModelSelectOpen]);

    // Smart routing handlers
    const handleAddSmartRule = useCallback(async () => {
        if (!configRecord) return;

        const newSmartRouting = {
            uuid: crypto.randomUUID(),
            description: 'New Smart Rule',
            ops: [],
            services: [],
        };

        const updated = {
            ...configRecord,
            smartRouting: [...(configRecord.smartRouting || []), newSmartRouting],
        };

        const previousRecord = { ...configRecord };
        setConfigRecord(updated);

        const success = await autoSave(updated);
        if (!success) {
            setConfigRecord(previousRecord);
        }
    }, [configRecord, autoSave]);

    const handleEditSmartRule = useCallback(async (ruleUuid: string) => {
        // TODO: Open smart rule edit dialog
        showNotification('Smart rule editing not yet implemented', 'info');
    }, [showNotification]);

    const handleDeleteSmartRule = useCallback(async (ruleUuid: string) => {
        if (!configRecord) return;

        const updated = {
            ...configRecord,
            smartRouting: (configRecord.smartRouting || []).filter(r => r.uuid !== ruleUuid),
        };

        const previousRecord = { ...configRecord };
        setConfigRecord(updated);

        const success = await autoSave(updated);
        if (!success) {
            setConfigRecord(previousRecord);
        } else {
            showNotification('Smart rule deleted successfully', 'success');
        }
    }, [configRecord, autoSave, showNotification]);

    const handleAddServiceToSmartRule = useCallback(async (ruleUuid: string) => {
        // TODO: Open provider/service selection dialog
        showNotification('Add service to smart rule not yet implemented', 'info');
    }, [showNotification]);

    const handleDeleteButtonClick = useCallback(() => {
        setDeleteDialogOpen(true);
    }, []);

    const confirmDeleteRule = useCallback(async () => {
        if (!onRuleDelete || !rule.uuid) {
            setDeleteDialogOpen(false);
            return;
        }

        try {
            const result = await api.deleteRule(rule.uuid);
            if (result.success) {
                showNotification('Rule deleted successfully!', 'success');
                onRuleDelete(rule.uuid);
            } else {
                showNotification(`Failed to delete rule: ${result.error || 'Unknown error'}`, 'error');
            }
        } catch (error) {
            console.error('Error deleting rule:', error);
            showNotification('Failed to delete routing rule', 'error');
        } finally {
            setDeleteDialogOpen(false);
        }
    }, [rule.uuid, onRuleDelete, showNotification]);

    if (!configRecord) return null;

    const isSmartMode = rule.smart_enabled;

    return (
        <>
            {isSmartMode ? (
                    <SmartRoutingGraph
                        record={configRecord}
                        providers={providers}
                        providerUuidToName={providerUuidToName}
                        active={configRecord.active}
                        onToggleSmartEnabled={(enabled) => handleUpdateRecord('smartEnabled', enabled)}
                        onAddSmartRule={handleAddSmartRule}
                        onEditSmartRule={handleEditSmartRule}
                        onDeleteSmartRule={handleDeleteSmartRule}
                        onAddServiceToSmartRule={handleAddServiceToSmartRule}
                    />
                ) : (
                    <RoutingGraph
                        record={configRecord}
                        recordUuid={configRecord.uuid}
                        providers={providers}
                        providerUuidToName={providerUuidToName}
                        saving={saving}
                        expanded={expanded}
                        collapsible={collapsible}
                        allowToggleRule={allowToggleRule}
                        onUpdateRecord={handleUpdateRecord}
                        onDeleteProvider={handleDeleteProvider}
                        onRefreshModels={handleRefreshModels}
                        onToggleExpanded={() => setExpanded(!expanded)}
                        onProviderNodeClick={handleProviderNodeClick}
                        onAddProviderButtonClick={handleAddProviderButtonClick}
                        extraActions={
                        <Box sx={{ display: 'flex', gap: 1 }}>
                            <Tooltip title="Test connection to provider">
                                <Button
                                    size="small"
                                    onClick={handleProbe}
                                    disabled={!configRecord.providers[0]?.provider || !configRecord.providers[0]?.model || isProbing}
                                    startIcon={<ProbeIcon fontSize="small" />}
                                    sx={{
                                        minWidth: 'auto',
                                        color: isProbing ? 'primary.main' : 'text.secondary',
                                        '&:hover': {
                                            backgroundColor: 'primary.main',
                                            color: 'primary.contrastText',
                                        },
                                        '&:disabled': {
                                            color: 'text.disabled',
                                        }
                                    }}
                                >
                                    Test
                                </Button>
                            </Tooltip>
                            {allowDeleteRule && (
                                <Tooltip title="Delete routing rule">
                                    <Button
                                        size="small"
                                        onClick={handleDeleteButtonClick}
                                        startIcon={<DeleteIcon fontSize="small" />}
                                        sx={{
                                            minWidth: 'auto',
                                            color: 'error.main',
                                            '&:hover': {
                                                backgroundColor: 'error.main',
                                                color: 'error.contrastText',
                                            },
                                        }}
                                    >
                                        Delete
                                    </Button>
                                </Tooltip>
                            )}
                        </Box>
                    }
                />
            )}

            {/* Probe Result Dialog */}
            <Dialog
                open={probeDialogOpen}
                onClose={() => setProbeDialogOpen(false)}
                maxWidth="md"
                fullWidth
                PaperProps={{
                    sx: { height: 'auto', maxHeight: '80vh' }
                }}
            >
                <DialogTitle sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                    <Typography variant="h6">Connection Test Result</Typography>
                    <Typography variant="body2" color="text.secondary">
                        {providerUuidToName[configRecord?.providers[0]?.provider || '']} / {configRecord?.providers[0]?.model}
                    </Typography>
                </DialogTitle>
                <DialogContent>
                    <Probe
                        provider={configRecord?.providers[0]?.provider}
                        model={configRecord?.providers[0]?.model}
                        isProbing={isProbing}
                        probeResult={probeResult}
                        onToggleDetails={() => setDetailsExpanded(!detailsExpanded)}
                        detailsExpanded={detailsExpanded}
                    />
                </DialogContent>
            </Dialog>

            {/* Delete Confirmation Dialog */}
            <Dialog
                open={deleteDialogOpen}
                onClose={() => setDeleteDialogOpen(false)}
                maxWidth="sm"
                fullWidth
            >
                <DialogTitle>Delete Routing Rule</DialogTitle>
                <DialogContent>
                    <DialogContentText>
                        Are you sure you want to delete this routing rule? This action cannot be undone.
                    </DialogContentText>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setDeleteDialogOpen(false)} color="primary">
                        Cancel
                    </Button>
                    <Button onClick={confirmDeleteRule} color="error" variant="contained">
                        Delete
                    </Button>
                </DialogActions>
            </Dialog>
        </>
    );
};

export default RuleCard;

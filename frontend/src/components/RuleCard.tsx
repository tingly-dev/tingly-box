import { PlayArrow as ProbeIcon } from '@mui/icons-material';
import { Button, Dialog, DialogContent, DialogTitle, Tooltip, Typography } from '@mui/material';
import React, { useCallback, useState } from 'react';
import type { ProbeResponse } from '../client';
import Probe from '../components/Probe';
import RuleGraphV2 from '../components/RuleGraphV2';
import { api } from '../services/api';
import type { Provider, ProviderModelsDataByUuid } from '../types/provider';
import type { ConfigRecord, Rule } from './RuleGraphTypes';

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
}) => {
    const [configRecord, setConfigRecord] = useState<ConfigRecord | null>(null);
    const [expanded, setExpanded] = useState(initiallyExpanded);

    // Probe state
    const [isProbing, setIsProbing] = useState(false);
    const [probeResult, setProbeResult] = useState<ProbeResponse | null>(null);
    const [detailsExpanded, setDetailsExpanded] = useState(false);
    const [probeDialogOpen, setProbeDialogOpen] = useState(false);

    // Convert rule to ConfigRecord format
    React.useEffect(() => {
        if (rule && providers.length > 0) {
            const services = rule.services || [];
            const providersList = services.map((service: any) => ({
                uuid: service.id || service.uuid || crypto.randomUUID(),
                provider: service.provider || '',
                model: service.model || '',
                isManualInput: false,
                weight: service.weight || 0,
                active: service.active !== undefined ? service.active : true,
                time_window: service.time_window || 0,
            }));

            if (providersList.length === 0) {
                providersList.push({
                    uuid: crypto.randomUUID(),
                    provider: '',
                    model: '',
                });
            }

            const newConfigRecord: ConfigRecord = {
                uuid: rule.uuid || crypto.randomUUID(),
                requestModel: rule.request_model || '',
                responseModel: rule.response_model || '',
                active: rule.active !== undefined ? rule.active : true,
                providers: providersList,
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
                request_model: newConfigRecord.requestModel,
                response_model: newConfigRecord.responseModel,
                active: newConfigRecord.active,
                services: newConfigRecord.providers
                    .filter(p => p.provider && p.model)
                    .map(provider => ({
                        provider: provider.provider,
                        model: provider.model,
                        weight: provider.weight || 0,
                        active: provider.active !== undefined ? provider.active : true,
                        time_window: provider.time_window || 0,
                    })),
            };

            const result = await api.updateRule(rule.uuid, ruleData);
            console.log("update rule: ", result)
            if (result.success) {
                onRuleChange?.({
                    ...rule,
                    request_model: ruleData.request_model,
                    response_model: ruleData.response_model,
                    active: ruleData.active,
                    services: ruleData.services,
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

    if (!configRecord) return null;

    return (
        <>
            <RuleGraphV2
                record={configRecord}
                recordUuid={configRecord.uuid}
                providers={providers}
                providerUuidToName={providerUuidToName}
                saving={saving}
                expanded={expanded}
                collapsible={collapsible}
                onUpdateRecord={handleUpdateRecord}
                onDeleteProvider={handleDeleteProvider}
                onRefreshModels={handleRefreshModels}
                onToggleExpanded={() => setExpanded(!expanded)}
                onProviderNodeClick={handleProviderNodeClick}
                onAddProviderButtonClick={handleAddProviderButtonClick}
                extraActions={
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
                }
            />

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
        </>
    );
};

export default RuleCard;

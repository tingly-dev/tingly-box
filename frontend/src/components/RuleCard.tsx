import { Delete as DeleteIcon, Download as ExportIcon, PlayArrow as ProbeIcon, Settings as SettingsIcon } from '@mui/icons-material';
import { Button, Dialog, DialogActions, DialogContent, DialogContentText, DialogTitle, IconButton, Menu, MenuItem, Tooltip, Typography } from '@mui/material';
import React, { useCallback, useState } from 'react';
import type { ProbeResponse } from '../client';
import Probe from './ProbeModal.tsx';
import RoutingGraph from './RoutingGraph';
import SmartRoutingGraph from './SmartRoutingGraph';
import SmartRuleEditDialog from './SmartRuleEditDialog';
import { api } from '../services/api';
import type { Provider, ProviderModelsDataByUuid } from '../types/provider';
import type { ConfigRecord, Rule, SmartRouting } from './RoutingGraphTypes.ts';
import { v4 as uuidv4 } from 'uuid';

export interface RuleCardProps {
    rule: Rule;
    providers: Provider[];
    providerModelsByUuid: ProviderModelsDataByUuid;
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
    const [providerName, setProviderName] = useState<string>('');

    // Delete confirmation state
    const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

    // Menu state
    const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
    const menuOpen = Boolean(menuAnchorEl);

    // Smart rule edit dialog state
    const [smartRuleDialogOpen, setSmartRuleDialogOpen] = useState(false);
    const [editingSmartRule, setEditingSmartRule] = useState<SmartRouting | null>(null);

    const handleMenuOpen = useCallback((event: React.MouseEvent<HTMLElement>) => {
        setMenuAnchorEl(event.currentTarget);
    }, []);

    const handleMenuClose = useCallback(() => {
        setMenuAnchorEl(null);
    }, []);

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

            // Ensure smartRouting services have uuid
            const smartRouting = (rule.smart_routing || []).map((routing: any) => ({
                ...routing,
                services: (routing.services || []).map((service: any) => ({
                    ...service,
                    uuid: service.id || service.uuid || uuidv4(),
                })),
            }));

            const newConfigRecord: ConfigRecord = {
                uuid: rule.uuid || uuidv4(),
                requestModel: rule.request_model || '',
                responseModel: rule.response_model || '',
                active: rule.active !== undefined ? rule.active : true,
                providers: providersList,
                description: rule.description,
                smartEnabled: rule.smart_enabled || false,
                smartRouting: smartRouting,
            };

            setConfigRecord(newConfigRecord);
        }
    }, [rule, providers]);

    // Fetch provider name when probe dialog opens
    React.useEffect(() => {
        if (probeDialogOpen && configRecord?.providers[0]?.provider) {
            const fetchProviderName = async () => {
                try {
                    const providerUuid = configRecord.providers[0].provider;
                    const result = await api.getProvider(providerUuid);
                    if (result.success && result.data) {
                        setProviderName(result.data.name || 'Unknown Provider');
                    } else {
                        setProviderName('Unknown Provider');
                    }
                } catch (error) {
                    console.error('Failed to fetch provider name:', error);
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
        if (!configRecord) return;

        console.log('Editing smart rule with UUID:', ruleUuid);
        const smartRule = (configRecord.smartRouting || []).find(r => r.uuid === ruleUuid);
        if (smartRule) {
            console.log('Found rule:', smartRule.uuid, smartRule.description);
            // Create a deep copy to avoid mutating the original object
            const smartRuleCopy: SmartRouting = JSON.parse(JSON.stringify(smartRule));
            setEditingSmartRule(smartRuleCopy);
            setSmartRuleDialogOpen(true);
        } else {
            console.error('Rule not found with UUID:', ruleUuid);
        }
    }, [configRecord]);

    const handleSaveSmartRule = useCallback(async (updatedRule: SmartRouting) => {
        if (!configRecord) return;

        console.log('Saving smart rule:', updatedRule.uuid, updatedRule.description);
        console.log('Existing rules:', configRecord.smartRouting?.map(r => ({ uuid: r.uuid, desc: r.description })));

        const updatedSmartRouting = (configRecord.smartRouting || []).map(r => {
            const shouldUpdate = r.uuid === updatedRule.uuid;
            if (shouldUpdate) {
                console.log('Updating rule:', r.uuid, '->', updatedRule.description);
            }
            return shouldUpdate ? updatedRule : r;
        });

        const updated = {
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
    }, [configRecord, autoSave, showNotification]);

    const handleCancelSmartRuleEdit = useCallback(() => {
        setSmartRuleDialogOpen(false);
        setEditingSmartRule(null);
    }, []);

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

    const handleAddServiceToSmartRule = useCallback(async (smartRuleIndex: number) => {
        if (!configRecord) return;

        // Open the model selection dialog with the smart rule index
        // We use a special format: "smart:${index}" to indicate this is for a smart rule
        const smartRuleRef = `smart:${smartRuleIndex}`;
        onModelSelectOpen(rule.uuid, configRecord, 'add', smartRuleRef);
    }, [configRecord, rule.uuid, onModelSelectOpen]);

    const handleDeleteServiceFromSmartRule = useCallback(async (ruleUuid: string, serviceUuid: string) => {
        console.log('handleDeleteServiceFromSmartRule called:', { ruleUuid, serviceUuid });
        if (!configRecord) {
            console.log('No configRecord, returning');
            return;
        }

        console.log('Current smartRouting:', configRecord.smartRouting);
        const updatedSmartRouting = (configRecord.smartRouting || []).map(rule => {
            if (rule.uuid === ruleUuid && rule.services) {
                console.log('Found rule, filtering services:', rule.services, 'serviceUuid:', serviceUuid);
                return {
                    ...rule,
                    services: rule.services.filter(s => s.uuid !== serviceUuid),
                };
            }
            return rule;
        });
        console.log('Updated smartRouting:', updatedSmartRouting);

        const updated = {
            ...configRecord,
            smartRouting: updatedSmartRouting,
        };

        const previousRecord = { ...configRecord };
        setConfigRecord(updated);

        const success = await autoSave(updated);
        console.log('autoSave result:', success);
        if (!success) {
            setConfigRecord(previousRecord);
        } else {
            showNotification('Service deleted successfully', 'success');
        }
    }, [configRecord, autoSave, showNotification]);

    const handleDeleteDefaultProvider = useCallback(async (providerUuid: string) => {
        if (!configRecord) return;

        const updated = {
            ...configRecord,
            providers: configRecord.providers.filter(p => p.uuid !== providerUuid),
        };

        const previousRecord = { ...configRecord };
        setConfigRecord(updated);

        const success = await autoSave(updated);
        if (!success) {
            setConfigRecord(previousRecord);
        } else {
            showNotification('Provider deleted successfully', 'success');
        }
    }, [configRecord, autoSave, showNotification]);

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

    const handleExport = useCallback(async () => {
        try {
            // Collect unique provider UUIDs from services
            const providerUuids = new Set<string>();
            (rule.services || []).forEach((service: any) => {
                if (service.provider) {
                    providerUuids.add(service.provider);
                }
            });

            // Fetch all providers
            const providersData: any[] = [];
            for (const uuid of providerUuids) {
                try {
                    const result = await api.getProvider(uuid);
                    if (result.success && result.data) {
                        providersData.push(result.data);
                    }
                } catch (error) {
                    console.error(`Failed to fetch provider ${uuid}:`, error);
                }
            }

            // Build JSONL export
            const lines: string[] = [];

            // Line 1: Metadata
            const metadata = {
                type: 'metadata',
                version: '1.0',
                exported_at: new Date().toISOString(),
            };
            lines.push(JSON.stringify(metadata));

            // Line 2: Rule
            const ruleExport = {
                type: 'rule',
                uuid: rule.uuid,
                scenario: rule.scenario,
                request_model: rule.request_model,
                response_model: rule.response_model,
                description: rule.description,
                services: rule.services || [],
                lb_tactic: rule.lb_tactic,
                active: rule.active,
                smart_enabled: rule.smart_enabled,
                smart_routing: rule.smart_routing || [],
            };
            lines.push(JSON.stringify(ruleExport));

            // Subsequent lines: Providers
            for (const provider of providersData) {
                const providerExport = {
                    type: 'provider',
                    uuid: provider.uuid,
                    name: provider.name,
                    api_base: provider.api_base,
                    api_style: provider.api_style,
                    auth_type: provider.auth_type || 'api_key',
                    token: provider.token,
                    oauth_detail: provider.oauth_detail,
                    enabled: provider.enabled,
                    proxy_url: provider.proxy_url,
                    timeout: provider.timeout,
                    tags: provider.tags,
                    models: provider.models,
                };
                lines.push(JSON.stringify(providerExport));
            }

            // Create download
            const blob = new Blob([lines.join('\n')], { type: 'application/jsonl' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `${rule.request_model || 'rule'}-${rule.scenario}.jsonl`;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);

            showNotification('Rule with API keys exported successfully!', 'success');
        } catch (error) {
            console.error('Error exporting rule:', error);
            showNotification('Failed to export rule', 'error');
        }
    }, [rule, showNotification]);

    if (!configRecord) return null;

    const isSmartMode = rule.smart_enabled;

    // Extra actions menu - shared between RoutingGraph and SmartRoutingGraph
    const extraActions = (
        <>
            <Tooltip title="Rule actions">
                <IconButton
                    size="small"
                    onClick={handleMenuOpen}
                    sx={{
                        color: 'text.secondary',
                        '&:hover': {
                            backgroundColor: 'action.hover',
                        },
                    }}
                >
                    <SettingsIcon fontSize="small" />
                </IconButton>
            </Tooltip>
            <Menu
                anchorEl={menuAnchorEl}
                open={menuOpen}
                onClose={handleMenuClose}
                anchorOrigin={{
                    vertical: 'bottom',
                    horizontal: 'right',
                }}
                transformOrigin={{
                    vertical: 'top',
                    horizontal: 'right',
                }}
            >
                <MenuItem
                    onClick={() => {
                        handleMenuClose();
                        handleProbe();
                    }}
                    disabled={!configRecord.providers[0]?.provider || !configRecord.providers[0]?.model || isProbing}
                >
                    <ProbeIcon fontSize="small" sx={{ mr: 1 }} />
                    Test Connection
                </MenuItem>
                <MenuItem
                    onClick={() => {
                        handleMenuClose();
                        handleExport();
                    }}
                >
                    <ExportIcon fontSize="small" sx={{ mr: 1 }} />
                    Export with API Keys
                </MenuItem>
                {allowDeleteRule && (
                    <MenuItem
                        onClick={() => {
                            handleMenuClose();
                            handleDeleteButtonClick();
                        }}
                        sx={{ color: 'error.main' }}
                    >
                        <DeleteIcon fontSize="small" sx={{ mr: 1 }} />
                        Delete Rule
                    </MenuItem>
                )}
            </Menu>
        </>
    );

    return (
        <>
            {isSmartMode ? (
                    <SmartRoutingGraph
                        record={configRecord}
                        providers={providers}
                        active={configRecord.active}
                        saving={saving}
                        collapsible={collapsible}
                        allowToggleRule={allowToggleRule}
                        expanded={expanded}
                        onToggleExpanded={() => setExpanded(!expanded)}
                        extraActions={extraActions}
                        onUpdateRecord={handleUpdateRecord}
                        onToggleSmartEnabled={(enabled) => handleUpdateRecord('smartEnabled', enabled)}
                        onAddSmartRule={handleAddSmartRule}
                        onEditSmartRule={handleEditSmartRule}
                        onDeleteSmartRule={handleDeleteSmartRule}
                        onAddServiceToSmartRule={handleAddServiceToSmartRule}
                        onDeleteServiceFromSmartRule={handleDeleteServiceFromSmartRule}
                        onAddDefaultProvider={handleAddProviderButtonClick}
                        onDeleteDefaultProvider={handleDeleteDefaultProvider}
                    />
                ) : (
                    <RoutingGraph
                        record={configRecord}
                        recordUuid={configRecord.uuid}
                        providers={providers}
                        saving={saving}
                        expanded={expanded}
                        collapsible={collapsible}
                        allowToggleRule={allowToggleRule}
                        onUpdateRecord={handleUpdateRecord}
                        onDeleteProvider={handleDeleteProvider}
                        onToggleExpanded={() => setExpanded(!expanded)}
                        onProviderNodeClick={handleProviderNodeClick}
                        onAddProviderButtonClick={handleAddProviderButtonClick}
                        extraActions={extraActions}
                        onAddSmartRule={handleAddSmartRule}
                        onEditSmartRule={handleEditSmartRule}
                        onDeleteSmartRule={handleDeleteSmartRule}
                        onAddServiceToSmartRule={handleAddServiceToSmartRule}
                        onDeleteServiceFromSmartRule={handleDeleteServiceFromSmartRule}
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
                        {providerName} / {configRecord?.providers[0]?.model || ''}
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

            {/* Smart Rule Edit Dialog */}
            <SmartRuleEditDialog
                open={smartRuleDialogOpen}
                smartRouting={editingSmartRule}
                onSave={handleSaveSmartRule}
                onCancel={handleCancelSmartRuleEdit}
            />
        </>
    );
};

export default RuleCard;

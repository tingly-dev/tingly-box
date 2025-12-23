import {
    Add as AddIcon
} from '@mui/icons-material';
import {
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogContentText,
    DialogTitle,
    Stack,
    Typography
} from '@mui/material';
import React, { useCallback, useEffect, useState, useMemo } from 'react';
import { useSearchParams } from 'react-router-dom';
import { PageLayout } from '../components/PageLayout';
import RuleGraph from '../components/RuleGraph'
import UnifiedCard from '../components/UnifiedCard';
import { api } from '../services/api';

const RuleCard = RuleGraph

interface ConfigProvider {
    uuid: string;
    provider: string;
    model: string;
    isManualInput?: boolean;
    weight?: number;
    active?: boolean;
    time_window?: number;
}

interface ConfigRecord {
    uuid: string;
    requestModel: string;
    responseModel: string;
    active: boolean;
    providers: ConfigProvider[];
}

const RulePage = () => {
    const [searchParams] = useSearchParams();
    const [rules, setRules] = useState<any>({});
    const [providers, setProviders] = useState<any[]>([]);
    const [providerModels, setProviderModels] = useState<any>({});
    const [configRecords, setConfigRecords] = useState<ConfigRecord[]>([]);
    const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [loading, setLoading] = useState(true);
    const [savingRecords, setSavingRecords] = useState<Set<string>>(new Set());
    const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
    const [recordToDelete, setRecordToDelete] = useState<string | null>(null);
    const [expandedCards, setExpandedCards] = useState<Set<string>>(new Set());

    // Create lookup maps for provider UUID to name and name to UUID
    const providerUuidToName = useMemo(() => {
        const map: { [uuid: string]: string } = {};
        providers.forEach(provider => {
            map[provider.uuid] = provider.name;
        });
        return map;
    }, [providers]);

    const providerNameToUuid = useMemo(() => {
        const map: { [name: string]: string } = {};
        providers.forEach(provider => {
            map[provider.name] = provider.uuid;
        });
        return map;
    }, [providers]);

    const loadData = useCallback(async () => {
        setLoading(true);
        try {
            const [providersResult, rulesResult] = await Promise.all([
                api.getProviders(),
                api.getRules(),
            ]);

            if (providersResult.success) {
                setProviders(providersResult.data);
            }
            if (rulesResult.success) {
                setRules(rulesResult.data);
            }
        } catch (error) {
            setMessage({ type: 'error', text: 'Failed to load data' });
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        loadData();
    }, [loadData]);

    useEffect(() => {
        if (Array.isArray(rules)) {
            const records: ConfigRecord[] = rules.map((rule: any) => {
                // Use the rule's UUID as-is, don't generate new ones for existing rules
                const ruleUuid = rule.uuid || '';
                return {
                    uuid: ruleUuid,
                    requestModel: rule.request_model || '',
                    responseModel: rule.response_model || '',
                    active: rule.active !== undefined ? rule.active : true,
                    providers: (rule.services || []).map((service: any) => ({
                        // Use service identifier if available, otherwise generate one
                        uuid: service.id || service.uuid || crypto.randomUUID(),
                        provider: service.provider || '', // This is now UUID, not name
                        model: service.model || '',
                        isManualInput: false,
                        weight: service.weight || 0,
                        active: service.active !== undefined ? service.active : true,
                        time_window: service.time_window || 0,
                    })),
                };
            });
            setConfigRecords(records);

            // Check for expand parameter in URL and auto-expand cards
            const expandParam = searchParams.get('expand');
            if (expandParam) {
                const uuidsToExpand = expandParam.split(',').map(uuid => uuid.trim()).filter(uuid => uuid);
                const validUuids = uuidsToExpand.filter(uuid => records.some(record => record.uuid === uuid));
                if (validUuids.length > 0) {
                    setExpandedCards(new Set(validUuids));
                }
            }
        } else {
            setConfigRecords([]);
        }
    }, [rules, searchParams]);

    const handleSaveRule = async (record: ConfigRecord) => {
        console.log(record)
        if (!record.requestModel || !record.uuid) {
            setMessage({ type: 'error', text: 'Request model name is required' });
            return;
        }

        for (const provider of record.providers) {
            if (provider.provider && !provider.model) {
                const providerName = providerUuidToName[provider.provider] || provider.provider;
                setMessage({ type: 'error', text: `Please select a model for provider ${providerName}` });
                return;
            }
        }

        setSavingRecords(prev => new Set(prev).add(record.uuid));

        try {
            const ruleData = {
                uuid: record.uuid,
                request_model: record.requestModel,
                response_model: record.responseModel,
                active: record.active,
                services: record.providers.map(provider => ({
                    provider: provider.provider, // This is now UUID
                    model: provider.model,
                    weight: provider.weight || 0,
                    active: provider.active !== undefined ? provider.active : true,
                    time_window: provider.time_window || 0,
                })),
            };

            const result = await api.updateRule(record.uuid, ruleData);

            if (result.success) {
                setMessage({ type: 'success', text: `Rule "${record.requestModel}" saved successfully` });
                await loadData();
            } else {
                setMessage({ type: 'error', text: `Failed to save rule: ${result.error || 'Unknown error'}` });
                setTimeout(() => loadData(), 3000);
            }
        } catch (error) {
            setMessage({ type: 'error', text: `Error saving rule: ${error}` });
            setTimeout(() => loadData(), 3000);
        } finally {
            setSavingRecords(prev => {
                const next = new Set(prev);
                next.delete(record.uuid);
                return next;
            });
        }
    };

    const addConfigRecord = () => {
        const newRecord: ConfigRecord = {
            uuid: crypto.randomUUID(),
            requestModel: '',
            responseModel: '',
            active: true,
            providers: [
                {
                    uuid: crypto.randomUUID(),
                    provider: '',
                    model: '',
                },
            ],
        };
        setConfigRecords([...configRecords, newRecord]);
        // Don't automatically expand to avoid render issues
        // Users can manually expand when needed
    };

    const deleteRule = (recordId: string) => {
        setRecordToDelete(recordId);
        setDeleteDialogOpen(true);
    };

    const confirmDeleteRule = async () => {
        if (recordToDelete) {
            await api.deleteRule(recordToDelete);
            await loadData();
        }
        setDeleteDialogOpen(false);
        setRecordToDelete(null);
    };

    const cancelDelete = () => {
        setDeleteDialogOpen(false);
        setRecordToDelete(null);
    };

    const updateConfigRecord = (recordId: string, field: keyof ConfigRecord, value: any) => {
        setConfigRecords(
            configRecords.map((record) =>
                record.uuid === recordId ? { ...record, [field]: value } : record
            )
        );
    };

    const addProvider = (recordId: string) => {
        setConfigRecords(
            configRecords.map((record) =>
                record.uuid === recordId
                    ? {
                        ...record,
                        providers: [
                            ...record.providers,
                            { uuid: crypto.randomUUID(), provider: '', model: '', isManualInput: false },
                        ],
                    }
                    : record
            )
        );
    };

    const deleteProvider = (recordId: string, providerId: string) => {
        setConfigRecords(
            configRecords.map((record) =>
                record.uuid === recordId
                    ? { ...record, providers: record.providers.filter((p) => p.uuid !== providerId) }
                    : record
            )
        );
    };

    const updateProvider = (
        recordId: string,
        providerId: string,
        field: keyof ConfigProvider,
        value: string | boolean
    ) => {
        setConfigRecords(
            configRecords.map((record) =>
                record.uuid === recordId
                    ? {
                        ...record,
                        providers: record.providers.map((p) => {
                            if (p.uuid === providerId) {
                                const updatedProvider = { ...p, [field]: value };

                                // If provider is changing, reset model to empty (Select option)
                                if (field === 'provider') {
                                    updatedProvider.model = '';
                                }

                                return updatedProvider;
                            }
                            return p;
                        }),
                    }
                    : record
            )
        );
    };

    const resetRule = async (recordId: string) => {
        // Reload data from server to reset changes
        await loadData();
        setMessage({ type: 'success', text: 'Rule reset to latest saved state' });
    };

    const toggleCardExpanded = (recordId: string) => {
        setExpandedCards(prev => {
            const next = new Set(prev);
            if (next.has(recordId)) {
                next.delete(recordId);
            } else {
                next.add(recordId);
            }
            return next;
        });
        // Don't update URL to avoid page jumps
        // URL params are only for initial load from bookmarks
    };

    // Fetch models for a specific provider (lazy loading)
    const fetchProviderModels = useCallback(async (providerUuid: string) => {
        if (!providerUuid || providerModels[providerUuid]) {
            // Already loaded or no UUID
            return;
        }

        try {
            const result = await api.getProviderModelsByUUID(providerUuid);
            if (result.success && result.data) {
                setProviderModels((prev: any) => ({
                    ...prev,
                    [providerUuid]: {
                        models: result.data.models || []
                    }
                }));
            }
        } catch (error) {
            console.error(`Failed to fetch models for provider ${providerUuid}:`, error);
        }
    }, [providerModels]);

    const handleRefreshProviderModels = async (uid: string) => {
        if (!uid) return;

        try {
            const result = await api.getProviderModelsByUUID(uid);
            if (result.success && result.data) {
                // Update providerModels with UUID as key
                setProviderModels((prev: any) => ({
                    ...prev,
                    [uid]: {
                        models: result.data.models || []
                    }
                }));
                setMessage({ type: 'success', text: `Successfully refreshed models for ${providerUuidToName[uid] || uid}` });
            } else {
                setMessage({ type: 'error', text: `Failed to refresh models: ${result.error || result.message}` });
            }
        } catch (error) {
            setMessage({ type: 'error', text: `Failed to refresh models: ${error}` });
        }
    };

    // Fetch models for existing providers when rules are loaded (lazy - only for providers that are actually used)
    useEffect(() => {
        const loadUsedProviderModels = async () => {
            const usedProviderUuids = new Set<string>();
            configRecords.forEach(record => {
                record.providers.forEach(p => {
                    if (p.provider) {
                        usedProviderUuids.add(p.provider);
                    }
                });
            });

            const loadPromises = Array.from(usedProviderUuids)
                .filter(uuid => !providerModels[uuid]) // Only load if not already cached
                .map(uuid => fetchProviderModels(uuid));

            await Promise.all(loadPromises);
        };

        loadUsedProviderModels();
    }, [configRecords, fetchProviderModels, providerModels]);

    return (
        <PageLayout
            loading={loading}
            notification={{
                open: !!message,
                message: message?.text,
                severity: message?.type,
                autoHideDuration: message?.type === 'success' ? 4000 : 6000, // Success messages close faster
                onClose: () => setMessage(null)
            }}
        >
            <UnifiedCard
                title="Advance Proxy Configuration"
                subtitle="Config local model which forwarding to remote providers"
                size="full"
                rightAction={
                    <Button
                        variant="contained"
                        size="small"
                        startIcon={<AddIcon />}
                        onClick={addConfigRecord}
                    >
                        Add Forwarding Rule
                    </Button>
                }
            >
                <Stack spacing={3}>
                    {configRecords.length === 0 ? (
                        <Box sx={{
                            display: 'flex',
                            flexDirection: 'column',
                            alignItems: 'center',
                            justifyContent: 'center',
                            py: 8,
                            textAlign: 'center'
                        }}>
                            <Typography variant="h6" color="text.secondary" gutterBottom>
                                No rules configured
                            </Typography>
                            <Typography variant="body2" color="text.secondary">
                                Click "Add Rule" to create your first rule
                            </Typography>
                        </Box>
                    ) : (
                        <Stack spacing={2}>
                            {configRecords.map((record) => (
                                <RuleCard
                                    key={record.uuid}
                                    record={record}
                                    recordUuid={record.uuid}
                                    providers={providers}
                                    providerModels={providerModels}
                                    providerUuidToName={providerUuidToName}
                                    saving={savingRecords.has(record.uuid)}
                                    expanded={expandedCards.has(record.uuid)}
                                    onUpdateRecord={(field, value) => updateConfigRecord(record.uuid, field, value)}
                                    onUpdateProvider={(recordId, providerId, field, value) => updateProvider(recordId, providerId, field, value)}
                                    onAddProvider={() => addProvider(record.uuid)}
                                    onDeleteProvider={(recordId, providerId) => deleteProvider(recordId, providerId)}
                                    onRefreshModels={handleRefreshProviderModels}
                                    onFetchModels={fetchProviderModels}
                                    onSave={() => handleSaveRule(record)}
                                    onDelete={() => deleteRule(record.uuid)}
                                    onReset={() => resetRule(record.uuid)}
                                    onToggleExpanded={() => toggleCardExpanded(record.uuid)}
                                />
                            ))}
                        </Stack>
                    )}

                </Stack>
            </UnifiedCard>

            <Dialog
                open={deleteDialogOpen}
                onClose={cancelDelete}
                aria-labelledby="delete-dialog-title"
                aria-describedby="delete-dialog-description"
            >
                <DialogTitle id="delete-dialog-title">
                    Delete Rule
                </DialogTitle>
                <DialogContent>
                    <DialogContentText id="delete-dialog-description">
                        Are you sure you want to delete this rule? This action cannot be undone.
                    </DialogContentText>
                </DialogContent>
                <DialogActions>
                    <Button onClick={cancelDelete} color="primary">
                        Cancel
                    </Button>
                    <Button onClick={confirmDeleteRule} color="error" variant="contained" autoFocus>
                        Delete
                    </Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
};

export default RulePage;
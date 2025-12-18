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
import { useCallback, useEffect, useState } from 'react';
import { PageLayout } from '../components/PageLayout';
import RuleCard from '../components/RuleCard';
import UnifiedCard from '../components/UnifiedCard';
import { api } from '../services/api';


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
    const [rules, setRules] = useState<any>({});
    const [providers, setProviders] = useState<any[]>([]);
    const [providerModels, setProviderModels] = useState<any>({});
    const [configRecords, setConfigRecords] = useState<ConfigRecord[]>([]);
    const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [loading, setLoading] = useState(true);
    const [savingRecords, setSavingRecords] = useState<Set<string>>(new Set());
    const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
    const [recordToDelete, setRecordToDelete] = useState<string | null>(null);

    const loadData = useCallback(async () => {
        setLoading(true);
        try {
            const [providersResult, modelsResult, rulesResult] = await Promise.all([
                api.getProviders(),
                api.getProviderModels(),
                api.getRules(),
            ]);

            if (providersResult.success) {
                setProviders(providersResult.data);
            }
            if (modelsResult.success) {
                setProviderModels(modelsResult.data);
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
                        provider: service.provider || '',
                        model: service.model || '',
                        isManualInput: false,
                        weight: service.weight || 0,
                        active: service.active !== undefined ? service.active : true,
                        time_window: service.time_window || 0,
                    })),
                };
            });
            setConfigRecords(records);
        } else {
            setConfigRecords([]);
        }
    }, [rules]);

    const handleSaveRule = async (record: ConfigRecord) => {
        console.log(record)
        if (!record.requestModel || !record.uuid) {
            setMessage({ type: 'error', text: 'Request model name is required' });
            return;
        }

        for (const provider of record.providers) {
            if (provider.provider && !provider.model) {
                setMessage({ type: 'error', text: `Please select a model for provider ${provider.provider}` });
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
                    provider: provider.provider,
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

    const handleRefreshProviderModels = async (providerName: string) => {
        if (!providerName) return;

        try {
            const result = await api.getProviderModelsByName(providerName);
            console.log("found models", result.data)
            if (result.success) {
                // Update providerModels with the refreshed data
                // The result from getProviderModelsByName is a direct array, not an object with models field
                setProviderModels((prev: any) => {
                    const updated = {
                        ...prev,
                        [providerName]: {
                            models: result.data  // Wrap the array in a models object to match the expected structure
                        }
                    };
                    return updated;
                });
                setMessage({ type: 'success', text: `Successfully refreshed models for ${providerName}` });
            } else {
                setMessage({ type: 'error', text: `Failed to refresh models for ${providerName}: ${result.message}` });
            }
        } catch (error) {
            setMessage({ type: 'error', text: `Failed to refresh models for ${providerName}: ${error}` });
        }
    };

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
                title="Routing Configuration"
                subtitle="Forwarding local model to remote providers"
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
                                    providers={providers}
                                    providerModels={providerModels}
                                    saving={savingRecords.has(record.uuid)}
                                    onUpdateRecord={(field, value) => updateConfigRecord(record.uuid, field, value)}
                                    onUpdateProvider={(providerId, field, value) => updateProvider(record.uuid, providerId, field, value)}
                                    onAddProvider={() => addProvider(record.uuid)}
                                    onDeleteProvider={(providerId) => deleteProvider(record.uuid, providerId)}
                                    onRefreshModels={handleRefreshProviderModels}
                                    onSave={() => handleSaveRule(record)}
                                    onDelete={() => deleteRule(record.uuid)}
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
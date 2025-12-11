import {
    Add as AddIcon,
    Check as CheckIcon,
    Delete as DeleteIcon,
    Edit as EditIcon,
    Refresh as RefreshIcon,
    Save as SaveIcon
} from '@mui/icons-material';
import {
    Box,
    Button,
    Divider,
    FormControl,
    IconButton,
    InputAdornment,
    InputLabel,
    MenuItem,
    Select,
    Stack,
    TextField,
    Typography
} from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import { PageLayout } from '../components/PageLayout';
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
    providers: ConfigProvider[];
}

const Rule = () => {
    const [rules, setRules] = useState<any>({});
    const [providers, setProviders] = useState<any[]>([]);
    const [providerModels, setProviderModels] = useState<any>({});
    const [configRecords, setConfigRecords] = useState<ConfigRecord[]>([]);
    const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [loading, setLoading] = useState(true);
    const [savingRecords, setSavingRecords] = useState<Set<string>>(new Set());

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
            const records: ConfigRecord[] = rules.map((rule: any) => ({
                uuid: rule.uuid || crypto.randomUUID(),
                requestModel: rule.request_model || '',
                responseModel: rule.response_model || '',
                providers: (rule.services || []).map((service: any) => ({
                    uuid: crypto.randomUUID(),
                    provider: service.provider || '',
                    model: service.model || '',
                    isManualInput: false,
                    weight: service.weight || 0,
                    active: service.active !== undefined ? service.active : true,
                    time_window: service.time_window || 0,
                })),
            }));
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
        api.deleteRule(recordId).then(() => loadData())
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
            if (result.success) {
                await loadData();
                setMessage({ type: 'success', text: `Successfully refreshed models for ${providerName}` });
            } else {
                setMessage({ type: 'error', text: `Failed to refresh models for ${providerName}: ${result.error}` });
                setTimeout(() => loadData(), 3000);
            }
        } catch (error) {
            setMessage({ type: 'error', text: `Failed to refresh models for ${providerName}: ${error}` });
            setTimeout(() => loadData(), 3000);
        }
    };


    return (
        <PageLayout
            loading={loading}
            notification={{
                open: !!message,
                message: message?.text,
                severity: message?.type,
                onClose: () => setMessage(null)
            }}
        >
            <UnifiedCard
                title="Request Rule Configuration"
                subtitle="Configure api request to models"
                size="full"
                rightAction={
                    <Button
                        variant="contained"
                        size="small"
                        startIcon={<AddIcon />}
                        onClick={addConfigRecord}
                    >
                        Add Rule
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
                        configRecords.map((record) => (
                            <Box
                                key={record.uuid}
                                sx={{
                                    p: 2,
                                    border: '1px solid',
                                    borderColor: 'divider',
                                    borderRadius: 1,
                                    bgcolor: 'background.paper',
                                }}
                            >
                                <Box
                                    sx={{
                                        display: 'grid',
                                        gridTemplateColumns: '1fr 1.5fr',
                                        gap: 4,
                                    }}
                                >
                                    {/* Row 1: Headers */}
                                    <Box sx={{ gridColumn: '1', gridRow: '1' }}>
                                        <Stack direction="row" justifyContent="space-between" alignItems="center">
                                            <Typography variant="subtitle2">
                                                Request
                                            </Typography>

                                            <Stack direction="row" spacing={1}>
                                                <Button
                                                    variant="contained"
                                                    color="primary"
                                                    size="small"
                                                    startIcon={<SaveIcon />}
                                                    onClick={() => handleSaveRule(record)}
                                                    disabled={savingRecords.has(record.uuid)}
                                                >
                                                    {savingRecords.has(record.uuid) ? 'Saving...' : 'Save'}
                                                </Button>
                                                <Button
                                                    startIcon={<DeleteIcon />}
                                                    onClick={() => deleteRule(record.uuid)}
                                                    variant="outlined"
                                                    size="small"
                                                    disabled={savingRecords.has(record.uuid)}
                                                >
                                                    Delete
                                                </Button>
                                            </Stack>
                                        </Stack>
                                    </Box>
                                    <Box sx={{ gridColumn: '2', gridRow: '1' }}>
                                        <Stack direction="row" justifyContent="space-between" alignItems="center">
                                            <Typography variant="subtitle2">
                                                Service ({record.providers.length})
                                            </Typography>
                                            <Button
                                                startIcon={<AddIcon />}
                                                onClick={() => addProvider(record.uuid)}
                                                variant="outlined"
                                                size="small"
                                            >
                                                Add Service
                                            </Button>
                                        </Stack>
                                    </Box>

                                    {/* Row 2: Request Input Fields */}
                                    <Box sx={{ gridColumn: '1', gridRow: '2' }}>
                                        <Stack spacing={1.5}>
                                            <Stack direction="row" spacing={1.5} alignItems="center">
                                                <FormControl sx={{ flex: 1 }} size="small">
                                                    <TextField
                                                        label="Request Model"
                                                        value={record.requestModel}
                                                        onChange={(e) =>
                                                            updateConfigRecord(record.uuid, 'requestModel', e.target.value)
                                                        }
                                                        helperText="Model name"
                                                        fullWidth
                                                        size="small"
                                                    />
                                                </FormControl>
                                                <FormControl sx={{ flex: 1 }} size="small">

                                                    <TextField
                                                        label="Response Model"
                                                        value={record.responseModel}
                                                        onChange={(e) =>
                                                            updateConfigRecord(record.uuid, 'responseModel', e.target.value)
                                                        }
                                                        helperText="Empty for as-is"
                                                        fullWidth
                                                        size="small"
                                                    />
                                                </FormControl>
                                            </Stack>
                                        </Stack>
                                    </Box>

                                    {/* Row 3+: Provider Configurations (one row per provider) */}
                                    <Box sx={{ gridColumn: '2', gridRow: '2' }}>
                                        <Stack spacing={1.5}>
                                            {record.providers.map((provider) => (
                                                <>
                                                    <Stack direction="row" spacing={1.5} alignItems="center">
                                                        <FormControl sx={{ flex: 1 }} size="small">
                                                            <InputLabel>Provider</InputLabel>
                                                            <Select
                                                                value={provider.provider}
                                                                onChange={(e) =>
                                                                    updateProvider(
                                                                        record.uuid,
                                                                        provider.uuid,
                                                                        'provider',
                                                                        e.target.value
                                                                    )
                                                                }
                                                                label="Provider"
                                                            >
                                                                <MenuItem value="">Select</MenuItem>
                                                                {providers.map((p) => (
                                                                    <MenuItem key={p.name} value={p.name}>
                                                                        {p.name}
                                                                    </MenuItem>
                                                                ))}
                                                            </Select>
                                                        </FormControl>

                                                        {provider.isManualInput ? (
                                                            <FormControl sx={{ flex: 1 }} size="small">
                                                                <TextField
                                                                    label="Model (Manual)"
                                                                    value={provider.model}
                                                                    onChange={(e) =>
                                                                        updateProvider(
                                                                            record.uuid,
                                                                            provider.uuid,
                                                                            'model',
                                                                            e.target.value
                                                                        )
                                                                    }
                                                                    placeholder="Enter model name manually"
                                                                    fullWidth
                                                                    size="small"
                                                                    InputProps={{
                                                                        endAdornment: (
                                                                            <InputAdornment position="end">
                                                                                <IconButton
                                                                                    size="small"
                                                                                    onClick={() =>
                                                                                        updateProvider(
                                                                                            record.uuid,
                                                                                            provider.uuid,
                                                                                            'isManualInput',
                                                                                            false
                                                                                        )
                                                                                    }
                                                                                    title="Switch to dropdown"
                                                                                    edge="end"
                                                                                >
                                                                                    <CheckIcon fontSize="small" />
                                                                                </IconButton>
                                                                            </InputAdornment>
                                                                        ),
                                                                    }}
                                                                />
                                                            </FormControl>
                                                        ) : (
                                                            <Box sx={{ flex: 1, position: 'relative' }}>
                                                                <FormControl size="small" disabled={!provider.provider}
                                                                    fullWidth>
                                                                    <InputLabel>Model</InputLabel>
                                                                    <Select
                                                                        value={provider.model}
                                                                        onChange={(e) =>
                                                                            updateProvider(
                                                                                record.uuid,
                                                                                provider.uuid,
                                                                                'model',
                                                                                e.target.value
                                                                            )
                                                                        }
                                                                        label="Model"
                                                                        sx={{
                                                                            '& .MuiOutlinedInput-notchedOutline': {
                                                                                paddingRight: '110px', // Make room for buttons
                                                                            },
                                                                        }}
                                                                    >
                                                                        <MenuItem value="">Select</MenuItem>
                                                                        {/* Show current model if it exists and is not in the API list */}
                                                                        {provider.model && !providerModels[provider.provider]?.models.includes(provider.model) && (
                                                                            <MenuItem key="current-manual"
                                                                                value={provider.model}>
                                                                                {provider.model} (custom)
                                                                            </MenuItem>
                                                                        )}
                                                                        {/* Show models from API */}
                                                                        {providerModels[provider.provider]?.models.map(
                                                                            (model: string) => (
                                                                                <MenuItem key={model} value={model}>
                                                                                    {model}
                                                                                </MenuItem>
                                                                            )
                                                                        )}
                                                                    </Select>
                                                                </FormControl>
                                                                <Box
                                                                    sx={{
                                                                        position: 'absolute',
                                                                        right: 8,
                                                                        top: '50%',
                                                                        transform: 'translateY(-50%)',
                                                                        display: 'flex',
                                                                        gap: 0.5,
                                                                        bgcolor: 'background.paper',
                                                                        borderRadius: 1,
                                                                        padding: '2px',
                                                                        pointerEvents: 'auto',
                                                                    }}
                                                                >
                                                                    <IconButton
                                                                        size="small"
                                                                        onClick={() => handleRefreshProviderModels(provider.provider)}
                                                                        disabled={!provider.provider}
                                                                        title="Refresh models"
                                                                        sx={{
                                                                            padding: '4px',
                                                                            '&:hover': { bgcolor: 'action.hover' }
                                                                        }}
                                                                    >
                                                                        <RefreshIcon fontSize="small" />
                                                                    </IconButton>
                                                                    <IconButton
                                                                        size="small"
                                                                        onClick={() =>
                                                                            updateProvider(
                                                                                record.uuid,
                                                                                provider.uuid,
                                                                                'isManualInput',
                                                                                true
                                                                            )
                                                                        }
                                                                        title="Switch to manual input"
                                                                        sx={{
                                                                            padding: '4px',
                                                                            '&:hover': { bgcolor: 'action.hover' }
                                                                        }}
                                                                    >
                                                                        <EditIcon fontSize="small" />
                                                                    </IconButton>
                                                                </Box>
                                                            </Box>
                                                        )}

                                                        <IconButton
                                                            size="small"
                                                            onClick={() => deleteProvider(record.uuid, provider.uuid)}
                                                            color="error"
                                                            sx={{ p: 0.5 }}
                                                        >
                                                            <DeleteIcon fontSize="small" />
                                                        </IconButton>
                                                    </Stack>
                                                    <Divider sx={{ mt: 1.5 }} />
                                                </>
                                            ))}
                                        </Stack>
                                    </Box>
                                </Box>
                            </Box>
                        )))
                    }

                </Stack>
            </UnifiedCard>
        </PageLayout>
    );
};

export default Rule;
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
import { useEffect, useState } from 'react';
import { PageLayout } from '../components/PageLayout';
import UnifiedCard from '../components/UnifiedCard';
import { api } from '../services/api';

interface ConfigProvider {
    id: string;
    provider: string;
    model: string;
    isManualInput?: boolean;
}

interface ConfigRecord {
    id: string;
    requestModel: string;
    responseModel: string;
    providers: ConfigProvider[];
}

const Rule = () => {
    const [defaults, setDefaults] = useState<any>({});
    const [providers, setProviders] = useState<any[]>([]);
    const [providerModels, setProviderModels] = useState<any>({});
    const [configRecords, setConfigRecords] = useState<ConfigRecord[]>([]);
    const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        loadAllData();
    }, []);

    const loadAllData = async () => {
        setLoading(true);
        await Promise.all([
            loadDefaults(),
            loadProviderSelectionPanel(),
        ]);
        setLoading(false);
    };

    const loadDefaults = async () => {
        const result = await api.getDefaults();
        if (result.success) {
            setDefaults(result.data);
        }
    };

    const loadProviderSelectionPanel = async () => {
        const [providersResult, modelsResult, defaultsResult] = await Promise.all([
            api.getProviders(),
            api.getProviderModels(),
            api.getDefaults(),
        ]);

        if (providersResult.success && modelsResult.success) {
            setProviders(providersResult.data);
            setProviderModels(modelsResult.data);
            if (defaultsResult.success) {
                setDefaults(defaultsResult.data);
            }
        }
    };

    useEffect(() => {
        if (defaults) {
            // Handle new data structure with request_configs
            const requestConfigs = defaults.request_configs || [];

            if (requestConfigs.length > 0) {
                // Group configs by request_model
                const groupedByRequestModel = requestConfigs.reduce((acc: any, config: any) => {
                    const requestModel = config.request_model || 'tingly';
                    if (!acc[requestModel]) {
                        acc[requestModel] = {
                            id: `record-${Date.now()}-${Math.random()}`,
                            requestModel,
                            responseModel: config.response_model || '',
                            providers: [],
                            responseModels: config.response_model ? [config.response_model] : [],
                        };
                    }

                    // Add provider to the group if it exists
                    if (config.provider) {
                        acc[requestModel].providers.push({
                            id: `provider-${Date.now()}-${Math.random()}`,
                            provider: config.provider,
                            model: config.default_model || '',
                            responseModel: config.response_model || '',
                            isManualInput: false,
                        });
                    }

                    return acc;
                }, {});

                // Convert grouped object to array of records
                const records: ConfigRecord[] = Object.values(groupedByRequestModel).map((record: any) => {
                    // Check if all providers have the same responseModel
                    const uniqueResponseModels = [...new Set(record.responseModels)] as string[];
                    const finalResponseModel: string = uniqueResponseModels.length === 1
                        ? uniqueResponseModels[0]
                        : '';

                    // Remove responseModels and responseModel from providers before returning
                    const providers = record.providers.map((p: any) => ({
                        id: p.id,
                        provider: p.provider,
                        model: p.model,
                    }));

                    return {
                        id: record.id,
                        requestModel: record.requestModel,
                        responseModel: finalResponseModel,
                        providers,
                    };
                });

                setConfigRecords(records);
            } else {
                // Fallback to old structure for backward compatibility
                const initialRecord: ConfigRecord = {
                    id: `record-${Date.now()}`,
                    requestModel: defaults.requestModel || 'tingly',
                    responseModel: defaults.responseModel || '',
                    providers: defaults.defaultProvider
                        ? [
                            {
                                id: `provider-${Date.now()}`,
                                provider: defaults.defaultProvider,
                                model: defaults.defaultModel || '',
                                isManualInput: false,
                            },
                        ]
                        : [],
                };
                setConfigRecords([initialRecord]);
            }
        }
    }, [defaults]);

    const generateId = () => `id-${Date.now()}-${Math.random()}`;

    const addConfigRecord = () => {
        const newRecord: ConfigRecord = {
            id: generateId(),
            requestModel: 'tingly',
            responseModel: '',
            providers: [
                {
                    id: generateId(),
                    provider: '',
                    model: '',
                },
            ],
        };
        setConfigRecords([...configRecords, newRecord]);
    };

    const deleteConfigRecord = (recordId: string) => {
        if (configRecords.length > 1) {
            setConfigRecords(configRecords.filter((record) => record.id !== recordId));
        }
    };

    const updateConfigRecord = (recordId: string, field: keyof ConfigRecord, value: any) => {
        setConfigRecords(
            configRecords.map((record) =>
                record.id === recordId ? { ...record, [field]: value } : record
            )
        );
    };

    const addProvider = (recordId: string) => {
        setConfigRecords(
            configRecords.map((record) =>
                record.id === recordId
                    ? {
                        ...record,
                        providers: [
                            ...record.providers,
                            { id: generateId(), provider: '', model: '', isManualInput: false },
                        ],
                    }
                    : record
            )
        );
    };

    const deleteProvider = (recordId: string, providerId: string) => {
        setConfigRecords(
            configRecords.map((record) =>
                record.id === recordId
                    ? { ...record, providers: record.providers.filter((p) => p.id !== providerId) }
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
                record.id === recordId
                    ? {
                        ...record,
                        providers: record.providers.map((p) =>
                            p.id === providerId ? { ...p, [field]: value } : p
                        ),
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
                await loadProviderSelectionPanel();
                setMessage({ type: 'success', text: `Successfully refreshed models for ${providerName}` });
            } else {
                setMessage({ type: 'error', text: `Failed to refresh models for ${providerName}: ${result.error}` });
            }
        } catch (error) {
            setMessage({ type: 'error', text: `Failed to refresh models for ${providerName}: ${error}` });
        }
    };

    const handleSaveDefaults = async () => {
        if (configRecords.length === 0) {
            setMessage({ type: 'error', text: 'No configuration records to save' });
            return;
        }

        for (const record of configRecords) {
            console.log("record", record)
            if (!record.requestModel) {
                setMessage({
                    type: 'error',
                    text: `Request model name is required for record ${record.id}`,
                });
                return;
            }

            for (const provider of record.providers) {
                if (provider.provider && !provider.model) {
                    setMessage({
                        type: 'error',
                        text: `Please select a model for provider ${provider.provider}`,
                    });
                    return;
                }
            }
        }

        // Convert config records to request_configs format
        const requestConfigs = configRecords.flatMap(record => {
            // For simplicity, use the first provider if exists
            console.log(record)
            return record.providers.map(it => {
                return {
                    request_model: record.requestModel,
                    response_model: record.responseModel,
                    provider: it.provider,
                    default_model: it.model,
                }
            })
        });

        const payload = {
            request_configs: requestConfigs,
        };

        try {
            console.log("payload", payload)
            const result = await api.setDefaults(payload);
            if (result.success) {
                setMessage({ type: 'success', text: 'Configurations saved successfully' });
                await loadProviderSelectionPanel();
            } else {
                setMessage({ type: 'error', text: result.error || 'Failed to save configurations' });
            }
        } catch (error) {
            setMessage({ type: 'error', text: `Error saving configurations: ${error}` });
        }
    };

    return (
        <PageLayout loading={loading} message={message} onClearMessage={() => setMessage(null)}>
            <UnifiedCard
                title="Request Rule Configuration"
                subtitle="Configure api request to models"
                size="full"
                rightAction={
                    <>
                        <Button
                            variant="contained"
                            size="small"
                            startIcon={<AddIcon />}
                            onClick={addConfigRecord}
                        >
                            Add Configuration
                        </Button>
                        <Button
                            variant="contained"
                            color="primary"
                            size="small"
                            startIcon={<SaveIcon />}
                            onClick={handleSaveDefaults}
                        >
                            Save
                        </Button>
                    </>
                }
            >
                <Stack spacing={3}>
                    {configRecords.map((record) => (
                        <Box
                            key={record.id}
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

                                        <Button
                                            startIcon={<DeleteIcon />}
                                            onClick={() => deleteConfigRecord(record.id)}
                                            variant="outlined"
                                            size="small"
                                        >
                                            Delete
                                        </Button>
                                    </Stack>
                                </Box>
                                <Box sx={{ gridColumn: '2', gridRow: '1' }}>
                                    <Stack direction="row" justifyContent="space-between" alignItems="center">
                                        <Typography variant="subtitle2">
                                            Models ({record.providers.length})
                                        </Typography>
                                        <Button
                                            startIcon={<AddIcon />}
                                            onClick={() => addProvider(record.id)}
                                            variant="outlined"
                                            size="small"
                                        >
                                            Add Model
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
                                                        updateConfigRecord(record.id, 'requestModel', e.target.value)
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
                                                        updateConfigRecord(record.id, 'responseModel', e.target.value)
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
                                                                    record.id,
                                                                    provider.id,
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
                                                                        record.id,
                                                                        provider.id,
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
                                                                                        record.id,
                                                                                        provider.id,
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
                                                                            record.id,
                                                                            provider.id,
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
                                                                            record.id,
                                                                            provider.id,
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
                                                        onClick={() => deleteProvider(record.id, provider.id)}
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
                    ))}

                </Stack>
            </UnifiedCard>
        </PageLayout>
    );
};

export default Rule;
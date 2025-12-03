import { Add as AddIcon, Delete as DeleteIcon, Refresh as RefreshIcon, Edit as EditIcon, Check as CheckIcon } from '@mui/icons-material';
import {
    Box,
    Button,
    Divider,
    FormControl,
    IconButton,
    InputLabel,
    MenuItem,
    Select,
    Stack,
    TextField,
    Typography,
    Switch,
    FormControlLabel,
} from '@mui/material';
import { useEffect, useState } from 'react';
import { api } from '../services/api';
import UnifiedCard from './UnifiedCard';

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

interface ModelConfigCardProps {
    defaults: any;
    providers: any[];
    providerModels: any;
    onLoadDefaults: () => Promise<void>;
    onLoadProviderSelectionPanel: () => Promise<void>;
    onFetchModels: (providerName: string) => Promise<void>;
}

const ModelConfigCard = ({
    defaults,
    providers,
    providerModels,
    onLoadDefaults,
    onLoadProviderSelectionPanel,
    onFetchModels,
}: ModelConfigCardProps) => {
    const [configRecords, setConfigRecords] = useState<ConfigRecord[]>([]);
    const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

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
            await onFetchModels(providerName);
            setMessage({ type: 'success', text: `Successfully refreshed models for ${providerName}` });
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
                await onLoadProviderSelectionPanel();
            } else {
                setMessage({ type: 'error', text: result.error || 'Failed to save configurations' });
            }
        } catch (error) {
            setMessage({ type: 'error', text: `Error saving configurations: ${error}` });
        }
    };

    return (
        <UnifiedCard
            title="Model Configuration"
            subtitle="Configure model providers and settings"
            size="full"
            message={message}
            onClearMessage={() => setMessage(null)}
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
                                        Providers ({record.providers.length})
                                    </Typography>
                                    <Button
                                        startIcon={<AddIcon />}
                                        onClick={() => addProvider(record.id)}
                                        variant="outlined"
                                        size="small"
                                    >
                                        Add Provider
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
                                                        />
                                                    </FormControl>
                                                ) : (
                                                    <FormControl
                                                        sx={{ flex: 1 }}
                                                        size="small"
                                                        disabled={!provider.provider}
                                                    >
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
                                                        >
                                                            <MenuItem value="">Select</MenuItem>
                                                            {/* Show current model if it exists and is not in the API list */}
                                                            {provider.model && !providerModels[provider.provider]?.models.includes(provider.model) && (
                                                                <MenuItem key="current-manual" value={provider.model}>
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
                                                )}

                                                <IconButton
                                                    size="small"
                                                    onClick={() => handleRefreshProviderModels(provider.provider)}
                                                    disabled={!provider.provider}
                                                    title="Refresh models"
                                                    sx={{ p: 0.5 }}
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
                                                            !provider.isManualInput
                                                        )
                                                    }
                                                    title={provider.isManualInput ? "Switch to dropdown" : "Switch to manual input"}
                                                    sx={{ p: 0.5 }}
                                                >
                                                    {provider.isManualInput ? <CheckIcon fontSize="small" /> : <EditIcon fontSize="small" />}
                                                </IconButton>

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

                <Stack direction="row" spacing={2}>
                    <Button variant="contained" onClick={addConfigRecord} startIcon={<AddIcon />}>
                        Add Configuration
                    </Button>
                    <Button variant="contained" onClick={handleSaveDefaults}>
                        Save All Configurations
                    </Button>
                    <Button variant="outlined" onClick={onLoadDefaults}>
                        Refresh Models
                    </Button>
                </Stack>
            </Stack>
        </UnifiedCard>
    );
};

export default ModelConfigCard;

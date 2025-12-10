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
    weight?: number;
    active?: boolean;
    time_window?: number;
}

interface ConfigRecord {
    id: string;
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

    useEffect(() => {
        loadAllData();
    }, []);

    const loadAllData = async () => {
        setLoading(true);
        await Promise.all([
            loadRules(),
            loadProviderSelectionPanel(),
        ]);
        setLoading(false);
    };

    const loadRules = async () => {
        const result = await api.getRules();
        if (result.success) {
            console.log('Loaded rules data:', result.data);
            setRules(result.data);
        }
    };

    const loadProviderSelectionPanel = async () => {
        const [providersResult, modelsResult, rulesResult] = await Promise.all([
            api.getProviders(),
            api.getProviderModels(),
            api.getRules(),
        ]);

        if (providersResult.success && modelsResult.success) {
            setProviders(providersResult.data);
            setProviderModels(modelsResult.data);
            if (rulesResult.success) {
                setRules(rulesResult.data);
            }
        }
    };

    useEffect(() => {
        console.log('Rules state changed:', rules);
        if (rules && Array.isArray(rules)) {
            // Handle new rule structure
            if (rules.length > 0) {
                const records: ConfigRecord[] = rules.map((rule: any) => {
                    console.log('Processing rule:', rule);
                    return {
                        id: `record-${rule.request_model || Date.now()}-${Math.random()}`,
                        requestModel: rule.request_model || '',  // Don't use default 'tingly' when request_model is empty
                        responseModel: rule.response_model || '',
                        providers: rule.services ? rule.services.map((service: any) => ({
                            id: `provider-${Date.now()}-${Math.random()}`,
                            provider: service.provider || '',
                            model: service.model || '',
                            isManualInput: false,
                            weight: service.weight || 0,
                            active: service.active !== undefined ? service.active : true,
                            time_window: service.time_window || 0,
                        })) : [],
                    };
                });
                console.log('Mapped config records:', records);
                setConfigRecords(records);
            } else {
                // No rules exist, don't create any records
                setConfigRecords([]);
            }
        } else if (rules === null || rules === undefined) {
            // Data not loaded yet, don't create any records
            setConfigRecords([]);
        }
    }, [rules]);

    const generateId = () => `id-${Date.now()}-${Math.random()}`;

    const handleSaveSingleRule = async (record: ConfigRecord) => {
        if (!record.requestModel) {
            setMessage({
                type: 'error',
                text: `Request model name is required`,
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

        const ruleName = record.requestModel;

        // Add to saving set
        setSavingRecords(prev => new Set(prev).add(record.id));

        try {
            // Create rule data with services
            const ruleData = {
                response_model: record.responseModel || undefined,
                services: record.providers.map(provider => ({
                    provider: provider.provider,
                    model: provider.model,
                    weight: provider.weight || 0,
                    active: provider.active !== undefined ? provider.active : true,
                    time_window: provider.time_window || 0,
                })),
            };

            // Check if rule exists and update or create
            const existingRule = await api.getRule(ruleName);

            let result;
            if (existingRule.success) {
                // Update existing rule
                result = await api.updateRule(ruleName, ruleData);
            } else {
                // Create new rule
                result = await api.createRule({
                    name: ruleName,
                    ...ruleData,
                });
            }

            if (result.success) {
                setMessage({ type: 'success', text: `Rule "${ruleName}" saved successfully` });
                await loadRules();
            } else {
                setMessage({ type: 'error', text: `Failed to save rule "${ruleName}": ${result.error || 'Unknown error'}` });
            }
        } catch (error) {
            setMessage({ type: 'error', text: `Error saving rule "${ruleName}": ${error}` });
        } finally {
            // Remove from saving set
            setSavingRecords(prev => {
                const newSet = new Set(prev);
                newSet.delete(record.id);
                return newSet;
            });
        }
    };

    const addConfigRecord = () => {
        const newRecord: ConfigRecord = {
            id: generateId(),
            requestModel: '',
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
        setConfigRecords(configRecords.filter((record) => record.id !== recordId));
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
                        providers: record.providers.map((p) => {
                            if (p.id === providerId) {
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
                await loadProviderSelectionPanel();
                setMessage({ type: 'success', text: `Successfully refreshed models for ${providerName}` });
            } else {
                setMessage({ type: 'error', text: `Failed to refresh models for ${providerName}: ${result.error}` });
            }
        } catch (error) {
            setMessage({ type: 'error', text: `Failed to refresh models for ${providerName}: ${error}` });
        }
    };

    const handleSaveAll = async () => {
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

        // Convert config records to rule format
        // Each record will become a rule with services
        try {
            for (const record of configRecords) {
                const ruleName = record.requestModel;

                // Create rule data with services
                const ruleData = {
                    response_model: record.responseModel || undefined,
                    services: record.providers.map(provider => ({
                        provider: provider.provider,
                        model: provider.model,
                    })),
                };

                // Check if rule exists and update or create
                const existingRule = await api.getRule(ruleName);

                let result;
                if (existingRule.success) {
                    // Update existing rule
                    result = await api.updateRule(ruleName, ruleData);
                } else {
                    // Create new rule
                    result = await api.createRule({
                        name: ruleName,
                        ...ruleData,
                    });
                }

                if (!result.success) {
                    setMessage({ type: 'error', text: `Failed to save rule ${ruleName}: ${result.error || 'Unknown error'}` });
                    return;
                }
            }

            setMessage({ type: 'success', text: 'Rules saved successfully' });
            await loadAllData();
        } catch (error) {
            setMessage({ type: 'error', text: `Error saving rules: ${error}` });
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
                    <>
                        <Button
                            variant="contained"
                            size="small"
                            startIcon={<AddIcon />}
                            onClick={addConfigRecord}
                        >
                            Add Rule
                        </Button>
                        <Button
                            variant="contained"
                            color="primary"
                            size="small"
                            startIcon={<SaveIcon />}
                            onClick={handleSaveAll}
                        >
                            Save All
                        </Button>
                    </>
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

                                        <Stack direction="row" spacing={1}>
                                            <Button
                                                variant="contained"
                                                color="primary"
                                                size="small"
                                                startIcon={<SaveIcon />}
                                                onClick={() => handleSaveSingleRule(record)}
                                                disabled={savingRecords.has(record.id)}
                                            >
                                                {savingRecords.has(record.id) ? 'Saving...' : 'Save'}
                                            </Button>
                                            <Button
                                                startIcon={<DeleteIcon />}
                                                onClick={() => deleteConfigRecord(record.id)}
                                                variant="outlined"
                                                size="small"
                                                disabled={savingRecords.has(record.id)}
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
                                            onClick={() => addProvider(record.id)}
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
                    )))
                    }

                </Stack>
            </UnifiedCard>
        </PageLayout>
    );
};

export default Rule;
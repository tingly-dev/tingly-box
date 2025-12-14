import {
    Add as AddIcon,
    Check as CheckIcon,
    Delete as DeleteIcon,
    Dns as DnsIcon,
    Edit as EditIcon,
    Refresh as RefreshIcon,
    Save as SaveIcon
} from '@mui/icons-material';
import {
    Box,
    Button,
    Card,
    CardActions,
    CardContent,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogContentText,
    DialogTitle,
    Divider,
    FormControl,
    IconButton,
    InputAdornment,
    InputLabel,
    MenuItem,
    Select,
    Stack,
    Switch,
    TextField,
    Tooltip,
    Typography
} from '@mui/material';
import { styled } from '@mui/material/styles';
import { useCallback, useEffect, useState } from 'react';
import { PageLayout } from '../components/PageLayout';
import UnifiedCard from '../components/UnifiedCard';
import { api } from '../services/api';

const ServiceSection = styled(Box)(({ theme }) => ({
    maxHeight: 160,
    overflowY: 'auto',
    paddingTop: 10,
    marginRight: -theme.spacing(1),
    paddingRight: theme.spacing(2),
    '&::-webkit-scrollbar': {
        width: 6,
    },
    '&::-webkit-scrollbar-track': {
        background: 'transparent',
    },
    '&::-webkit-scrollbar-thumb': {
        backgroundColor: 'rgba(0, 0, 0, 0.2)',
        borderRadius: 3,
    },
    '&::-webkit-scrollbar-thumb:hover': {
        backgroundColor: 'rgba(0, 0, 0, 0.3)',
    },
}));

const RuleCard = styled(Card)(({ theme }) => ({
    display: 'flex',
    flexDirection: 'column',
    '&:hover': {
        borderColor: theme.palette.primary.main,
        boxShadow: theme.shadows[1],
    },
}));

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

const Rule = () => {
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
                        <Stack spacing={2}>
                            {configRecords.map((record) => (
                                <RuleCard
                                    key={record.uuid}
                                    sx={{
                                        border: '1px solid',
                                        borderColor: record.active ? 'divider' : 'grey.300',
                                        position: 'relative',
                                        transition: 'all 0.2s ease-in-out',
                                        '&:hover': {
                                            borderColor: record.active ? 'primary.main' : 'grey.400',
                                        },
                                        ...(record.active ? {} : {
                                            backgroundColor: 'grey.50',
                                            '&::before': {
                                                content: '""',
                                                position: 'absolute',
                                                top: 0,
                                                left: 0,
                                                right: 0,
                                                bottom: 0,
                                                backgroundColor: 'rgba(0, 0, 0, 0.04)',
                                                zIndex: 1,
                                                pointerEvents: 'none',
                                            },
                                        }),
                                    }}
                                >
                                    <CardContent
                                        sx={{
                                            pb: 2,
                                            flexGrow: 1,
                                            display: 'flex',
                                            flexDirection: 'column',
                                            position: 'relative',
                                            zIndex: record.active ? 'auto' : 2,
                                            opacity: record.active ? 1 : 0.7,
                                            transition: 'opacity 0.2s ease-in-out',
                                        }}
                                    >
                                        <Stack spacing={2} sx={{ flexGrow: 1 }}>
                                            {/* Request Section */}
                                            <Box>
                                                <Stack direction="row" alignItems="center" justifyContent="space-between" mb={1.5}>
                                                    <Stack direction="row" alignItems="center" spacing={1}>
                                                        {/* <AutoAwesome sx={{ color: 'primary.main', fontSize: 20 }} /> */}
                                                        {/* <Typography variant="subtitle1" component="div" fontWeight={600}>
                                                            Rule
                                                        </Typography> */}
                                                        <Chip
                                                            label={record.active ? 'Active' : 'Inactive'}
                                                            color={record.active ? 'success' : 'default'}
                                                            size="small"
                                                        />
                                                    </Stack>
                                                    <Switch
                                                        checked={record.active}
                                                        onChange={(e) => updateConfigRecord(record.uuid, 'active', e.target.checked)}
                                                        size="small"
                                                        disabled={savingRecords.has(record.uuid)}
                                                    />
                                                </Stack>
                                                <Stack direction="row" spacing={1}>
                                                    <TextField
                                                        label="Request Model"
                                                        value={record.requestModel}
                                                        onChange={(e) =>
                                                            updateConfigRecord(record.uuid, 'requestModel', e.target.value)
                                                        }
                                                        helperText="Model name"
                                                        fullWidth
                                                        size="small"
                                                        disabled={!record.active}
                                                    />
                                                    <TextField
                                                        label="Response Model"
                                                        value={record.responseModel}
                                                        onChange={(e) =>
                                                            updateConfigRecord(record.uuid, 'responseModel', e.target.value)
                                                        }
                                                        helperText="Empty for as-is"
                                                        fullWidth
                                                        size="small"
                                                        disabled={!record.active}
                                                    />
                                                </Stack>
                                            </Box>

                                            <Divider />

                                            {/* Service Section */}
                                            <Box sx={{ flexGrow: 1, display: 'flex', flexDirection: 'column' }}>
                                                <Stack direction="row" alignItems="center" justifyContent="space-between" mb={1.5}>
                                                    <Stack direction="row" alignItems="center" spacing={1}>
                                                        <DnsIcon sx={{ color: 'secondary.main', fontSize: 20 }} />
                                                        <Typography variant="subtitle1" component="div" fontWeight={600}>
                                                            Services
                                                        </Typography>
                                                        <Chip
                                                            label={`${record.providers.length}`}
                                                            variant="outlined"
                                                            size="small"
                                                        />
                                                    </Stack>
                                                    <Tooltip title="Add service">
                                                        <IconButton
                                                            size="small"
                                                            onClick={() => addProvider(record.uuid)}
                                                            disabled={!record.active}
                                                            sx={{
                                                                padding: 1,
                                                                '&:hover': { bgcolor: 'action.hover' }
                                                            }}
                                                        >
                                                            <AddIcon fontSize="small" />
                                                        </IconButton>
                                                    </Tooltip>
                                                </Stack>

                                                <ServiceSection>
                                                    <Stack spacing={1}>
                                                        {record.providers.map((provider) => (
                                                            <Box key={provider.uuid}>
                                                                <Stack direction="row" spacing={1} alignItems="center">
                                                                    <FormControl sx={{ minWidth: 110 }} size="small" disabled={!record.active}>
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
                                                                            size="small"
                                                                            disabled={!record.active}
                                                                        >
                                                                            <MenuItem value="">Select</MenuItem>
                                                                            {providers.map((p) => (
                                                                                <MenuItem key={p.name} value={p.name}>
                                                                                    {p.name}
                                                                                </MenuItem>
                                                                            ))}
                                                                        </Select>
                                                                    </FormControl>

                                                                    <TextField
                                                                        label="API Style"
                                                                        value={provider.provider ? (providers.find(p => p.name === provider.provider)?.api_style || 'openai') : ''}
                                                                        placeholder=""
                                                                        size="small"
                                                                        sx={{ minWidth: 50 }}
                                                                        disabled
                                                                        slotProps={{
                                                                            input: {
                                                                                readOnly: true,
                                                                                style: {
                                                                                    fontFamily: 'monospace',
                                                                                }
                                                                            }
                                                                        }}
                                                                    />

                                                                    {provider.isManualInput ? (
                                                                        <Box sx={{ flex: 1, minWidth: 0, display: 'flex', alignItems: 'flex-end' }}>
                                                                            <TextField
                                                                                label="Model"
                                                                                value={provider.model}
                                                                                onChange={(e) =>
                                                                                    updateProvider(
                                                                                        record.uuid,
                                                                                        provider.uuid,
                                                                                        'model',
                                                                                        e.target.value
                                                                                    )
                                                                                }
                                                                                placeholder="Manual"
                                                                                size="small"
                                                                                sx={{ flex: 1, minWidth: 0 }}
                                                                                disabled={!record.active}
                                                                                slotProps={{
                                                                                    input: {
                                                                                        endAdornment: (
                                                                                            <InputAdornment position="end">
                                                                                                <Tooltip title="Switch to dropdown">
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
                                                                                                        edge="end"
                                                                                                        sx={{ padding: 1 }}
                                                                                                        disabled={!record.active}
                                                                                                    >
                                                                                                        <CheckIcon fontSize="small" />
                                                                                                    </IconButton>
                                                                                                </Tooltip>
                                                                                            </InputAdornment>
                                                                                        ),
                                                                                    }
                                                                                }}
                                                                            />
                                                                        </Box>
                                                                    ) : (
                                                                        <Box sx={{ flex: 1, minWidth: 0, display: 'flex', alignItems: 'flex-end', gap: 0.5 }}>
                                                                            <FormControl size="small" disabled={!provider.provider || !record.active} fullWidth sx={{ flex: 1 }}>
                                                                                <InputLabel>Model</InputLabel>
                                                                                <Select
                                                                                    key={`${provider.provider}-${JSON.stringify(providerModels[provider.provider]?.models || [])}`}
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
                                                                                    size="small"
                                                                                    disabled={!record.active}
                                                                                >
                                                                                    <MenuItem value="">Select</MenuItem>
                                                                                    {provider.model && !(providerModels[provider.provider]?.models || []).includes(provider.model) && (
                                                                                        <MenuItem key="current-manual" value={provider.model}>
                                                                                            {provider.model} (custom)
                                                                                        </MenuItem>
                                                                                    )}
                                                                                    {(providerModels[provider.provider]?.models || []).map(
                                                                                        (model: string) => (
                                                                                            <MenuItem key={model} value={model}>
                                                                                                {model}
                                                                                            </MenuItem>
                                                                                        )
                                                                                    )}
                                                                                </Select>
                                                                            </FormControl>
                                                                            <Box sx={{ display: 'flex', gap: 0.25 }}>
                                                                                <Tooltip title="Refresh models">
                                                                                    <IconButton
                                                                                        size="small"
                                                                                        onClick={() => handleRefreshProviderModels(provider.provider)}
                                                                                        disabled={!provider.provider || !record.active}
                                                                                        sx={{
                                                                                            padding: 1,
                                                                                            '&:hover': { bgcolor: 'action.hover' }
                                                                                        }}
                                                                                    >
                                                                                        <RefreshIcon fontSize="small" />
                                                                                    </IconButton>
                                                                                </Tooltip>
                                                                                <Tooltip title="Switch to manual input">
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
                                                                                        disabled={!record.active}
                                                                                        sx={{
                                                                                            padding: 1,
                                                                                            '&:hover': { bgcolor: 'action.hover' }
                                                                                        }}
                                                                                    >
                                                                                        <EditIcon fontSize="small" />
                                                                                    </IconButton>
                                                                                </Tooltip>
                                                                            </Box>
                                                                        </Box>
                                                                    )}

                                                                    <Tooltip title="Delete provider">
                                                                        <IconButton
                                                                            size="small"
                                                                            onClick={() => deleteProvider(record.uuid, provider.uuid)}
                                                                            color="error"
                                                                            sx={{ padding: 0.5, flexShrink: 0 }}
                                                                            disabled={!record.active}
                                                                        >
                                                                            <DeleteIcon fontSize="small" />
                                                                        </IconButton>
                                                                    </Tooltip>
                                                                </Stack>
                                                            </Box>
                                                        ))}
                                                    </Stack>
                                                </ServiceSection>

                                                {record.providers.length === 0 && (
                                                    <Box
                                                        sx={{
                                                            display: 'flex',
                                                            flexDirection: 'column',
                                                            alignItems: 'center',
                                                            justifyContent: 'center',
                                                            minHeight: 120,
                                                            textAlign: 'center',
                                                            border: '2px dashed',
                                                            borderColor: 'divider',
                                                            borderRadius: 1,
                                                        }}
                                                    >
                                                        <Typography variant="body2" color="text.secondary">
                                                            No services
                                                        </Typography>
                                                        <Typography variant="caption" color="text.secondary">
                                                            Click "Add" to configure
                                                        </Typography>
                                                    </Box>
                                                )}
                                            </Box>
                                        </Stack>
                                    </CardContent>
                                    <CardActions sx={{ px: 2, pb: 2, justifyContent: 'center', gap: 1 }}>
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
                                            color="error"
                                            disabled={savingRecords.has(record.uuid)}
                                        >
                                            Delete
                                        </Button>
                                    </CardActions>
                                </RuleCard>
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

export default Rule;
import {
    Add as AddIcon,
    Check as CheckIcon,
    Delete as DeleteIcon,
    Edit as EditIcon,
    Refresh as RefreshIcon
} from '@mui/icons-material';
import {
    Box,
    Chip,
    FormControl,
    IconButton,
    InputAdornment,
    InputLabel,
    MenuItem,
    Select,
    TextField,
    Tooltip,
    Typography
} from '@mui/material';
import React, { useEffect } from 'react';
import { dispatchCustomModelUpdate, listenForCustomModelUpdates, useCustomModels } from '../hooks/useCustomModels';

interface ConfigProvider {
    uuid: string;
    provider: string;
    model: string;
    isManualInput?: boolean;
    weight?: number;
    active?: boolean;
    time_window?: number;
}

interface ProviderConfigProps {
    providers: ConfigProvider[];
    availableProviders: any[];
    providerModels: any;
    providerUuidToName: { [uuid: string]: string };
    active: boolean;
    onAddProvider: () => void;
    onDeleteProvider: (providerId: string) => void;
    onUpdateProvider: (providerId: string, field: keyof ConfigProvider, value: any) => void;
    onRefreshModels: (providerUuid: string) => void;
}

const ProviderConfig: React.FC<ProviderConfigProps> = ({
    providers,
    availableProviders,
    providerModels,
    providerUuidToName,
    active,
    onAddProvider,
    onDeleteProvider,
    onUpdateProvider,
    onRefreshModels
}) => {
    const getApiStyle = (providerUuid: string) => {
        const provider = availableProviders.find(p => p.uuid === providerUuid);
        return provider?.api_style || 'openai';
    };

    return (
        <Box sx={{ flexGrow: 1, display: 'flex', flexDirection: 'column' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Typography variant="subtitle1" sx={{ fontWeight: 600, color: 'text.primary' }}>
                        Forwarding To
                    </Typography>
                    <Chip
                        label={providers.length}
                        size="small"
                        variant="outlined"
                        sx={{ height: 24, fontSize: '0.75rem' }}
                    />
                </Box>
                <Tooltip title="Add API Key">
                    <IconButton
                        size="small"
                        onClick={onAddProvider}
                        disabled={!active}
                        sx={{ padding: 0.5 }}
                    >
                        <AddIcon fontSize="small" />
                    </IconButton>
                </Tooltip>
            </Box>

            {providers.length === 0 ? (
                <Box
                    sx={{
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        justifyContent: 'center',
                        minHeight: 80,
                        border: '2px dashed',
                        borderColor: 'divider',
                        borderRadius: 2,
                        textAlign: 'center',
                        py: 2,
                        backgroundColor: 'grey.50'
                    }}
                >
                    <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                        No API keys configured
                    </Typography>
                </Box>
            ) : (
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5, maxHeight: 200, overflowY: 'auto' }}>
                    {providers.map((provider) => (
                        <ProviderRow
                            key={provider.uuid}
                            provider={provider}
                            apiStyle={getApiStyle(provider.provider)}
                            models={providerModels[providerUuidToName[provider.provider]]?.models || []}
                            availableProviders={availableProviders}
                            active={active}
                            onUpdate={(field, value) => onUpdateProvider(provider.uuid, field, value)}
                            onDelete={() => onDeleteProvider(provider.uuid)}
                            onRefreshModels={() => onRefreshModels(provider.provider)}
                            providerUuidToName={providerUuidToName}
                        />
                    ))}
                </Box>
            )}
        </Box>
    );
};

interface ProviderRowProps {
    provider: ConfigProvider;
    apiStyle: string;
    models: string[];
    availableProviders: any[];
    active: boolean;
    onUpdate: (field: keyof ConfigProvider, value: any) => void;
    onDelete: () => void;
    onRefreshModels: () => void;
    providerUuidToName: { [uuid: string]: string };
}

const ProviderRow: React.FC<ProviderRowProps> = ({
    provider,
    apiStyle,
    models,
    availableProviders,
    active,
    onUpdate,
    onDelete,
    onRefreshModels,
    providerUuidToName
}) => {
    const { saveCustomModel, getCustomModels } = useCustomModels();

    // Listen for custom model updates
    useEffect(() => {
        const cleanup = listenForCustomModelUpdates(() => {
            // Force re-render when custom models are updated
            // The hook will automatically handle state updates
        });
        return cleanup;
    }, []);

    // Handle model update
    const handleModelUpdate = (field: 'model' | 'isManualInput', value: any) => {
        if (field === 'model' && typeof value === 'string' && provider.isManualInput) {
            // Save to custom models when manually inputting
            saveCustomModel(provider.provider, value);
            // Notify other components
            dispatchCustomModelUpdate(provider.provider, value);
        }
        onUpdate(field, value);
    };

    // Get all available models including custom ones
    const getAllModels = () => {
        const customModelsList = getCustomModels(provider.provider);
        const allModels = [...models];

        // Add custom models if they exist and are not already in the list
        customModelsList.forEach((customModel: string) => {
            if (!allModels.includes(customModel)) {
                allModels.unshift(customModel); // Add at the beginning to prioritize
            }
        });

        return allModels;
    };

    // Check if a model is a custom model
    const isModelCustom = (model: string) => {
        return getCustomModels(provider.provider).includes(model);
    };
    return (
        <Box
            sx={{
                display: 'grid',
                gridTemplateColumns: '190px 80px 1fr auto',
                gap: 1.5,
                alignItems: 'center',
                p: 1.5,
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
                backgroundColor: 'background.paper',
                transition: 'all 0.2s ease-in-out',
                '&:hover': {
                    borderColor: 'primary.main',
                    boxShadow: 1,
                }
            }}
        >
            {/* Provider Select */}
            <FormControl size="small" disabled={!active} fullWidth>
                <InputLabel shrink sx={{ fontSize: '0.875rem',backgroundColor: 'white',  }}>Provider</InputLabel>
                <Select
                    value={provider.provider} // This is UUID
                    onChange={(e) => onUpdate('provider', e.target.value)}
                    label="Key"
                    size="small"
                    notched
                    sx={{
                        fontSize: '0.875rem',
                        '& .MuiSelect-select': {
                            whiteSpace: 'nowrap',
                            overflow: 'hidden',
                            textOverflow: 'ellipsis'
                        }
                    }}
                >
                    <MenuItem value="">Select API Key</MenuItem>
                    {availableProviders.map((p) => (
                        <MenuItem key={p.uuid} value={p.uuid}> {/* Use UUID as value */}
                            {p.name} {/* Display provider name */}
                        </MenuItem>
                    ))}
                </Select>
            </FormControl>

            {/* API Style */}
            <TextField
                value={provider.provider ? apiStyle : ''}
                size="small"
                fullWidth
                disabled
                label="Style"
                sx={{
                    '& .MuiInputBase-input': {
                        fontFamily: 'monospace',
                        fontSize: '0.75rem'
                    }
                }}
            />

            {/* Model Input */}
            {provider.isManualInput ? (
                <TextField
                    value={provider.model}
                    onChange={(e) => handleModelUpdate('model', e.target.value)}
                    placeholder="Enter model name"
                    size="small"
                    fullWidth
                    label="Model"
                    InputProps={{
                        endAdornment: (
                            <InputAdornment position="end">
                                <Tooltip title="Use dropdown">
                                    <IconButton
                                        size="small"
                                        onClick={() => onUpdate('isManualInput', false)}
                                        disabled={!active}
                                        sx={{ padding: 0.25 }}
                                    >
                                        <CheckIcon fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                            </InputAdornment>
                        )
                    }}
                />
            ) : (
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, width: '100%' }}>
                    <FormControl size="small" disabled={!provider.provider || !active} fullWidth>
                        <InputLabel shrink sx={{ fontSize: '0.875rem' }}>Model</InputLabel>
                        <Select
                            value={provider.model}
                            onChange={(e) => onUpdate('model', e.target.value)}
                            label="Model"
                            size="small"
                            notched
                            sx={{ fontSize: '0.875rem' }}
                        >
                            <MenuItem value="">Select Model</MenuItem>
                            {getAllModels().map((model) => (
                                <MenuItem key={model} value={model}>
                                    {model}
                                    {isModelCustom(model) && ' (custom)'}
                                </MenuItem>
                            ))}
                        </Select>
                    </FormControl>
                    <Box sx={{ display: 'flex', gap: 0.25 }}>
                        <Tooltip title="Refresh">
                            <IconButton
                                size="small"
                                onClick={onRefreshModels}
                                disabled={!provider.provider || !active}
                                sx={{ padding: 0.25 }}
                            >
                                <RefreshIcon fontSize="small" />
                            </IconButton>
                        </Tooltip>
                        <Tooltip title="Manual input">
                            <IconButton
                                size="small"
                                onClick={() => onUpdate('isManualInput', true)}
                                disabled={!active}
                                sx={{ padding: 0.25 }}
                            >
                                <EditIcon fontSize="small" />
                            </IconButton>
                        </Tooltip>
                    </Box>
                </Box>
            )}

            {/* Delete Button */}
            <Tooltip title="Remove">
                <IconButton
                    size="small"
                    onClick={onDelete}
                    color="error"
                    disabled={!active}
                    sx={{ padding: 0.5 }}
                >
                    <DeleteIcon fontSize="small" />
                </IconButton>
            </Tooltip>
        </Box>
    );
};

export default ProviderConfig;
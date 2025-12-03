import { Add, Cancel, CheckCircle, Delete, Edit } from '@mui/icons-material';
import {
    Box,
    Chip,
    IconButton,
    Stack,
    Typography,
} from '@mui/material';

export interface Provider {
    name: string;
    enabled: boolean;
    api_base: string;
    api_version: string; // "openai" or "anthropic", defaults to "openai"
    token?: string;
}

export interface ProviderModelsData {
    [providerName: string]: {
        models: string[];
    };
}

interface ProviderCardProps {
    provider: Provider;
    variant?: 'detailed' | 'simple';
    isDefault?: boolean;
    providerModels?: ProviderModelsData;
    onEdit?: (providerName: string) => void;
    onToggle?: (providerName: string) => void;
    onDelete?: (providerName: string) => void;
    onSetDefault?: (providerName: string) => void;
    onFetchModels?: (providerName: string) => void;
    onAdd?: () => void;
}

const ProviderCard = ({
    provider,
    variant = 'detailed',
    isDefault = false,
    providerModels,
    onEdit,
    onToggle,
    onDelete,
    onSetDefault,
    onFetchModels,
    onAdd,
}: ProviderCardProps) => {
    const models = providerModels?.[provider.name]?.models || [];
    const modelsCount = models.length;

    if (variant === 'simple') {
        return (
            <Box
                sx={{
                    width: '100%',
                    height: 120,
                    border: 1,
                    borderColor: isDefault
                        ? 'primary.main'
                        : provider.enabled
                            ? 'success.main'
                            : 'divider',
                    borderRadius: 1,
                    p: 2,
                    backgroundColor: isDefault
                        ? 'primary.50'
                        : provider.enabled
                            ? 'transparent'
                            : 'grey.50',
                    opacity: provider.enabled ? 1 : 0.7,
                    transition: 'all 0.2s ease-in-out',
                    '&:hover': {
                        boxShadow: 1,
                    }
                }}
            >
                <Stack spacing={1}>
                    <Stack direction="row" justifyContent="space-between" alignItems="center">
                        <Typography variant="body2" sx={{ fontWeight: 600, fontSize: '0.875rem' }}>
                            {provider.name}
                        </Typography>
                        <Chip
                            label={provider.enabled ? 'Enabled' : 'Disabled'}
                            color={provider.enabled ? 'success' : 'error'}
                            size="small"
                        />
                    </Stack>
                    <Typography variant="body2" color="text.secondary">
                        {modelsCount > 0 ? `${modelsCount} models` : 'No models loaded'}
                    </Typography>
                    <Stack direction="row" spacing={1}>
                        {!isDefault && onSetDefault ? (
                            <Chip
                                label="Set Default"
                                size="small"
                                variant="outlined"
                                clickable
                                onClick={() => onSetDefault(provider.name)}
                            />
                        ) : isDefault ? (
                            <Chip label="Default" color="primary" size="small" />
                        ) : null}
                        {onFetchModels && (
                            <Chip
                                label="Fetch Models"
                                size="small"
                                variant="outlined"
                                clickable
                                onClick={() => onFetchModels(provider.name)}
                            />
                        )}
                    </Stack>
                </Stack>
            </Box>
        );
    }

    return (
        <Box
            sx={{
                width: '100%',
                height: 240,
                border: 1,
                borderLeft: 4,
                borderColor: provider.enabled ? 'success.main' : 'error.main',
                borderRadius: 2,
                p: 2,
                backgroundColor: 'background.paper',
                opacity: provider.enabled ? 1 : 0.7,
                transition: 'all 0.2s ease-in-out',
                '&:hover': {
                    boxShadow: 1,
                }
            }}
        >
            <Stack spacing={2}>
                <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" alignItems={{ xs: 'flex-start', sm: 'center' }}>
                    <Stack direction="row" alignItems="center" spacing={1}>
                        <Typography variant="body2" sx={{ fontWeight: 600, fontSize: '0.9rem' }}>
                            {provider.name}
                        </Typography>
                        {provider.enabled ? (
                            <CheckCircle color="success" fontSize="small" />
                        ) : (
                            <Cancel color="error" fontSize="small" />
                        )}
                        <Chip
                            label={provider.enabled ? 'Enabled' : 'Disabled'}
                            color={provider.enabled ? 'success' : 'error'}
                            size="small"
                        />
                    </Stack>
                    <Stack direction="row" spacing={1}>
                        {onAdd && (
                            <IconButton
                                size="small"
                                color="primary"
                                onClick={onAdd}
                                title="Add New Provider"
                            >
                                <Add fontSize="small" />
                            </IconButton>
                        )}
                        {onEdit && (
                            <IconButton
                                size="small"
                                color="primary"
                                onClick={() => onEdit(provider.name)}
                            >
                                <Edit fontSize="small" />
                            </IconButton>
                        )}
                        {onToggle && (
                            <IconButton
                                size="small"
                                color={provider.enabled ? 'warning' : 'success'}
                                onClick={() => onToggle(provider.name)}
                            >
                                {provider.enabled ? <Cancel fontSize="small" /> : <CheckCircle fontSize="small" />}
                            </IconButton>
                        )}
                        {onDelete && (
                            <IconButton
                                size="small"
                                color="error"
                                onClick={() => onDelete(provider.name)}
                            >
                                <Delete fontSize="small" />
                            </IconButton>
                        )}
                    </Stack>
                </Stack>
                <Box>
                    <Typography variant="caption" color="text.secondary" gutterBottom>
                        API Base
                    </Typography>
                    <Typography variant="caption" sx={{ fontFamily: 'monospace', backgroundColor: 'grey.100', p: 0.5, borderRadius: 1, display: 'block', wordBreak: 'break-all' }}>
                        {provider.api_base}
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="caption" color="text.secondary" gutterBottom>
                        API Version
                    </Typography>
                    <Typography variant="caption" sx={{ fontFamily: 'monospace', backgroundColor: 'grey.100', p: 0.5, borderRadius: 1, display: 'block' }}>
                        {provider.api_version || 'openai'}
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="caption" color="text.secondary" gutterBottom>
                        API Token
                    </Typography>
                    <Typography variant="caption" sx={{ fontFamily: 'monospace', backgroundColor: 'grey.100', p: 0.5, borderRadius: 1, display: 'block' }}>
                        {provider.token ? `${provider.token.substring(0, 8)}...` : 'Not set'}
                    </Typography>
                </Box>
            </Stack>
        </Box>
    );
};

export default ProviderCard;
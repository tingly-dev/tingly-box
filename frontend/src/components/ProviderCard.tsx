import { Cancel, CheckCircle, ContentCopy, Delete, Edit, Visibility } from '@mui/icons-material';
import {
    Box,
    Chip,
    FormControlLabel,
    IconButton,
    Menu,
    MenuItem,
    Stack,
    Switch,
    Tooltip,
    Typography,
} from '@mui/material';
import { useState } from 'react';

export interface Provider {
    name: string;
    enabled: boolean;
    api_base: string;
    api_style: string; // "openai" or "anthropic", defaults to "openai"
    token?: string;
}

export interface ProviderModelsData {
    [providerName: string]: {
        models: string[];
        star_models?: string[];
        last_updated?: string;
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
}: ProviderCardProps) => {
    const models = providerModels?.[provider.name]?.models || [];
    const modelsCount = models.length;
    const [tokenMenuAnchor, setTokenMenuAnchor] = useState<null | HTMLElement>(null);
    const [showToken, setShowToken] = useState(false);

    const handleTokenMenuClick = (event: React.MouseEvent<HTMLElement>) => {
        setTokenMenuAnchor(event.currentTarget);
    };

    const handleTokenMenuClose = () => {
        setTokenMenuAnchor(null);
    };

    const handleViewToken = () => {
        setShowToken(true);
        handleTokenMenuClose();
    };

    const handleCopyToken = async () => {
        if (provider.token) {
            try {
                await navigator.clipboard.writeText(provider.token);
            } catch (err) {
                console.error('Failed to copy token:', err);
            }
        }
        handleTokenMenuClose();
    };

    if (variant === 'simple') {
        return (
            <Box
                sx={{
                    width: '100%',
                    height: 140,
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
                    display: 'flex',
                    flexDirection: 'column',
                    '&:hover': {
                        boxShadow: 1,
                    }
                }}
            >
                <Stack spacing={1} sx={{ flex: 1 }}>
                    <Stack direction="row" justifyContent="space-between" alignItems="center">
                        <Typography variant="body2" sx={{ fontWeight: 600, fontSize: '0.875rem' }}>
                            {provider.name}
                        </Typography>
                        <FormControlLabel
                            control={
                                <Switch
                                    checked={provider.enabled}
                                    onChange={() => onToggle?.(provider.name)}
                                    size="small"
                                    color="success"
                                />
                            }
                            label=""
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
                height: 280,
                border: 1,
                borderLeft: 4,
                borderColor: provider.enabled ? 'success.main' : 'error.main',
                borderRadius: 2,
                p: 2,
                backgroundColor: 'background.paper',
                opacity: provider.enabled ? 1 : 0.7,
                transition: 'all 0.2s ease-in-out',
                display: 'flex',
                flexDirection: 'column',
                '&:hover': {
                    boxShadow: 1,
                }
            }}
        >

            <Stack spacing={2} sx={{ flex: 1 }}>
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
                        <FormControlLabel
                            control={
                                <Switch
                                    checked={provider.enabled}
                                    onChange={() => onToggle?.(provider.name)}
                                    size="small"
                                    color="success"
                                />
                            }
                            label=""
                        />
                    </Stack>
                    <Stack direction="row" spacing={1}>
                        {onEdit && (
                            <IconButton
                                size="small"
                                color="primary"
                                onClick={() => onEdit(provider.name)}
                            >
                                <Edit fontSize="small" />
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
                        API Style
                    </Typography>
                    <Typography variant="caption" sx={{ fontFamily: 'monospace', backgroundColor: 'grey.100', p: 0.5, borderRadius: 1, display: 'block' }}>
                        {provider.api_style || 'openai'}
                    </Typography>
                </Box>
                <Box>
                    <Stack direction="row" justifyContent="space-between" alignItems="center">
                        <Typography variant="caption" color="text.secondary" gutterBottom>
                            API Token
                        </Typography>
                        {provider.token && (
                            <Stack direction="row" spacing={0.5}>
                                <Tooltip title="View Token">
                                    <IconButton
                                        size="small"
                                        onClick={handleViewToken}
                                        sx={{ p: 0.5 }}
                                    >
                                        <Visibility fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                                <Tooltip title="Copy Token">
                                    <IconButton
                                        size="small"
                                        onClick={handleCopyToken}
                                        sx={{ p: 0.5 }}
                                    >
                                        <ContentCopy fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                                <IconButton
                                    size="small"
                                    onClick={handleTokenMenuClick}
                                    sx={{ p: 0.5 }}
                                >
                                    <Typography variant="caption" sx={{ fontSize: '0.7rem' }}>
                                        •••
                                    </Typography>
                                </IconButton>
                            </Stack>
                        )}
                    </Stack>
                    <Typography variant="caption" sx={{ fontFamily: 'monospace', backgroundColor: 'grey.100', p: 0.5, borderRadius: 1, display: 'block', wordBreak: 'break-all' }}>
                        {showToken ? provider.token : (provider.token ? `${provider.token.substring(0, 8)}...` : 'Not set')}
                    </Typography>
                </Box>
            </Stack>

            {/* Token context menu */}
            <Menu
                anchorEl={tokenMenuAnchor}
                open={Boolean(tokenMenuAnchor)}
                onClose={handleTokenMenuClose}
                anchorOrigin={{
                    vertical: 'bottom',
                    horizontal: 'right',
                }}
                transformOrigin={{
                    vertical: 'top',
                    horizontal: 'right',
                }}
            >
                <MenuItem onClick={handleViewToken}>
                    <Visibility fontSize="small" sx={{ mr: 1 }} />
                    View Token
                </MenuItem>
                <MenuItem onClick={handleCopyToken}>
                    <ContentCopy fontSize="small" sx={{ mr: 1 }} />
                    Copy Token
                </MenuItem>
            </Menu>
        </Box>
    );
};

export default ProviderCard;
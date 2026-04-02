import { ApiStyleBadge } from '@/components/ApiStyleBadge.tsx';
import ModelListDialog from '@/components/ModelListDialog';
import ProviderExportMenu from '@/components/ProviderExportMenu';
import { exportProvider, exportProviderAsBase64ToClipboard, exportProviderAsJsonlToClipboard } from '@/components/rule-card/utils';
import { Delete, Edit, ListAlt, Refresh as RefreshIcon, Route, Schedule, VpnKey } from '@mui/icons-material';
import {
    Box,
    Button,
    Chip,
    CircularProgress,
    Divider,
    IconButton,
    Modal,
    Paper,
    Stack,
    Switch,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Tooltip,
    Typography,
} from '@mui/material';
import type { ExportFormat } from '@/components/rule-card/utils';
import {useCallback, useState} from 'react';
import type { Provider } from '../types/provider';

interface OAuthTableProps {
    providers: Provider[];
    onEdit?: (providerUuid: string) => void;
    onToggle?: (providerUuid: string) => void;
    onDelete?: (providerUuid: string) => void;
    onReauthorize?: (providerUuid: string) => void;
    onRefreshToken?: (providerUuid: string) => Promise<void>;
    onNotification?: (message: string, severity: 'success' | 'error') => void;
}

interface DeleteModalState {
    open: boolean;
    providerUuid: string;
    providerName: string;
}

interface RefreshModalState {
    open: boolean;
    providerUuid: string;
    providerName: string;
}

interface ModelListDialogState {
    open: boolean;
    provider: Provider | null;
}

const OAuthTable = ({ providers, onEdit, onToggle, onDelete, onReauthorize, onRefreshToken, onNotification }: OAuthTableProps) => {
    const [deleteModal, setDeleteModal] = useState<DeleteModalState>({
        open: false,
        providerUuid: '',
        providerName: '',
    });

    const [refreshModal, setRefreshModal] = useState<RefreshModalState>({
        open: false,
        providerUuid: '',
        providerName: '',
    });

    const [refreshing, setRefreshing] = useState<string | null>(null);

    const [modelListDialog, setModelListDialog] = useState<ModelListDialogState>({
        open: false,
        provider: null,
    });

    const handleDeleteClick = (providerUuid: string) => {
        const provider = providers.find((p) => p.uuid === providerUuid);
        setDeleteModal({
            open: true,
            providerUuid,
            providerName: provider?.name || 'Unknown Provider',
        });
    };

    const handleCloseDeleteModal = () => {
        setDeleteModal({ open: false, providerUuid: '', providerName: '' });
    };

    const handleConfirmDelete = () => {
        if (onDelete && deleteModal.providerUuid) {
            onDelete(deleteModal.providerUuid);
        }
        handleCloseDeleteModal();
    };

    const handleRefreshClick = (providerUuid: string) => {
        const provider = providers.find((p) => p.uuid === providerUuid);
        setRefreshModal({
            open: true,
            providerUuid,
            providerName: provider?.name || 'Unknown Provider',
        });
    };

    const handleCloseRefreshModal = () => {
        setRefreshModal({ open: false, providerUuid: '', providerName: '' });
    };

    const handleConfirmRefresh = async () => {
        if (!onRefreshToken || !refreshModal.providerUuid) return;

        setRefreshing(refreshModal.providerUuid);
        try {
            await onRefreshToken(refreshModal.providerUuid);
        } finally {
            setRefreshing(null);
        }
        handleCloseRefreshModal();
    };

    const handleModelListClick = (providerUuid: string) => {
        const provider = providers.find((p) => p.uuid === providerUuid);
        if (provider) {
            setModelListDialog({ open: true, provider });
        }
    };

    const handleCloseModelListDialog = () => {
        setModelListDialog({ open: false, provider: null });
    };

    const handleExportProvider = useCallback(async (provider: Provider, format: ExportFormat) => {
        await exportProvider(provider, format, (message, severity) => {
            onNotification?.(message, severity);
        });
    }, [onNotification]);

    const handleCopyProviderBase64 = useCallback(async (provider: Provider) => {
        await exportProviderAsBase64ToClipboard(provider, (message, severity) => {
            onNotification?.(message, severity);
        });
    }, [onNotification]);

    const handleCopyProviderJsonl = useCallback(async (provider: Provider) => {
        await exportProviderAsJsonlToClipboard(provider, (message, severity) => {
            onNotification?.(message, severity);
        });
    }, [onNotification]);

    const formatExpiresAt = (expiresAt?: string) => {
        if (!expiresAt) return 'Never';
        const date = new Date(expiresAt);
        const now = new Date();
        const isExpired = date < now;

        // Format as relative time
        const diffMs = date.getTime() - now.getTime();
        const diffMins = Math.floor(diffMs / 60000);
        const diffHours = Math.floor(diffMs / 3600000);
        const diffDays = Math.floor(diffMs / 86400000);

        if (isExpired) {
            return 'Expired';
        } else if (diffMins < 60) {
            return `in ${diffMins} min`;
        } else if (diffHours < 24) {
            return `in ${diffHours}h`;
        } else if (diffDays < 7) {
            return `in ${diffDays} days`;
        } else {
            // For longer periods, show date
            return date.toLocaleDateString();
        }
    };

    const getExpirationColor = (expiresAt?: string) => {
        if (!expiresAt) return 'default';
        const date = new Date(expiresAt);
        const now = new Date();
        const diffMs = date.getTime() - now.getTime();
        const diffHours = diffMs / 3600000;

        if (date < now) return 'error';
        if (diffHours < 1) return 'error';
        if (diffHours < 24) return 'warning';
        return 'success';
    };

    return (
        <TableContainer component={Paper} elevation={0} sx={{ border: 1, borderColor: 'divider' }}>
            <Table sx={{ tableLayout: 'fixed' }}>
                <TableHead>
                    <TableRow>
                        <TableCell sx={{ fontWeight: 600, width: 90 }}>Status</TableCell>
                        <TableCell sx={{ fontWeight: 600, width: 140 }}>Name</TableCell>
                        <TableCell sx={{ fontWeight: 600, width: 120 }}>API Style</TableCell>
                        <TableCell sx={{ fontWeight: 600, width: 120 }}>Provider</TableCell>
                        <TableCell sx={{ fontWeight: 600, width: 130 }}>Expires At</TableCell>
                        <TableCell sx={{ fontWeight: 600, width: 60 }}>Proxy</TableCell>
                        <TableCell sx={{ fontWeight: 600, width: 240 }}>Actions</TableCell>
                    </TableRow>
                </TableHead>
                <TableBody>
                    {providers.map((provider) => {
                        const expiresAt = provider.oauth_detail?.expires_at;
                        const isExpired = expiresAt ? new Date(expiresAt) < new Date() : false;

                        return (
                            <TableRow key={provider.uuid}>
                                {/* Status */}
                                <TableCell>
                                    <Stack direction="row" alignItems="center" spacing={1}>
                                        <Switch
                                            checked={provider.enabled}
                                            onChange={() => onToggle?.(provider.uuid)}
                                            size="small"
                                            color="success"
                                        />
                                        <Chip
                                            label={provider.enabled ? 'On' : 'Off'}
                                            size="small"
                                            color={provider.enabled ? 'success' : 'default'}
                                            variant={provider.enabled ? 'filled' : 'outlined'}
                                            sx={{ height: 22, fontSize: '0.7rem', minWidth: 40 }}
                                        />
                                    </Stack>
                                </TableCell>
                                {/* Name */}
                                <TableCell>
                                    <Stack direction="row" alignItems="center" spacing={1}>
                                        <Typography variant="body2" sx={{ fontWeight: 500, minWidth: 120 }}>
                                            {provider.name}
                                        </Typography>
                                    </Stack>
                                </TableCell>
                                {/* API Style */}
                                <TableCell>
                                    <ApiStyleBadge sx={{ minWidth: '110px' }} apiStyle={provider.api_style} />
                                </TableCell>
                                {/* Provider Type */}
                                <TableCell>
                                    <Typography variant="body2" sx={{ textTransform: 'capitalize' }}>
                                        {provider.oauth_detail?.provider_type || 'N/A'}
                                    </Typography>
                                </TableCell>
                                {/* Expires At */}
                                <TableCell>
                                    <Stack direction="row" alignItems="center" spacing={1}>
                                        <Schedule fontSize="small" color={getExpirationColor(expiresAt) as any} />
                                        <Typography variant="body2" color={getExpirationColor(expiresAt) + '.main' as any}>
                                            {formatExpiresAt(expiresAt)}
                                        </Typography>
                                        {isExpired && (
                                            <Chip label="Expired" color="error" size="small" sx={{ height: 20, fontSize: '0.7rem' }} />
                                        )}
                                    </Stack>
                                </TableCell>
                                {/* Proxy */}
                                <TableCell align="center">
                                    {provider.proxy_url ? (
                                        <Tooltip title={provider.proxy_url} arrow>
                                            <Route fontSize="small" sx={{ color: 'text.secondary' }} />
                                        </Tooltip>
                                    ) : (
                                        <Typography variant="body2" color="text.secondary">
                                            -
                                        </Typography>
                                    )}
                                </TableCell>
                                {/* Actions */}
                                <TableCell sx={{ whiteSpace: 'nowrap' }}>
                                    <Box
                                        sx={{
                                            display: 'flex',
                                            alignItems: 'center',
                                            gap: 0.5,
                                            border: 1,
                                            borderColor: 'divider',
                                            borderRadius: 1.5,
                                            p: 0.5,
                                            pr: 1,
                                            width: 240,
                                        }}
                                    >
                                        <ProviderExportMenu
                                            provider={provider}
                                            onExport={handleExportProvider}
                                            onCopyJsonl={handleCopyProviderJsonl}
                                            onCopyBase64={handleCopyProviderBase64}
                                        />
                                        {onEdit && (
                                            <Tooltip title="View Details">
                                                <IconButton size="small" color="primary" onClick={() => onEdit(provider.uuid)}>
                                                    <Edit fontSize="small" />
                                                </IconButton>
                                            </Tooltip>
                                        )}
                                        {onRefreshToken && provider.oauth_detail?.refresh_token && (
                                            <Tooltip title="Refresh Token">
                                                <IconButton
                                                    size="small"
                                                    color="info"
                                                    onClick={() => handleRefreshClick(provider.uuid)}
                                                    disabled={refreshing === provider.uuid}
                                                >
                                                    {refreshing === provider.uuid ? (
                                                        <CircularProgress size={16} />
                                                    ) : (
                                                        <RefreshIcon fontSize="small" />
                                                    )}
                                                </IconButton>
                                            </Tooltip>
                                        )}
                                        {onReauthorize && (
                                            <Tooltip title="Reauthorize">
                                                <IconButton
                                                    size="small"
                                                    color={isExpired ? 'warning' : 'default'}
                                                    onClick={() => onReauthorize(provider.uuid)}
                                                >
                                                    <VpnKey fontSize="small" />
                                                </IconButton>
                                            </Tooltip>
                                        )}
                                        {onDelete && (
                                            <Tooltip title="Delete">
                                                <IconButton size="small" color="error" onClick={() => handleDeleteClick(provider.uuid)}>
                                                    <Delete fontSize="small" />
                                                </IconButton>
                                            </Tooltip>
                                        )}
                                        <Divider orientation="vertical" flexItem />
                                        <Button
                                            variant="text"
                                            size="small"
                                            startIcon={<ListAlt />}
                                            onClick={() => handleModelListClick(provider.uuid)}
                                            disabled={!provider.enabled}
                                            sx={{
                                                textTransform: 'none',
                                                fontSize: '0.75rem',
                                                minWidth: 'auto',
                                                px: 1,
                                                color: 'text.primary',
                                            }}
                                        >
                                            Models
                                        </Button>
                                    </Box>
                                </TableCell>
                            </TableRow>
                        );
                    })}
                </TableBody>
            </Table>

            {/* Delete Confirmation Modal */}
            <Modal open={deleteModal.open} onClose={handleCloseDeleteModal}>
                <Box
                    sx={{
                        position: 'absolute',
                        top: '50%',
                        left: '50%',
                        transform: 'translate(-50%, -50%)',
                        width: 400,
                        maxWidth: '80vw',
                        bgcolor: 'background.paper',
                        boxShadow: 24,
                        p: 4,
                        borderRadius: 2,
                    }}
                >
                    <Typography variant="h6" sx={{ mb: 2 }}>Delete OAuth Provider</Typography>
                    <Typography variant="body2" sx={{ mb: 3 }}>
                        Are you sure you want to delete the OAuth provider "{deleteModal.providerName}"? This action cannot be undone.
                    </Typography>
                    <Stack direction="row" spacing={2} justifyContent="flex-end">
                        <Button onClick={handleCloseDeleteModal} color="inherit">
                            Cancel
                        </Button>
                        <Button onClick={handleConfirmDelete} color="error" variant="contained">
                            Delete
                        </Button>
                    </Stack>
                </Box>
            </Modal>

            {/* Refresh Token Confirmation Modal */}
            <Modal open={refreshModal.open} onClose={handleCloseRefreshModal}>
                <Box
                    sx={{
                        position: 'absolute',
                        top: '50%',
                        left: '50%',
                        transform: 'translate(-50%, -50%)',
                        width: 400,
                        maxWidth: '80vw',
                        bgcolor: 'background.paper',
                        boxShadow: 24,
                        p: 4,
                        borderRadius: 2,
                    }}
                >
                    <Typography variant="h6" sx={{ mb: 2 }}>Refresh OAuth Token</Typography>
                    <Typography variant="body2" sx={{ mb: 3 }}>
                        Are you sure you want to refresh the OAuth token for "{refreshModal.providerName}"? This will update the access token using the refresh token.
                    </Typography>
                    <Stack direction="row" spacing={2} justifyContent="flex-end">
                        <Button onClick={handleCloseRefreshModal} color="inherit" disabled={refreshing !== null}>
                            Cancel
                        </Button>
                        <Button
                            onClick={handleConfirmRefresh}
                            color="info"
                            variant="contained"
                            disabled={refreshing !== null}
                            startIcon={refreshing !== null ? <CircularProgress size={16} /> : <RefreshIcon fontSize="small" />}
                        >
                            {refreshing !== null ? 'Refreshing...' : 'Refresh'}
                        </Button>
                    </Stack>
                </Box>
            </Modal>

            {/* Model List Dialog */}
            <ModelListDialog
                open={modelListDialog.open}
                onClose={handleCloseModelListDialog}
                provider={modelListDialog.provider}
            />
        </TableContainer>
    );
};

export default OAuthTable;

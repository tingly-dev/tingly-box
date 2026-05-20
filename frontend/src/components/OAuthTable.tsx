import { ApiStyleBadge } from '@/components/ApiStyleBadge.tsx';
import ModelListDialog from '@/components/ModelListDialog';
import { exportProvider, exportProviderAsBase64ToClipboard, exportProviderAsJsonlToClipboard } from '@/components/rule-card/utils';
import { ProviderQuotaDetailRow } from '@/components/credential/ProviderQuotaDetailRow';
import { ContentCopy, DataUsage, Delete, Download, Edit, ListAlt, MoreVert, Refresh as RefreshIcon, Route, Schedule, VpnKey } from '@mui/icons-material';
import {
    Box,
    Button,
    Chip,
    CircularProgress,
    Divider,
    IconButton,
    Menu,
    MenuItem,
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
import type { ProviderQuota } from '@/types/quota';
import React, {useCallback, useState} from 'react';
import type { Provider } from '../types/provider';

interface OAuthTableProps {
    providers: Provider[];
    onEdit?: (providerUuid: string) => void;
    onToggle?: (providerUuid: string) => void;
    onDelete?: (providerUuid: string) => void;
    onReauthorize?: (providerUuid: string) => void;
    onRefreshToken?: (providerUuid: string) => Promise<void>;
    onNotification?: (message: string, severity: 'success' | 'error') => void;
    providerQuotas?: { [uuid: string]: ProviderQuota };
    refreshingQuotas?: Set<string>;
    onQuotaRefresh?: (providerUuid: string) => void;
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

const OAuthTable = ({ providers, onEdit, onToggle, onDelete, onReauthorize, onRefreshToken, onNotification, providerQuotas, refreshingQuotas, onQuotaRefresh }: OAuthTableProps) => {
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
    const [moreMenu, setMoreMenu] = useState<{ anchorEl: HTMLElement | null; providerUuid: string }>({
        anchorEl: null,
        providerUuid: '',
    });

    const handleMoreOpen = (e: React.MouseEvent<HTMLElement>, providerUuid: string) => {
        e.stopPropagation();
        setMoreMenu({ anchorEl: e.currentTarget, providerUuid });
    };
    const handleMoreClose = () => setMoreMenu({ anchorEl: null, providerUuid: '' });

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
                        <TableCell sx={{ fontWeight: 600, width: 140 }}>API Style</TableCell>
                        <TableCell sx={{ fontWeight: 600, width: 200 }}>Provider</TableCell>
                        <TableCell sx={{ fontWeight: 600, width: 140 }}>Expires At</TableCell>
                        <TableCell sx={{ fontWeight: 600, width: 60 }}>Proxy</TableCell>
                        <TableCell sx={{ fontWeight: 600, width: 200 }}>Actions</TableCell>
                    </TableRow>
                </TableHead>
                <TableBody>
                    {providers.map((provider) => {
                        const expiresAt = provider.oauth_detail?.expires_at;
                        const isExpired = expiresAt ? new Date(expiresAt) < new Date() : false;

                        return (
                            <React.Fragment key={provider.uuid}>
                                {/* Main provider row */}
                                <TableRow>
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
                                        {provider.oauth_detail?.issuer || 'N/A'}
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
                                            width: 'fit-content',
                                        }}
                                    >
                                        {/* Edit — primary action */}
                                        {onEdit && (
                                            <Tooltip title="View Details">
                                                <IconButton size="small" color="primary" onClick={() => onEdit(provider.uuid)}>
                                                    <Edit fontSize="small" />
                                                </IconButton>
                                            </Tooltip>
                                        )}
                                        <Divider orientation="vertical" flexItem />
                                        {/* Quota text button */}
                                        {onQuotaRefresh && (
                                            <Button
                                                variant="text"
                                                size="small"
                                                startIcon={refreshingQuotas?.has(provider.uuid)
                                                    ? <CircularProgress size={12} />
                                                    : <DataUsage fontSize="small" />}
                                                onClick={() => onQuotaRefresh(provider.uuid)}
                                                disabled={refreshingQuotas?.has(provider.uuid)}
                                                color={providerQuotas?.[provider.uuid] ? 'primary' : 'inherit'}
                                                sx={{ fontSize: '0.75rem', minWidth: 'auto', px: 1 }}
                                            >
                                                Quota
                                            </Button>
                                        )}
                                        {/* Models text button */}
                                        <Button
                                            variant="text"
                                            size="small"
                                            startIcon={<ListAlt />}
                                            onClick={() => handleModelListClick(provider.uuid)}
                                            disabled={!provider.enabled}
                                            sx={{ fontSize: '0.75rem', minWidth: 'auto', px: 1 }}
                                        >
                                            Models
                                        </Button>
                                        <Divider orientation="vertical" flexItem />
                                        {/* Overflow menu */}
                                        <IconButton size="small" onClick={(e) => handleMoreOpen(e, provider.uuid)}>
                                            <MoreVert fontSize="small" />
                                        </IconButton>
                                    </Box>
                                </TableCell>
                            </TableRow>

                    {/* Quota detail row */}
                    {providerQuotas && onQuotaRefresh && (
                        <ProviderQuotaDetailRow
                            provider={provider}
                            quota={providerQuotas[provider.uuid]}
                            isRefreshing={refreshingQuotas?.has(provider.uuid) || false}
                            onRefresh={onQuotaRefresh}
                        />
                    )}
                </React.Fragment>
                );
                    })}
                </TableBody>
            </Table>

            {/* Overflow menu (shared) */}
            <Menu
                anchorEl={moreMenu.anchorEl}
                open={Boolean(moreMenu.anchorEl)}
                onClose={handleMoreClose}
                onClick={(e) => e.stopPropagation()}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                transformOrigin={{ vertical: 'top', horizontal: 'right' }}
            >
                {(() => {
                    const p = providers.find(p => p.uuid === moreMenu.providerUuid);
                    if (!p) return null;
                    const hasRefreshToken = onRefreshToken && p.oauth_detail?.refresh_token;
                    const expired = p.oauth_detail?.expires_at
                        ? new Date(p.oauth_detail.expires_at) < new Date()
                        : false;
                    return [
                        hasRefreshToken && (
                            <MenuItem key="refresh-token" onClick={() => { handleMoreClose(); handleRefreshClick(p.uuid); }}
                                disabled={refreshing === p.uuid}>
                                {refreshing === p.uuid
                                    ? <CircularProgress size={14} sx={{ mr: 1 }} />
                                    : <RefreshIcon fontSize="small" sx={{ mr: 1 }} />}
                                Refresh Token
                            </MenuItem>
                        ),
                        onReauthorize && (
                            <MenuItem key="reauthorize" onClick={() => { handleMoreClose(); onReauthorize(p.uuid); }}
                                sx={{ color: expired ? 'warning.main' : undefined }}>
                                <VpnKey fontSize="small" sx={{ mr: 1 }} /> Reauthorize
                            </MenuItem>
                        ),
                        <Divider key="div1" />,
                        <MenuItem key="export-jsonl" onClick={() => { handleMoreClose(); handleExportProvider(p, 'jsonl'); }}>
                            <Download fontSize="small" sx={{ mr: 1 }} /> Download JSONL
                        </MenuItem>,
                        <MenuItem key="export-base64" onClick={() => { handleMoreClose(); handleExportProvider(p, 'base64'); }}>
                            <Download fontSize="small" sx={{ mr: 1 }} /> Download Base64
                        </MenuItem>,
                        <MenuItem key="copy-jsonl" onClick={() => { handleMoreClose(); handleCopyProviderJsonl(p); }}>
                            <ContentCopy fontSize="small" sx={{ mr: 1 }} /> Copy JSONL
                        </MenuItem>,
                        <MenuItem key="copy-base64" onClick={() => { handleMoreClose(); handleCopyProviderBase64(p); }}>
                            <ContentCopy fontSize="small" sx={{ mr: 1 }} /> Copy Base64
                        </MenuItem>,
                        onDelete && <Divider key="div2" />,
                        onDelete && (
                            <MenuItem key="delete" onClick={() => { handleMoreClose(); handleDeleteClick(p.uuid); }} sx={{ color: 'error.main' }}>
                                <Delete fontSize="small" sx={{ mr: 1 }} /> Delete
                            </MenuItem>
                        ),
                    ].filter(Boolean);
                })()}
            </Menu>

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

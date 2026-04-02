import { ApiStyleBadge } from '@/components/ApiStyleBadge.tsx';
import ModelListDialog from '@/components/ModelListDialog';
import ProviderExportMenu from '@/components/ProviderExportMenu';
import { exportProvider, exportProviderAsBase64ToClipboard, exportProviderAsJsonlToClipboard } from '@/components/rule-card/utils';
import { Cancel, ContentCopy, Delete, Edit, ListAlt, Route, Visibility } from '@mui/icons-material';
import {
    Box,
    Button,
    Chip,
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
import api from '../services/api';
import type { Provider } from '../types/provider';

interface ApiKeyTableProps {
    providers: Provider[];
    onEdit?: (providerUuid: string) => void;
    onToggle?: (providerUuid: string) => void;
    onDelete?: (providerUuid: string) => void;
    onNotification?: (message: string, severity: 'success' | 'error') => void;
}

interface TokenModalState {
    open: boolean;
    providerName: string;
    token: string;
    loading: boolean;
}

interface DeleteModalState {
    open: boolean;
    providerUuid: string;
    providerName: string;
}

interface ModelListDialogState {
    open: boolean;
    provider: Provider | null;
}

const ApiKeyTable = ({ providers, onEdit, onToggle, onDelete, onNotification }: ApiKeyTableProps) => {
    const [tokenModal, setTokenModal] = useState<TokenModalState>({
        open: false,
        providerName: '',
        token: '',
        loading: false,
    });
    const [deleteModal, setDeleteModal] = useState<DeleteModalState>({
        open: false,
        providerUuid: '',
        providerName: '',
    });
    const [modelListDialog, setModelListDialog] = useState<ModelListDialogState>({
        open: false,
        provider: null,
    });

    const fetchFullToken = async (providerUuid: string): Promise<string> => {
        try {
            const response = await api.getProvider(providerUuid);
            if (!response.success) {
                throw new Error(`Failed to fetch token for provider ${providerUuid}`);
            }
            return response.data.token || '';
        } catch (error) {
            console.error('Error fetching full token:', error);
            throw error;
        }
    };

    const handleViewToken = async (providerUuid: string) => {
        setTokenModal({
            open: true,
            providerName: '',
            token: '',
            loading: true,
        });

        try {
            const fullToken = await fetchFullToken(providerUuid);
            const providerResponse = await api.getProvider(providerUuid);
            if (providerResponse.success) {
                setTokenModal({
                    open: true,
                    providerName: providerResponse.data.name,
                    token: fullToken,
                    loading: false,
                });
            }
        } catch (error) {
            console.error('Failed to fetch token:', error);
            setTokenModal({
                open: true,
                providerName: '',
                token: '',
                loading: false,
            });
        }
    };

    const handleCloseTokenModal = () => {
        setTokenModal({ open: false, providerName: '', token: '', loading: false });
    };

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

    const formatTokenDisplay = (provider: Provider) => {
        if (!provider.token) return 'Not set';
        if (provider.token.length <= 12) return provider.token;
        const prefix = provider.token.substring(0, 4);
        const suffix = provider.token.substring(provider.token.length - 4);
        return `${prefix}${'*'.repeat(4)}${suffix}`;
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

    return (
        <TableContainer component={Paper} elevation={0} sx={{ border: 1, borderColor: 'divider' }}>
            <Table sx={{ tableLayout: 'fixed' }}>
                <TableHead>
                    <TableRow>
                        <TableCell sx={{ fontWeight: 600, width: 90 }}>Status</TableCell>
                        <TableCell sx={{ fontWeight: 600, width: 140 }}>Name</TableCell>
                        <TableCell sx={{ fontWeight: 600, width: 140 }}>API Style</TableCell>
                        <TableCell sx={{ fontWeight: 600, width: 200 }}>API Base URL</TableCell>
                        <TableCell sx={{ fontWeight: 600, width: 140 }}>API Key</TableCell>
                        <TableCell sx={{ fontWeight: 600, width: 60 }}>Proxy</TableCell>
                        <TableCell sx={{ fontWeight: 600, width: 200 }}>Actions</TableCell>
                    </TableRow>
                </TableHead>
                <TableBody>
                    {providers.map((provider) => (
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
                                <Tooltip title={provider.name} arrow placement="top">
                                    <Typography
                                        variant="body2"
                                        sx={{
                                            fontWeight: 500,
                                            maxWidth: 120,
                                            overflow: 'hidden',
                                            textOverflow: 'ellipsis',
                                            whiteSpace: 'nowrap',
                                        }}
                                    >
                                        {provider.name}
                                    </Typography>
                                </Tooltip>
                            </TableCell>
                            {/* API Style */}
                            <TableCell>
                                <ApiStyleBadge sx={{ minWidth: '110px' }} apiStyle={provider.api_style} />
                            </TableCell>
                            {/* API Base URL */}
                            <TableCell>
                                <Typography variant="body2" sx={{
                                    maxWidth: 150,
                                    fontFamily: 'monospace', wordBreak: 'break-all',
                                }}>
                                    {provider.api_base}
                                </Typography>
                            </TableCell>
                            {/* API Key */}
                            <TableCell>
                                <Stack direction="row" alignItems="center" spacing={1}>
                                    {provider.token && (
                                        <Tooltip title="View Token">
                                            <IconButton size="small" onClick={() => handleViewToken(provider.uuid)} sx={{ p: 0.25 }}>
                                                <Visibility fontSize="small" />
                                            </IconButton>
                                        </Tooltip>
                                    )}
                                    <Typography
                                        variant="body2"
                                        sx={{
                                            fontFamily: 'monospace',
                                            wordBreak: 'break-all',
                                            flex: 1,
                                            minWidth: 0,
                                        }}
                                    >
                                        {formatTokenDisplay(provider)}
                                    </Typography>
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
                            <TableCell>
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
                                        width: 200,
                                    }}
                                >
                                    <ProviderExportMenu
                                        provider={provider}
                                        onExport={handleExportProvider}
                                        onCopyJsonl={handleCopyProviderJsonl}
                                        onCopyBase64={handleCopyProviderBase64}
                                    />
                                    {onEdit && (
                                        <Tooltip title="Edit">
                                            <IconButton size="small" color="primary" onClick={() => onEdit(provider.uuid)}>
                                                <Edit fontSize="small" />
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
                    ))}
                </TableBody>
            </Table>

            {/* Token View Modal */}
            <Modal open={tokenModal.open} onClose={handleCloseTokenModal}>
                <Box
                    sx={{
                        position: 'absolute',
                        top: '50%',
                        left: '50%',
                        transform: 'translate(-50%, -50%)',
                        width: 600,
                        maxWidth: '80vw',
                        bgcolor: 'background.paper',
                        boxShadow: 24,
                        p: 4,
                        borderRadius: 2,
                    }}
                >
                    <Typography variant="h6" sx={{ mb: 2 }}>
                        {tokenModal.token ? `API Key - ${tokenModal.providerName}` : tokenModal.providerName}
                    </Typography>

                    {tokenModal.loading ? (
                        <Box sx={{ mb: 3, textAlign: 'center', py: 4 }}>
                            <Typography variant="body2" color="text.secondary">Loading API key...</Typography>
                        </Box>
                    ) : tokenModal.token ? (
                        <Box sx={{ mb: 3 }}>
                            <Box
                                sx={{
                                    p: 2,
                                    bgcolor: 'action.hover',
                                    borderRadius: 1,
                                    fontFamily: 'monospace',
                                    fontSize: '0.875rem',
                                    wordBreak: 'break-all',
                                    border: '1px solid',
                                    borderColor: 'divider',
                                }}
                            >
                                {tokenModal.token}
                            </Box>
                        </Box>
                    ) : null}

                    <Stack direction="row" spacing={2} justifyContent="flex-end">
                        <IconButton
                            color="primary"
                            disabled={tokenModal.loading || !tokenModal.token}
                            onClick={async () => {
                                if (tokenModal.token) {
                                    try {
                                        await navigator.clipboard.writeText(tokenModal.token);
                                    } catch (err) {
                                        console.error('Failed to copy token:', err);
                                    }
                                }
                            }}
                            title={tokenModal.loading ? 'Loading...' : 'Copy Token'}
                        >
                            <ContentCopy />
                        </IconButton>
                        <Tooltip title="Close">
                            <IconButton onClick={handleCloseTokenModal}>
                                <Cancel />
                            </IconButton>
                        </Tooltip>
                    </Stack>
                </Box>
            </Modal>

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
                    <Typography variant="h6" sx={{ mb: 2 }}>Delete Provider</Typography>
                    <Typography variant="body2" sx={{ mb: 3 }}>
                        Are you sure you want to delete the provider "{deleteModal.providerName}"? This action cannot be undone.
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

            {/* Model List Dialog */}
            <ModelListDialog
                open={modelListDialog.open}
                onClose={handleCloseModelListDialog}
                provider={modelListDialog.provider}
            />
        </TableContainer>
    );
};

export default ApiKeyTable;

import { Cancel, CheckCircle, ContentCopy, Delete, Edit, ListAlt, Visibility } from '@mui/icons-material';
import {
    Box,
    Button,
    FormControlLabel,
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
import { useState } from 'react';
import api from '../services/api';
import type { Provider } from '../types/provider';
import { ApiStyleBadge } from '@/components/ApiStyleBadge.tsx';
import ModelListDialog from '@/components/ModelListDialog';

interface ApiKeyTableProps {
    providers: Provider[];
    onEdit?: (providerUuid: string) => void;
    onToggle?: (providerUuid: string) => void;
    onDelete?: (providerUuid: string) => void;
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

const ApiKeyTable = ({ providers, onEdit, onToggle, onDelete }: ApiKeyTableProps) => {
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

    return (
        <TableContainer component={Paper} elevation={0} sx={{ border: 1, borderColor: 'divider' }}>
            <Table>
                <TableHead>
                    <TableRow>
                        <TableCell sx={{ fontWeight: 600, minWidth: 150 }}>Name</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 150 }}>API Key</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 200 }}>API Base URL</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 120 }}>API Style</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 120 }}>Actions</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 120 }}>Model List</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 120 }}>Status</TableCell>
                    </TableRow>
                </TableHead>
                <TableBody>
                    {providers.map((provider) => (
                        <TableRow key={provider.uuid}>
                            <TableCell>
                                <Stack direction="row" alignItems="center" spacing={1}>
                                    {provider.enabled ? (
                                        <CheckCircle color="success" fontSize="small" />
                                    ) : (
                                        <Cancel color="error" fontSize="small" />
                                    )}
                                    <Typography variant="body2" sx={{ fontWeight: 500 }}>
                                        {provider.name}
                                    </Typography>
                                </Stack>
                            </TableCell>
                            <TableCell>
                                <Stack direction="row" alignItems="center" spacing={1}>
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
                                    {provider.token && (
                                        <Tooltip title="View Token">
                                            <IconButton size="small" onClick={() => handleViewToken(provider.uuid)} sx={{ p: 0.25 }}>
                                                <Visibility fontSize="small" />
                                            </IconButton>
                                        </Tooltip>
                                    )}
                                </Stack>
                            </TableCell>
                            <TableCell>
                                <Typography variant="body2" sx={{ fontFamily: 'monospace', wordBreak: 'break-all', maxWidth: 200 }}>
                                    {provider.api_base}
                                </Typography>
                            </TableCell>
                            <TableCell>
                                <ApiStyleBadge sx={{ minWidth: '110px' }} apiStyle={provider.api_style} />
                            </TableCell>
                            <TableCell>
                                <Stack direction="row" spacing={0.5}>
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
                                </Stack>
                            </TableCell>
                            <TableCell>
                                <Button
                                    variant="outlined"
                                    size="small"
                                    startIcon={<ListAlt />}
                                    onClick={() => handleModelListClick(provider.uuid)}
                                    disabled={!provider.enabled}
                                    sx={{
                                        textTransform: 'none',
                                        borderRadius: 1.5,
                                        fontSize: '0.8rem',
                                    }}
                                >
                                    Models
                                </Button>
                            </TableCell>
                            <TableCell>
                                <Stack direction="row" alignItems="center" spacing={1}>
                                    <FormControlLabel
                                        control={
                                            <Switch
                                                checked={provider.enabled}
                                                onChange={() => onToggle?.(provider.uuid)}
                                                size="small"
                                                color="success"
                                            />
                                        }
                                        label=""
                                    />
                                    <Typography variant="body2" color={provider.enabled ? 'success.main' : 'error.main'}>
                                        {provider.enabled ? 'Enabled' : 'Disabled'}
                                    </Typography>
                                </Stack>
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
                    <Typography variant="h6" sx={{ mb: 2 }}>API Key - {tokenModal.providerName}</Typography>

                    {tokenModal.loading ? (
                        <Box sx={{ mb: 3, textAlign: 'center', py: 4 }}>
                            <Typography variant="body2" color="text.secondary">Loading API key...</Typography>
                        </Box>
                    ) : (
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
                                {tokenModal.token || 'Failed to load token'}
                            </Box>
                        </Box>
                    )}

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

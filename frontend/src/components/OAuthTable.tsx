import { Cancel, CheckCircle, Delete, Edit, VpnKey as RefreshToken, Schedule, VpnKey } from '@mui/icons-material';
import {
    Box,
    Button,
    Chip,
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
import type { Provider } from '../types/provider';
import { ApiStyleBadge } from '@/components/ApiStyleBadge.tsx';

interface OAuthTableProps {
    providers: Provider[];
    onEdit?: (providerUuid: string) => void;
    onToggle?: (providerUuid: string) => void;
    onDelete?: (providerUuid: string) => void;
    onReauthorize?: (providerUuid: string) => void;
}

interface DeleteModalState {
    open: boolean;
    providerUuid: string;
    providerName: string;
}

const OAuthTable = ({ providers, onEdit, onToggle, onDelete, onReauthorize }: OAuthTableProps) => {
    const [deleteModal, setDeleteModal] = useState<DeleteModalState>({
        open: false,
        providerUuid: '',
        providerName: '',
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
            <Table>
                <TableHead>
                    <TableRow>
                        <TableCell sx={{ fontWeight: 600, minWidth: 150 }}>Name</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 120 }}>Provider Type</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 120 }}>Expires At</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 120 }}>API Style</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 140 }}>Actions</TableCell>
                        <TableCell sx={{ fontWeight: 600, minWidth: 120 }}>Status</TableCell>
                    </TableRow>
                </TableHead>
                <TableBody>
                    {providers.map((provider) => {
                        const expiresAt = provider.oauth_detail?.expires_at;
                        const isExpired = expiresAt ? new Date(expiresAt) < new Date() : false;

                        return (
                            <TableRow key={provider.uuid}>
                                <TableCell>
                                    <Stack direction="row" alignItems="center" spacing={1}>
                                        {provider.enabled ? (
                                            <CheckCircle color="success" fontSize="small" />
                                        ) : (
                                            <Cancel color="error" fontSize="small" />
                                        )}
                                         <Chip
                                            // icon={<VpnKey fontSize="small" sx={{ fontSize: 14 }} />}
                                            label="OAuth"
                                            size="small"
                                            color="primary"
                                            variant="outlined"
                                            sx={{ height: 20, fontSize: '0.7rem', '& .MuiChip-label': { px: 0.5 } }}
                                        />
                                        <Typography variant="body2" sx={{ fontWeight: 500 }}>
                                            {provider.name}
                                        </Typography>
                                    </Stack>
                                </TableCell>
                                <TableCell>
                                    <Typography variant="body2" sx={{ textTransform: 'capitalize' }}>
                                        {provider.oauth_detail?.provider_type || 'N/A'}
                                    </Typography>
                                </TableCell>
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
                                <TableCell>
                                    <ApiStyleBadge sx={{ minWidth: '110px' }} apiStyle={provider.api_style} />
                                </TableCell>
                                <TableCell>
                                    <Stack direction="row" spacing={0.5}>
                                        {onEdit && (
                                            <Tooltip title="View Details">
                                                <IconButton size="small" color="primary" onClick={() => onEdit(provider.uuid)}>
                                                    <Edit fontSize="small" />
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
                                                    <RefreshToken fontSize="small" />
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
        </TableContainer>
    );
};

export default OAuthTable;

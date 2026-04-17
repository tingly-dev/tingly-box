import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import {
    IconKey,
    IconPlus,
    IconTrash,
    IconCopy,
    IconCheck,
    IconClock,
    IconUser,
    IconShield,
    IconInfoCircle,
    IconEye,
    IconEyeOff,
} from '@tabler/icons-react';
import {
    Box,
    Button,
    Chip,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    TextField,
    Typography,
    IconButton,
    Tooltip,
    Alert,
} from '@mui/material';
import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';

interface APIToken {
    token_id: string;
    user_uuid: string;
    display_name: string;
    enabled: boolean;
    last_used_at?: string;
    created_at: string;
    created_by?: string;
}

const APITokensPage = () => {
    const { t } = useTranslation();
    const [tokens, setTokens] = useState<APIToken[]>([]);
    const [loading, setLoading] = useState(true);
    const [notification, setNotification] = useState<{
        open: boolean;
        message?: string;
        severity?: 'success' | 'error' | 'info' | 'warning';
    }>({ open: false });

    // Create token dialog state
    const [createDialogOpen, setCreateDialogOpen] = useState(false);
    const [newTokenDisplayName, setNewTokenDisplayName] = useState('');
    const [creatingToken, setCreatingToken] = useState(false);


    // Delete confirm dialog
    const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
    const [tokenToDelete, setTokenToDelete] = useState<APIToken | null>(null);
    const [deletingToken, setDeletingToken] = useState(false);

    // Token visibility state - map of token_id -> boolean
    const [visibleTokens, setVisibleTokens] = useState<Record<string, boolean>>({});

    const toggleTokenVisibility = (tokenId: string) => {
        setVisibleTokens(prev => ({
            ...prev,
            [tokenId]: !prev[tokenId]
        }));
    };

    const maskToken = (token: string): string => {
        if (!token) return '';
        if (token.length <= 16) {
            return `${token.slice(0, 8)}...${token.slice(-4)}`;
        }
        return `${token.slice(0, 12)}...${token.slice(-4)}`;
    };

    useEffect(() => {
        loadTokens();
    }, []);

    const loadTokens = async () => {
        setLoading(true);
        const result = await api.listAPITokens();
        if (result.success && result.data) {
            setTokens(result.data.tokens || []);
        }
        setLoading(false);
    };

    const handleCreateToken = async () => {
        if (!newTokenDisplayName) {
            setNotification({
                open: true,
                message: 'Display Name is required',
                severity: 'error',
            });
            return;
        }

        setCreatingToken(true);
        const result = await api.createAPIToken({
            display_name: newTokenDisplayName,
        });

        setCreatingToken(false);
        setCreateDialogOpen(false);

        if (result.success && result.data) {
            setNotification({
                open: true,
                message: 'Token created successfully',
                severity: 'success',
            });
            loadTokens();

            // Reset form
            setNewTokenDisplayName('');
        } else {
            setNotification({
                open: true,
                message: result.error?.message || 'Failed to create token',
                severity: 'error',
            });
        }
    };

    const handleToggleTokenEnabled = async (token: APIToken) => {
        const result = await api.setAPITokenEnabled(token.token_id, !token.enabled);
        if (result.success) {
            setNotification({
                open: true,
                message: token.enabled ? 'Token disabled' : 'Token enabled',
                severity: 'success',
            });
            loadTokens();
        } else {
            setNotification({
                open: true,
                message: result.error?.message || 'Failed to update token',
                severity: 'error',
            });
        }
    };

    const handleDeleteToken = async () => {
        if (!tokenToDelete) return;

        setDeletingToken(true);
        const result = await api.deleteAPIToken(tokenToDelete.token_id);

        setDeletingToken(false);
        setDeleteDialogOpen(false);
        setTokenToDelete(null);

        if (result.success) {
            setNotification({
                open: true,
                message: 'Token deleted successfully',
                severity: 'success',
            });
            loadTokens();
        } else {
            setNotification({
                open: true,
                message: result.error?.message || 'Failed to delete token',
                severity: 'error',
            });
        }
    };

    const formatDate = (dateStr?: string) => {
        return new Date(dateStr).toLocaleString();
    };

    const getStatusChip = (token: APIToken) => {
        if (!token.enabled) {
            return <Chip label="Disabled" color="default" size="small" />;
        }
        return <Chip label="Active" color="success" size="small" icon={<IconCheck size={14} />} />;
    };

    return (
        <PageLayout loading={loading} notification={notification}>
            <Stack spacing={3}>
                {/* Header */}
                <Box display="flex" justifyContent="space-between" alignItems="center">
                    <Typography variant="h4" fontWeight="bold">
                        Tingly Box Share Model Tokens
                    </Typography>
                    <Button
                        variant="contained"
                        startIcon={<IconPlus size={18} />}
                        onClick={() => setCreateDialogOpen(true)}
                    >
                        Create Token
                    </Button>
                </Box>

                {/* Info Alert */}
                <Alert severity="info" icon={<IconInfoCircle size={20} />}>
                    API tokens allow you to authenticate with the system. Each token is automatically associated with your user account
                    and isolates usage data. Tokens can be revoked at any time.
                </Alert>

                {/* Tokens Table */}
                <UnifiedCard title="Tokens" size="full">
                    <TableContainer>
                        <Table>
                            <TableHead>
                                <TableRow
                                    sx={{
                                        bgcolor: 'action.hover',
                                        '& th': {
                                            fontWeight: 700,
                                            fontSize: '0.75rem',
                                            textTransform: 'uppercase',
                                            letterSpacing: '0.05em',
                                            color: 'text.secondary',
                                            borderBottom: '2px solid',
                                            borderColor: 'divider',
                                            py: 1.5,
                                        },
                                    }}
                                >
                                    <TableCell>Name</TableCell>
                                    <TableCell>UUID</TableCell>
                                    <TableCell>Token</TableCell>
                                    <TableCell>Status</TableCell>
                                    <TableCell>Created</TableCell>
                                    <TableCell>Last Used</TableCell>
                                    <TableCell align="right">Actions</TableCell>
                                </TableRow>
                            </TableHead>
                            <TableBody>
                                {tokens.length === 0 ? (
                                    <TableRow>
                                        <TableCell colSpan={7} align="center" sx={{ border: 0 }}>
                                            <Stack alignItems="center" spacing={1.5} sx={{ py: 6 }}>
                                                <Box
                                                    sx={{
                                                        width: 48,
                                                        height: 48,
                                                        borderRadius: '50%',
                                                        bgcolor: 'action.hover',
                                                        display: 'flex',
                                                        alignItems: 'center',
                                                        justifyContent: 'center',
                                                        color: 'text.disabled',
                                                    }}
                                                >
                                                    <IconKey size={24} />
                                                </Box>
                                                <Typography variant="body2" color="text.secondary">
                                                    No API tokens found. Create your first token to get started.
                                                </Typography>
                                            </Stack>
                                        </TableCell>
                                    </TableRow>
                                ) : (
                                    tokens.map((token) => (
                                        <TableRow
                                            key={token.token_id}
                                            sx={{
                                                '&:last-child td': { border: 0 },
                                                opacity: !token.enabled ? 0.55 : 1,
                                                transition: 'background-color 0.15s ease',
                                                '&:hover': {
                                                    bgcolor: 'action.hover',
                                                },
                                                '& td': { py: 1.5 },
                                            }}
                                        >
                                            <TableCell>
                                                <Stack direction="row" spacing={1} alignItems="center">
                                                    <Box
                                                        sx={{
                                                            color: token.enabled ? 'primary.main' : 'text.disabled',
                                                            display: 'flex',
                                                            flexShrink: 0,
                                                        }}
                                                    >
                                                        <IconKey size={15} />
                                                    </Box>
                                                    <Typography variant="body2" fontWeight={600}>
                                                        {token.display_name}
                                                    </Typography>
                                                </Stack>
                                            </TableCell>
                                            <TableCell>
                                                <Tooltip title={token.user_uuid} placement="top">
                                                    <Stack direction="row" spacing={0.5} alignItems="center" sx={{ width: 'fit-content', cursor: 'default' }}>
                                                        <IconUser size={13} style={{ opacity: 0.4, flexShrink: 0 }} />
                                                        <Typography
                                                            variant="caption"
                                                            sx={{ fontFamily: 'monospace', color: 'text.secondary' }}
                                                        >
                                                            {token.user_uuid.slice(0, 8)}…
                                                        </Typography>
                                                    </Stack>
                                                </Tooltip>
                                            </TableCell>
                                            <TableCell sx={{ maxWidth: 260 }}>
                                                <Box
                                                    sx={{
                                                        px: 1,
                                                        py: 0.5,
                                                        bgcolor: 'action.selected',
                                                        borderRadius: 1,
                                                        border: '1px solid',
                                                        borderColor: 'divider',
                                                        display: 'flex',
                                                        alignItems: 'center',
                                                        gap: 0.5,
                                                    }}
                                                >
                                                    <Typography
                                                        variant="caption"
                                                        sx={{
                                                            fontFamily: 'monospace',
                                                            flex: 1,
                                                            overflow: 'hidden',
                                                            textOverflow: 'ellipsis',
                                                            whiteSpace: 'nowrap',
                                                            color: 'text.secondary',
                                                            letterSpacing: '0.02em',
                                                        }}
                                                    >
                                                        {visibleTokens[token.token_id] ? token.token_id : maskToken(token.token_id)}
                                                    </Typography>
                                                    <Box sx={{ display: 'flex', gap: 0, flexShrink: 0 }}>
                                                        <Tooltip title={visibleTokens[token.token_id] ? 'Hide' : 'Show'}>
                                                            <IconButton
                                                                size="small"
                                                                sx={{ p: 0.25, color: 'text.disabled', '&:hover': { color: 'text.primary' } }}
                                                                onClick={() => toggleTokenVisibility(token.token_id)}
                                                            >
                                                                {visibleTokens[token.token_id] ? <IconEyeOff size={13} /> : <IconEye size={13} />}
                                                            </IconButton>
                                                        </Tooltip>
                                                        <Tooltip title="Copy">
                                                            <IconButton
                                                                size="small"
                                                                sx={{ p: 0.25, color: 'text.disabled', '&:hover': { color: 'text.primary' } }}
                                                                onClick={() => {
                                                                    navigator.clipboard.writeText(token.token_id);
                                                                    setNotification({
                                                                        open: true,
                                                                        message: 'Token copied to clipboard',
                                                                        severity: 'success',
                                                                    });
                                                                }}
                                                            >
                                                                <IconCopy size={13} />
                                                            </IconButton>
                                                        </Tooltip>
                                                    </Box>
                                                </Box>
                                            </TableCell>
                                            <TableCell>{getStatusChip(token)}</TableCell>
                                            <TableCell>
                                                <Stack direction="row" spacing={0.5} alignItems="center">
                                                    <IconClock size={13} style={{ opacity: 0.4, flexShrink: 0 }} />
                                                    <Typography variant="caption" color="text.secondary">
                                                        {formatDate(token.created_at)}
                                                    </Typography>
                                                </Stack>
                                            </TableCell>
                                            <TableCell>
                                                <Typography variant="caption" color="text.secondary">
                                                    {formatDate(token.last_used_at)}
                                                </Typography>
                                            </TableCell>
                                            <TableCell align="right">
                                                <Stack direction="row" spacing={0.25} justifyContent="flex-end">
                                                    <Tooltip title={token.enabled ? 'Disable token' : 'Enable token'}>
                                                        <IconButton
                                                            size="small"
                                                            color={token.enabled ? 'warning' : 'success'}
                                                            onClick={() => handleToggleTokenEnabled(token)}
                                                            sx={{ opacity: 0.75, '&:hover': { opacity: 1 } }}
                                                        >
                                                            {token.enabled ? <IconShield size={15} /> : <IconCheck size={15} />}
                                                        </IconButton>
                                                    </Tooltip>
                                                    <Tooltip title="Delete token">
                                                        <IconButton
                                                            size="small"
                                                            color="error"
                                                            onClick={() => {
                                                                setTokenToDelete(token);
                                                                setDeleteDialogOpen(true);
                                                            }}
                                                            sx={{ opacity: 0.75, '&:hover': { opacity: 1 } }}
                                                        >
                                                            <IconTrash size={15} />
                                                        </IconButton>
                                                    </Tooltip>
                                                </Stack>
                                            </TableCell>
                                        </TableRow>
                                    ))
                                )}
                            </TableBody>
                        </Table>
                    </TableContainer>
                </UnifiedCard>
            </Stack>

            {/* Create Token Dialog */}
            <Dialog open={createDialogOpen} onClose={() => setCreateDialogOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>Create API Token</DialogTitle>
                <DialogContent>
                    <Stack spacing={3} sx={{ mt: 1 }}>
                        <TextField
                            label="Display Name"
                            fullWidth
                            value={newTokenDisplayName}
                            onChange={(e) => setNewTokenDisplayName(e.target.value)}
                            placeholder="e.g., Production API Token"
                            helperText="A descriptive name for this token"
                            autoFocus
                        />
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setCreateDialogOpen(false)}>Cancel</Button>
                    <Button
                        variant="contained"
                        onClick={handleCreateToken}
                        disabled={creatingToken || !newTokenDisplayName}
                        startIcon={creatingToken ? <CircularProgress size={16} /> : <IconPlus size={18} />}
                    >
                        Create Token
                    </Button>
                </DialogActions>
            </Dialog>

            {/* Delete Confirm Dialog */}
            <Dialog open={deleteDialogOpen} onClose={() => setDeleteDialogOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>
                    <Stack direction="row" spacing={1} alignItems="center">
                        <IconTrash color="#f44336" />
                        <span>Delete Token</span>
                    </Stack>
                </DialogTitle>
                <DialogContent>
                    <Typography>
                        Are you sure you want to delete the token <strong>"{tokenToDelete?.display_name}"</strong>?
                        This action cannot be undone.
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setDeleteDialogOpen(false)} disabled={deletingToken}>
                        Cancel
                    </Button>
                    <Button
                        variant="contained"
                        color="error"
                        onClick={handleDeleteToken}
                        disabled={deletingToken}
                        startIcon={deletingToken ? <CircularProgress size={16} /> : <IconTrash size={18} />}
                    >
                        Delete Token
                    </Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
};

export default APITokensPage;

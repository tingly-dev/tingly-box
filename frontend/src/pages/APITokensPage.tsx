import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import { Key as IconKey, Add as IconPlus, Delete as IconTrash, DeleteOutline as IconDeleteOutline, ContentCopy as IconCopy, AccessTime as IconClock, Person as IconUser, Visibility as IconEye, VisibilityOff as IconEyeOff } from '@/components/icons';
import {
    Box,
    Button,
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
    Switch,
} from '@mui/material';
import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useNotify } from '@/hooks/useNotify';

interface APIToken {
    token_id: string;
    user_id: string;
    display_name: string;
    enabled: boolean;
    last_used_at?: string;
    created_at: string;
    created_by?: string;
}

const APITokensPage = () => {
    const { t } = useTranslation();
    const notify = useNotify();
    const [tokens, setTokens] = useState<APIToken[]>([]);
    const [loading, setLoading] = useState(true);

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
            notify.error('Display Name is required');
            return;
        }

        setCreatingToken(true);
        const result = await api.createAPIToken({
            display_name: newTokenDisplayName,
        });

        setCreatingToken(false);
        setCreateDialogOpen(false);

        if (result.success && result.data) {
            notify.success('Token created successfully');
            loadTokens();

            // Reset form
            setNewTokenDisplayName('');
        } else {
            notify.error(result.error?.message || 'Failed to create token');
        }
    };

    const handleToggleTokenEnabled = async (token: APIToken) => {
        const result = await api.setAPITokenEnabled(token.token_id, !token.enabled);
        if (result.success) {
            notify.success(token.enabled ? 'Token disabled' : 'Token enabled');
            loadTokens();
        } else {
            notify.error(result.error?.message || 'Failed to update token');
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
            notify.success('Token deleted successfully');
            loadTokens();
        } else {
            notify.error(result.error?.message || 'Failed to delete token');
        }
    };

    const formatDate = (dateStr?: string) => {
        if (!dateStr) return '-';
        return new Date(dateStr).toLocaleString();
    };

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                {/* Tokens Table — the page-level explanation now lives as a hover tooltip
                    on the Sharing sidebar item (see useActivityItems.tsx). */}
                <UnifiedCard
                    title="Tingly Box Share Model Tokens"
                    subtitle="Manage API tokens for sharing model access with clients and environments."
                    size="full"
                    rightAction={
                        <Button
                            variant="contained"
                            startIcon={<IconPlus sx={{ fontSize: 18 }} />}
                            onClick={() => setCreateDialogOpen(true)}
                        >
                            Create Token
                        </Button>
                    }
                >
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
                                    <TableCell>UUID</TableCell>
                                    <TableCell>Name</TableCell>
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
                                                    <IconKey sx={{ fontSize: 24 }} />
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
                                                <Tooltip title={token.user_id} placement="top">
                                                    <Stack direction="row" spacing={0.5} alignItems="center" sx={{ width: 'fit-content', cursor: 'default' }}>
                                                        <IconUser sx={{ fontSize: 13, opacity: 0.4, flexShrink: 0 }} />
                                                        <Typography
                                                            variant="caption"
                                                            sx={{ fontFamily: 'monospace', color: 'text.secondary' }}
                                                        >
                                                            {token.user_id.slice(0, 8)}…
                                                        </Typography>
                                                    </Stack>
                                                </Tooltip>
                                            </TableCell>
                                            <TableCell>
                                                <Stack direction="row" spacing={1} alignItems="center">
                                                    <Box
                                                        sx={{
                                                            color: token.enabled ? 'primary.main' : 'text.disabled',
                                                            display: 'flex',
                                                            flexShrink: 0,
                                                        }}
                                                    >
                                                        <IconKey sx={{ fontSize: 15 }} />
                                                    </Box>
                                                    <Typography variant="body2" fontWeight={600}>
                                                        {token.display_name}
                                                    </Typography>
                                                </Stack>
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
                                                                {visibleTokens[token.token_id] ? <IconEyeOff sx={{ fontSize: 13 }} /> : <IconEye sx={{ fontSize: 13 }} />}
                                                            </IconButton>
                                                        </Tooltip>
                                                        <Tooltip title="Copy">
                                                            <IconButton
                                                                size="small"
                                                                sx={{ p: 0.25, color: 'text.disabled', '&:hover': { color: 'text.primary' } }}
                                                                onClick={() => {
                                                                    navigator.clipboard.writeText(token.token_id);
                                                                    notify.success('Token copied to clipboard');
                                                                }}
                                                            >
                                                                <IconCopy sx={{ fontSize: 13 }} />
                                                            </IconButton>
                                                        </Tooltip>
                                                    </Box>
                                                </Box>
                                            </TableCell>
                                            <TableCell>
                                                <Tooltip title={token.enabled ? 'Active' : 'Disabled'} placement="top">
                                                    <Switch
                                                        size="small"
                                                        checked={token.enabled}
                                                        onChange={() => handleToggleTokenEnabled(token)}
                                                        color="success"
                                                        inputProps={{ 'aria-label': `${token.enabled ? 'Disable' : 'Enable'} ${token.display_name}` }}
                                                    />
                                                </Tooltip>
                                            </TableCell>
                                            <TableCell>
                                                <Stack direction="row" spacing={0.5} alignItems="center">
                                                    <IconClock sx={{ fontSize: 13, opacity: 0.4, flexShrink: 0 }} />
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
                                                        <IconDeleteOutline sx={{ fontSize: 16 }} />
                                                    </IconButton>
                                                </Tooltip>
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
                        startIcon={creatingToken ? <CircularProgress size={16} /> : <IconPlus sx={{ fontSize: 18 }} />}
                    >
                        Create Token
                    </Button>
                </DialogActions>
            </Dialog>

            {/* Delete Confirm Dialog */}
            <Dialog open={deleteDialogOpen} onClose={() => setDeleteDialogOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>
                    <Stack direction="row" spacing={1} alignItems="center">
                        <IconTrash color="error" />
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
                        startIcon={deletingToken ? <CircularProgress size={16} /> : <IconTrash sx={{ fontSize: 18 }} />}
                    >
                        Delete Token
                    </Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
};

export default APITokensPage;

import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';
import { Add as IconPlus, Delete as IconTrash } from '@/components/icons';
import {
    Button,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useNotify } from '@/hooks/useNotify';
import SharingKeysTable, { type SharingKey } from '@/components/SharingKeysTable';

const SharingKeysPage = () => {
    const { t } = useTranslation();
    const notify = useNotify();
    const [tokens, setTokens] = useState<SharingKey[]>([]);
    const [loading, setLoading] = useState(true);

    // Create token dialog state
    const [createDialogOpen, setCreateDialogOpen] = useState(false);
    const [newTokenDisplayName, setNewTokenDisplayName] = useState('');
    const [creatingToken, setCreatingToken] = useState(false);

    // Delete confirm dialog
    const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
    const [tokenToDelete, setTokenToDelete] = useState<SharingKey | null>(null);
    const [deletingToken, setDeletingToken] = useState(false);

    // Token visibility state - map of token_id -> boolean
    const [visibleTokens, setVisibleTokens] = useState<Record<string, boolean>>({});

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
                    <SharingKeysTable
                        tokens={tokens}
                        loading={loading}
                        visibleTokens={visibleTokens}
                        onToggleVisibility={(tokenId) => setVisibleTokens(prev => ({ ...prev, [tokenId]: !prev[tokenId] }))}
                        onCopy={(tokenId) => {
                            navigator.clipboard.writeText(tokenId);
                            notify.success('Token copied to clipboard');
                        }}
                        onToggleEnabled={async (token) => {
                            const result = await api.setAPITokenEnabled(token.token_id, !token.enabled);
                            if (result.success) {
                                notify.success(token.enabled ? 'Token disabled' : 'Token enabled');
                                loadTokens();
                            } else {
                                notify.error(result.error?.message || 'Failed to update token');
                            }
                        }}
                        onDelete={(token) => {
                            setTokenToDelete(token);
                            setDeleteDialogOpen(true);
                        }}
                        showUserColumn={true}
                        showLastUsedColumn={true}
                        userColumnLabel="UUID"
                    />
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

export default SharingKeysPage;

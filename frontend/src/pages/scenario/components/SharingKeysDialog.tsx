import { Key as IconKey, Add as IconPlus, Delete as IconTrash } from '@/components/icons';
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
import { api } from '@/services/api';
import { useNotify } from '@/hooks/useNotify';
import SharingKeysTable, { type SharingKey } from '@/components/SharingKeysTable';

interface SharingKeysDialogProps {
    open: boolean;
    onClose: () => void;
}

const SharingKeysDialog: React.FC<SharingKeysDialogProps> = ({ open, onClose }) => {
    const notify = useNotify();

    const [sharingKeys, setSharingKeys] = useState<SharingKey[]>([]);
    const [keysLoading, setKeysLoading] = useState(true);
    const [visibleTokens, setVisibleTokens] = useState<Record<string, boolean>>({});
    const [createDialogOpen, setCreateDialogOpen] = useState(false);
    const [newTokenName, setNewTokenName] = useState('');
    const [creatingToken, setCreatingToken] = useState(false);
    const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
    const [tokenToDelete, setTokenToDelete] = useState<SharingKey | null>(null);
    const [deletingToken, setDeletingToken] = useState(false);

    const loadSharingKeys = async () => {
        setKeysLoading(true);
        const result = await api.listAPITokens();
        if (result.success && result.data) {
            setSharingKeys(result.data.tokens || []);
        }
        setKeysLoading(false);
    };

    useEffect(() => {
        if (open) {
            loadSharingKeys();
        }
    }, [open]);

    const handleCreateToken = async () => {
        if (!newTokenName.trim()) {
            notify.error('Display Name is required');
            return;
        }
        setCreatingToken(true);
        const result = await api.createAPIToken({ display_name: newTokenName.trim() });
        setCreatingToken(false);
        if (result.success) {
            notify.success('Token created successfully');
            setCreateDialogOpen(false);
            setNewTokenName('');
            loadSharingKeys();
        } else {
            notify.error(result.error?.message || 'Failed to create token');
        }
    };

    const handleDeleteToken = async () => {
        if (!tokenToDelete) return;
        setDeletingToken(true);
        const result = await api.deleteAPIToken(tokenToDelete.token_id);
        setDeletingToken(false);
        if (result.success) {
            notify.success('Token deleted successfully');
            setDeleteDialogOpen(false);
            setTokenToDelete(null);
            loadSharingKeys();
        } else {
            notify.error(result.error?.message || 'Failed to delete token');
        }
    };

    return (
        <>
            <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
                <DialogTitle sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                    <Stack direction="row" spacing={1} alignItems="center">
                        <IconKey />
                        <span>Sharing Keys</span>
                    </Stack>
                    <Button
                        variant="contained"
                        startIcon={<IconPlus sx={{ fontSize: 18 }} />}
                        onClick={() => setCreateDialogOpen(true)}
                    >
                        Create Token
                    </Button>
                </DialogTitle>
                <DialogContent>
                    <SharingKeysTable
                        tokens={sharingKeys}
                        loading={keysLoading}
                        visibleTokens={visibleTokens}
                        onToggleVisibility={(tokenId) => setVisibleTokens(prev => ({ ...prev, [tokenId]: !prev[tokenId] }))}
                        onCopy={(tokenId) => {
                            navigator.clipboard.writeText(tokenId);
                            notify.success('Token copied to clipboard');
                        }}
                        onToggleEnabled={async (key) => {
                            const result = await api.setAPITokenEnabled(key.token_id, !key.enabled);
                            if (result.success) {
                                notify.success(key.enabled ? 'Token disabled' : 'Token enabled');
                                loadSharingKeys();
                            } else {
                                notify.error(result.error?.message || 'Failed to update token');
                            }
                        }}
                        onDelete={(key) => {
                            setTokenToDelete(key);
                            setDeleteDialogOpen(true);
                        }}
                        showUserColumn={true}
                        showLastUsedColumn={false}
                    />
                </DialogContent>
            </Dialog>

            {/* Create Token Dialog */}
            <Dialog open={createDialogOpen} onClose={() => setCreateDialogOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>Create Sharing Key</DialogTitle>
                <DialogContent>
                    <Stack spacing={3} sx={{ mt: 1 }}>
                        <TextField
                            label="Display Name"
                            fullWidth
                            value={newTokenName}
                            onChange={(e) => setNewTokenName(e.target.value)}
                            placeholder="e.g., Team Alpha Key"
                            helperText="A descriptive name for this sharing key"
                            autoFocus
                        />
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setCreateDialogOpen(false)}>Cancel</Button>
                    <Button
                        variant="contained"
                        onClick={handleCreateToken}
                        disabled={creatingToken || !newTokenName.trim()}
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
                    <Button onClick={() => setDeleteDialogOpen(false)} disabled={deletingToken}>Cancel</Button>
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
        </>
    );
};

export default SharingKeysDialog;

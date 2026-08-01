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
import { useTranslation } from 'react-i18next';
import { api } from '@/services/api';
import { useNotify } from '@/hooks/useNotify';
import SharingKeysTable, { type SharingKey } from '@/components/SharingKeysTable';

interface SharingKeysDialogProps {
    open: boolean;
    onClose: () => void;
}

const SharingKeysDialog: React.FC<SharingKeysDialogProps> = ({ open, onClose }) => {
    const { t } = useTranslation();
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
            notify.error(t('sharingKeys.nameRequired'));
            return;
        }
        setCreatingToken(true);
        const result = await api.createAPIToken({ display_name: newTokenName.trim() });
        setCreatingToken(false);
        if (result.success) {
            notify.success(t('sharingKeys.createSuccess'));
            setCreateDialogOpen(false);
            setNewTokenName('');
            loadSharingKeys();
        } else {
            notify.error(result.error?.message || t('sharingKeys.createFailed'));
        }
    };

    const handleDeleteToken = async () => {
        if (!tokenToDelete) return;
        setDeletingToken(true);
        const result = await api.deleteAPIToken(tokenToDelete.token_id);
        setDeletingToken(false);
        if (result.success) {
            notify.success(t('sharingKeys.deleteSuccess'));
            setDeleteDialogOpen(false);
            setTokenToDelete(null);
            loadSharingKeys();
        } else {
            notify.error(result.error?.message || t('sharingKeys.deleteFailed'));
        }
    };

    return (
        <>
            <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth>
                <DialogTitle sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                    <Stack direction="row" spacing={1} sx={{
                        alignItems: "center"
                    }}>
                        <IconKey />
                        <span>{t('sharingKeys.title')}</span>
                    </Stack>
                    <Button
                        variant="contained"
                        startIcon={<IconPlus sx={{ fontSize: 18 }} />}
                        onClick={() => setCreateDialogOpen(true)}
                    >
                        {t('sharingKeys.createToken')}
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
                            notify.success(t('sharingKeys.copiedToClipboard'));
                        }}
                        onToggleEnabled={async (key) => {
                            const result = await api.setAPITokenEnabled(key.token_id, !key.enabled);
                            if (result.success) {
                                notify.success(key.enabled ? t('sharingKeys.disabled') : t('sharingKeys.enabled'));
                                loadSharingKeys();
                            } else {
                                notify.error(result.error?.message || t('sharingKeys.updateFailed'));
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
                <DialogTitle>{t('sharingKeys.createDialogTitle')}</DialogTitle>
                <DialogContent>
                    <Stack spacing={3} sx={{ mt: 1 }}>
                        <TextField
                            label={t('sharingKeys.displayName')}
                            fullWidth
                            value={newTokenName}
                            onChange={(e) => setNewTokenName(e.target.value)}
                            placeholder={t('sharingKeys.displayNamePlaceholder')}
                            helperText={t('sharingKeys.displayNameHelper')}
                            autoFocus
                        />
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setCreateDialogOpen(false)}>{t('common.cancel')}</Button>
                    <Button
                        variant="contained"
                        onClick={handleCreateToken}
                        disabled={creatingToken || !newTokenName.trim()}
                        startIcon={creatingToken ? <CircularProgress size={16} /> : <IconPlus sx={{ fontSize: 18 }} />}
                    >
                        {t('sharingKeys.createToken')}
                    </Button>
                </DialogActions>
            </Dialog>
            {/* Delete Confirm Dialog */}
            <Dialog open={deleteDialogOpen} onClose={() => setDeleteDialogOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>
                    <Stack direction="row" spacing={1} sx={{
                        alignItems: "center"
                    }}>
                        <IconTrash color="error" />
                        <span>{t('sharingKeys.deleteToken')}</span>
                    </Stack>
                </DialogTitle>
                <DialogContent>
                    <Typography>
                        {t('sharingKeys.deleteConfirm', { name: tokenToDelete?.display_name })}
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setDeleteDialogOpen(false)} disabled={deletingToken}>{t('common.cancel')}</Button>
                    <Button
                        variant="contained"
                        color="error"
                        onClick={handleDeleteToken}
                        disabled={deletingToken}
                        startIcon={deletingToken ? <CircularProgress size={16} /> : <IconTrash sx={{ fontSize: 18 }} />}
                    >
                        {t('sharingKeys.deleteToken')}
                    </Button>
                </DialogActions>
            </Dialog>
        </>
    );
};

export default SharingKeysDialog;

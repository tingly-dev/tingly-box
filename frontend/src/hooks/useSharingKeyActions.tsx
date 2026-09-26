import { Add as IconPlus, Delete as IconTrash } from '@/components/icons';
import {
    Button,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    MenuItem,
    Stack,
    TextField,
} from '@mui/material';
import { useState, type ComponentProps } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '@/services/api';
import { useNotify } from '@/hooks/useNotify';
import type SharingKeysTable from '@/components/SharingKeysTable';
import type { SharingKey } from '@/components/SharingKeysTable';
import type { Team } from '@/types/team';
import TeamKeyScopeAlert from '@/pages/scenario/components/TeamKeyScopeAlert';
import ConfirmDialog from '@/components/ConfirmDialog';

type TableActionProps = Pick<
    ComponentProps<typeof SharingKeysTable>,
    'visibleTokens' | 'onToggleVisibility' | 'onCopy' | 'onToggleEnabled' | 'onDelete' | 'onMove'
>;

interface UseSharingKeyActionsOptions {
    /** All teams — used as move destinations. */
    teams: Team[];
    /** Called after any mutation so the caller can reload its key list. */
    onChanged: () => void;
}

/**
 * Create / move / enable / delete behaviour for Sharing Keys, shared by every
 * surface that lists them (the per-Team dialog and the all-keys page) so the
 * dialogs, copy and notifications stay identical. Spread `tableProps` into
 * `SharingKeysTable` and render `dialogs` once.
 */
export function useSharingKeyActions({ teams, onChanged }: UseSharingKeyActionsOptions) {
    const { t } = useTranslation();
    const notify = useNotify();

    const [visibleTokens, setVisibleTokens] = useState<Record<string, boolean>>({});
    const [createTeam, setCreateTeam] = useState<Team | null>(null);
    const [newTokenName, setNewTokenName] = useState('');
    const [creatingToken, setCreatingToken] = useState(false);
    const [tokenToDelete, setTokenToDelete] = useState<SharingKey | null>(null);
    // Kept separate from tokenToDelete so the name doesn't blank out while the dialog animates closed.
    const [deleteOpen, setDeleteOpen] = useState(false);
    const [deletingToken, setDeletingToken] = useState(false);
    const [tokenToMove, setTokenToMove] = useState<SharingKey | null>(null);
    const [moveTargetTeamID, setMoveTargetTeamID] = useState('');
    const [movingToken, setMovingToken] = useState(false);

    const openCreate = (team: Team) => {
        setNewTokenName('');
        setCreateTeam(team);
    };

    const handleCreateToken = async () => {
        if (!createTeam) return;
        if (!newTokenName.trim()) {
            notify.error(t('sharingKeys.nameRequired'));
            return;
        }
        setCreatingToken(true);
        const result = await api.createAPIToken({ display_name: newTokenName.trim(), team_id: createTeam.id });
        setCreatingToken(false);
        if (result.success) {
            notify.success(t('sharingKeys.createSuccess'));
            // Reveal the new key so it can be copied straight away — the
            // key itself is what the user needs next, not just a toast.
            const newTokenId = result.data?.token_id;
            if (newTokenId) setVisibleTokens((prev) => ({ ...prev, [newTokenId]: true }));
            setCreateTeam(null);
            setNewTokenName('');
            onChanged();
        } else {
            notify.error(result.error?.message || t('sharingKeys.createFailed'));
        }
    };

    const handleMoveToken = async () => {
        if (!tokenToMove || !moveTargetTeamID) return;
        setMovingToken(true);
        const result = await api.moveAPITokenToTeam(tokenToMove.token_id, moveTargetTeamID);
        setMovingToken(false);
        if (result.success) {
            notify.success(t('sharingKeys.moveSuccess'));
            setTokenToMove(null);
            setMoveTargetTeamID('');
            onChanged();
        } else {
            notify.error(result.error?.message || t('sharingKeys.moveFailed'));
        }
    };

    const handleDeleteToken = async () => {
        if (!tokenToDelete) return;
        setDeletingToken(true);
        const result = await api.deleteAPIToken(tokenToDelete.token_id);
        setDeletingToken(false);
        if (result.success) {
            notify.success(t('sharingKeys.deleteSuccess'));
            setDeleteOpen(false);
            onChanged();
        } else {
            notify.error(result.error?.message || t('sharingKeys.deleteFailed'));
        }
    };

    const tableProps: TableActionProps = {
        visibleTokens,
        onToggleVisibility: (tokenId) => setVisibleTokens((prev) => ({ ...prev, [tokenId]: !prev[tokenId] })),
        onCopy: (tokenId) => {
            navigator.clipboard.writeText(tokenId);
            notify.success(t('sharingKeys.copiedToClipboard'));
        },
        onToggleEnabled: async (key) => {
            const result = await api.setAPITokenEnabled(key.token_id, !key.enabled);
            if (result.success) {
                notify.success(key.enabled ? t('sharingKeys.disabled') : t('sharingKeys.enabled'));
                onChanged();
            } else {
                notify.error(result.error?.message || t('sharingKeys.updateFailed'));
            }
        },
        onDelete: (key) => {
            setTokenToDelete(key);
            setDeleteOpen(true);
        },
        onMove: (key) => {
            setTokenToMove(key);
            setMoveTargetTeamID('');
        },
    };

    const eligibleMoveTargets = teams.filter(
        (candidate) => candidate.id !== tokenToMove?.team_id && candidate.enabled,
    );

    const dialogs = (
        <>
            <Dialog open={Boolean(tokenToMove)} onClose={() => setTokenToMove(null)} maxWidth="sm" fullWidth>
                <DialogTitle>{t('sharingKeys.moveToken')}</DialogTitle>
                <DialogContent>
                    <TextField
                        select
                        fullWidth
                        sx={{ mt: 1 }}
                        label={t('sharingKeys.destinationTeam')}
                        value={moveTargetTeamID}
                        onChange={(event) => setMoveTargetTeamID(event.target.value)}
                        helperText={t('sharingKeys.moveHelper', { name: tokenToMove?.display_name })}
                    >
                        {eligibleMoveTargets.length === 0 && (
                            <MenuItem disabled value="">{t('sharingKeys.noDestinationTeam')}</MenuItem>
                        )}
                        {eligibleMoveTargets.map((candidate) => (
                            <MenuItem key={candidate.id} value={candidate.id}>{candidate.name}</MenuItem>
                        ))}
                    </TextField>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setTokenToMove(null)} disabled={movingToken}>{t('common.cancel')}</Button>
                    <Button variant="contained" onClick={handleMoveToken} disabled={movingToken || !moveTargetTeamID}>
                        {t('sharingKeys.moveToken')}
                    </Button>
                </DialogActions>
            </Dialog>
            <Dialog open={Boolean(createTeam)} onClose={() => setCreateTeam(null)} maxWidth="sm" fullWidth>
                <DialogTitle>{t('sharingKeys.createDialogTitle')}</DialogTitle>
                <DialogContent>
                    <Stack spacing={2} sx={{ mt: 1 }}>
                        {createTeam && <TeamKeyScopeAlert team={createTeam} />}
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
                    <Button onClick={() => setCreateTeam(null)}>{t('common.cancel')}</Button>
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
            <ConfirmDialog
                open={deleteOpen}
                onClose={() => setDeleteOpen(false)}
                onConfirm={handleDeleteToken}
                loading={deletingToken}
                confirmColor="error"
                confirmLabel={t('sharingKeys.deleteToken')}
                title={
                    <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                        <IconTrash color="error" />
                        <span>{t('sharingKeys.deleteToken')}</span>
                    </Stack>
                }
                description={t('sharingKeys.deleteConfirm', { name: tokenToDelete?.display_name })}
            />
        </>
    );

    return { tableProps, openCreate, dialogs };
}

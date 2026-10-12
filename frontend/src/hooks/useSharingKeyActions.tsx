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
import { writeToClipboard } from '@/utils/clipboard';
import type SharingKeysTable from '@/components/SharingKeysTable';
import type { SharingKey } from '@/components/SharingKeysTable';
import type { Team } from '@/types/team';
import TeamKeyScopeAlert from '@/components/TeamKeyScopeAlert';
import ConfirmDialog from '@/components/ConfirmDialog';

type TableActionProps = Pick<
    ComponentProps<typeof SharingKeysTable>,
    'visibleTokens' | 'onToggleVisibility' | 'onCopy' | 'onToggleEnabled' | 'onDelete' | 'onMove' | 'onEditRateLimit'
>;

/** Server default for a new key's requests-per-minute budget (TEP-1851): 0 = unlimited. */
const DEFAULT_RPM_LIMIT = 0;
/** Mirrors db.MaxSharingKeyRPMLimit; the API rejects anything above it. */
const MAX_RPM_LIMIT = 1_000_000;

/**
 * Parse the rpm field: a whole number in [0, MAX_RPM_LIMIT], where 0 means
 * unlimited. An empty field also means 0 — the user has to clear the
 * prefilled "0" to type a value, and that moment must not read as an error.
 */
const parseRpm = (raw: string): number | null => {
    const trimmed = raw.trim();
    if (trimmed === '') return 0;
    if (!/^\d+$/.test(trimmed)) return null;
    const n = Number(trimmed);
    return n > MAX_RPM_LIMIT ? null : n;
};

interface UseSharingKeyActionsOptions {
    /** All teams — used as move destinations. */
    teams: Team[];
    /** Called after any mutation so the caller can reload its key list. */
    onChanged: () => void;
}

/**
 * Create / move / enable / delete / rate-limit behaviour for Sharing Keys,
 * shared by every surface that lists them (the per-Team dialog and the
 * all-keys page) so the dialogs, copy and notifications stay identical.
 * Spread `tableProps` into `SharingKeysTable` and render `dialogs` once.
 */
export function useSharingKeyActions({ teams, onChanged }: UseSharingKeyActionsOptions) {
    const { t } = useTranslation();
    const notify = useNotify();

    const [visibleTokens, setVisibleTokens] = useState<Record<string, boolean>>({});
    const [createTeam, setCreateTeam] = useState<Team | null>(null);
    const [newTokenName, setNewTokenName] = useState('');
    const [newTokenRpm, setNewTokenRpm] = useState(String(DEFAULT_RPM_LIMIT));
    const [creatingToken, setCreatingToken] = useState(false);
    const [tokenToLimit, setTokenToLimit] = useState<SharingKey | null>(null);
    const [limitRpm, setLimitRpm] = useState('');
    const [savingLimit, setSavingLimit] = useState(false);
    const [tokenToDelete, setTokenToDelete] = useState<SharingKey | null>(null);
    // Kept separate from tokenToDelete so the name doesn't blank out while the dialog animates closed.
    const [deleteOpen, setDeleteOpen] = useState(false);
    const [deletingToken, setDeletingToken] = useState(false);
    const [tokenToMove, setTokenToMove] = useState<SharingKey | null>(null);
    const [moveTargetTeamID, setMoveTargetTeamID] = useState('');
    const [movingToken, setMovingToken] = useState(false);

    const openCreate = (team: Team) => {
        setNewTokenName('');
        setNewTokenRpm(String(DEFAULT_RPM_LIMIT));
        setCreateTeam(team);
    };

    const handleCreateToken = async () => {
        if (!createTeam) return;
        if (!newTokenName.trim()) {
            notify.error(t('sharingKeys.nameRequired'));
            return;
        }
        const rpm = parseRpm(newTokenRpm);
        if (rpm === null) {
            notify.error(t('sharingKeys.rpmInvalid'));
            return;
        }
        setCreatingToken(true);
        const result = await api.createAPIToken({ display_name: newTokenName.trim(), team_id: createTeam.id, rpm_limit: rpm });
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

    const handleSaveRateLimit = async () => {
        if (!tokenToLimit) return;
        const rpm = parseRpm(limitRpm);
        if (rpm === null) {
            notify.error(t('sharingKeys.rpmInvalid'));
            return;
        }
        setSavingLimit(true);
        const result = await api.setAPITokenRateLimit(tokenToLimit.token_id, rpm);
        setSavingLimit(false);
        if (result.success) {
            notify.success(t('sharingKeys.rateLimitSuccess'));
            setTokenToLimit(null);
            onChanged();
        } else {
            notify.error(result.error?.message || t('sharingKeys.rateLimitFailed'));
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
        onCopy: async (tokenId) => {
            try {
                await writeToClipboard(tokenId);
                notify.success(t('sharingKeys.copiedToClipboard'));
            } catch {
                notify.error(t('sharingKeys.copyFailed'));
            }
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
        onEditRateLimit: (key) => {
            setLimitRpm(String(key.rpm_limit));
            setTokenToLimit(key);
        },
    };

    // Shared by the create and edit dialogs so both explain "0 = unlimited"
    // the same way (one number, no separate on/off toggle).
    const rpmField = (value: string, onChange: (v: string) => void, autoFocus = false) => (
        <TextField
            label={t('sharingKeys.rpmLimit')}
            fullWidth
            value={value}
            onChange={(e) => onChange(e.target.value)}
            placeholder="0"
            slotProps={{ htmlInput: { inputMode: 'numeric', pattern: '[0-9]*' } }}
            error={parseRpm(value) === null}
            helperText={parseRpm(value) === null ? t('sharingKeys.rpmInvalid') : t('sharingKeys.rpmLimitHelper')}
            autoFocus={autoFocus}
        />
    );

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
                        {rpmField(newTokenRpm, setNewTokenRpm)}
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setCreateTeam(null)}>{t('common.cancel')}</Button>
                    <Button
                        variant="contained"
                        onClick={handleCreateToken}
                        disabled={creatingToken || !newTokenName.trim() || parseRpm(newTokenRpm) === null}
                        startIcon={creatingToken ? <CircularProgress size={16} /> : <IconPlus sx={{ fontSize: 18 }} />}
                    >
                        {t('sharingKeys.createToken')}
                    </Button>
                </DialogActions>
            </Dialog>
            <Dialog open={Boolean(tokenToLimit)} onClose={() => setTokenToLimit(null)} maxWidth="xs" fullWidth>
                {/* A form so Enter in the single field submits. */}
                <form
                    onSubmit={(e) => {
                        e.preventDefault();
                        if (!savingLimit && parseRpm(limitRpm) !== null) void handleSaveRateLimit();
                    }}
                >
                    <DialogTitle>{t('sharingKeys.rateLimitDialogTitle')}</DialogTitle>
                    <DialogContent>
                        <Stack spacing={2} sx={{ mt: 1 }}>
                            {rpmField(limitRpm, setLimitRpm, true)}
                        </Stack>
                    </DialogContent>
                    <DialogActions>
                        <Button onClick={() => setTokenToLimit(null)} disabled={savingLimit}>{t('common.cancel')}</Button>
                        <Button
                            type="submit"
                            variant="contained"
                            disabled={savingLimit || parseRpm(limitRpm) === null}
                            startIcon={savingLimit ? <CircularProgress size={16} /> : undefined}
                        >
                            {t('sharingKeys.saveRateLimit')}
                        </Button>
                    </DialogActions>
                </form>
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

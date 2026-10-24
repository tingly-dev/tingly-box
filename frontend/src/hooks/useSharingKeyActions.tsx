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
    Typography,
} from '@mui/material';
import { useEffect, useState, type ComponentProps } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '@/services/api';
import { useNotify } from '@/hooks/useNotify';
import { writeToClipboard } from '@/utils/clipboard';
import type SharingKeysTable from '@/components/SharingKeysTable';
import type { SharingKey } from '@/components/SharingKeysTable';
import type { Team } from '@/types/team';
import TeamKeyScopeAlert from '@/components/TeamKeyScopeAlert';
import ConfirmDialog from '@/components/ConfirmDialog';

export type SharingKeyTableActions = Pick<
    ComponentProps<typeof SharingKeysTable>,
    'visibleTokens' | 'onToggleVisibility' | 'onCopy' | 'onToggleEnabled' | 'onDelete' | 'onMove' | 'onQuota'
>;

interface UseSharingKeyActionsOptions {
    /** All teams — used as move destinations. */
    teams: Team[];
    /** Called after any mutation so the caller can reload its key list. */
    onChanged: () => void;
}

/**
 * Create / move / enable / delete behaviour for Sharing Keys, shared by every
 * surface that lists them (the Team page's key card and the all-keys page) so the
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
    const [weeklyLimit, setWeeklyLimit] = useState('');
    const [quotaKey, setQuotaKey] = useState<SharingKey | null>(null);
    const [savingQuota, setSavingQuota] = useState(false);
    const [resetMode, setResetMode] = useState<'provider' | 'manual' | 'legacy'>('provider');
    const [resetProvider, setResetProvider] = useState('');
    const [resetAnchor, setResetAnchor] = useState('');
    const [providers, setProviders] = useState<{uuid: string; name: string}[]>([]);
    const [providerLoadFailed, setProviderLoadFailed] = useState(false);
    const quotaOpen = Boolean(quotaKey);
    useEffect(() => {
        if (!quotaOpen) return;
        let active = true;
        api.getProviders().then((result) => {
            if (!active) return;
            if (!result.success) { setProviderLoadFailed(true); return; }
            const items = (result.data ?? []).filter((provider: {source?: string; auth_type?: string; enabled?: boolean}) => provider.source !== 'builtin' && provider.auth_type !== 'vmodel' && provider.enabled !== false);
            setProviders(items);
            if (items.length === 1) setResetProvider((value) => value || items[0].uuid);
        }).catch(() => { if (active) setProviderLoadFailed(true); });
        return () => { active = false; };
    }, [quotaOpen]);
    const validReset = (resetMode !== 'manual' || (resetAnchor !== '' && Number.isFinite(new Date(resetAnchor).getTime())))
        && (resetMode !== 'provider' || providers.length <= 1 || resetProvider !== '');
    const parsedLimit = weeklyLimit.trim() === '' ? null : Number(weeklyLimit);
    const validLimit = parsedLimit === null || (Number.isSafeInteger(parsedLimit) && parsedLimit >= 0);
    const [tokenToDelete, setTokenToDelete] = useState<SharingKey | null>(null);
    // Kept separate from tokenToDelete so the name doesn't blank out while the dialog animates closed.
    const [deleteOpen, setDeleteOpen] = useState(false);
    const [deletingToken, setDeletingToken] = useState(false);
    const [tokenToMove, setTokenToMove] = useState<SharingKey | null>(null);
    const [moveTargetTeamID, setMoveTargetTeamID] = useState('');
    const [movingToken, setMovingToken] = useState(false);

    const openCreate = (team: Team) => {
        setNewTokenName('');
        setWeeklyLimit('');
        setCreateTeam(team);
    };

    const handleCreateToken = async () => {
        if (!createTeam || !validLimit) return;
        if (!newTokenName.trim()) {
            notify.error(t('sharingKeys.nameRequired'));
            return;
        }
        setCreatingToken(true);
        const result = await api.createAPIToken({ display_name: newTokenName.trim(), team_id: createTeam.id, weekly_token_limit: parsedLimit });
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

    const tableProps: SharingKeyTableActions = {
        onQuota: (key) => {
            setProviderLoadFailed(false);
            setProviders([]);
            setWeeklyLimit(key.weekly_token_limit == null ? '' : String(key.weekly_token_limit));
            setResetMode(key.quota_reset?.mode || 'legacy');
            setResetProvider(key.quota_reset?.provider_id || '');
            const anchor = key.quota_reset?.anchor ? new Date(key.quota_reset.anchor) : null;
            setResetAnchor(anchor ? new Date(anchor.getTime() - anchor.getTimezoneOffset() * 60000).toISOString().slice(0, 19) : '');
            setQuotaKey(key);
        },
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
    };

    const eligibleMoveTargets = teams.filter(
        (candidate) => candidate.id !== tokenToMove?.team_id && candidate.enabled,
    );

    const handleSaveQuota = async () => {
        if (!quotaKey || !validLimit || !validReset) return;
        setSavingQuota(true);
        try {
            const result = await api.setAPITokenQuota(quotaKey.token_id, parsedLimit, {
                mode: resetMode,
                ...(resetMode === 'provider' ? {provider_id: resetProvider} : {}),
                ...(resetMode === 'manual' ? {anchor: new Date(resetAnchor).toISOString()} : {}),
            });
            if (result.success) {
                setQuotaKey(null);
                onChanged();
            } else {
                notify.error(result.error?.message || result.error || t('sharingKeys.updateFailed'));
            }
        } finally {
            setSavingQuota(false);
        }
    };

    const dialogs = (
        <>
            <Dialog open={Boolean(quotaKey)} onClose={() => { if (!savingQuota) setQuotaKey(null); }} maxWidth="sm" fullWidth>
                <DialogTitle>{t('sharingKeys.quota.title')}</DialogTitle>
                <DialogContent>
                    <Typography variant="body2">
                        {quotaKey?.display_name}
                        {' — '}
                        {t('sharingKeys.quota.remaining', {
                            used: (quotaKey?.weekly_tokens_used ?? 0).toLocaleString(),
                            remaining: parsedLimit == null || !validLimit ? t('sharingKeys.quota.unlimited')
                                : Math.max(0, parsedLimit - (quotaKey?.weekly_tokens_used ?? 0)).toLocaleString(),
                        })}
                    </Typography>
                    <TextField
                        fullWidth
                        sx={{ mt: 1 }}
                        type="number"
                        label={t('sharingKeys.quota.limit')}
                        value={weeklyLimit}
                        onChange={(event) => setWeeklyLimit(event.target.value)}
                        disabled={savingQuota}
                        error={!validLimit}
                        helperText={t('sharingKeys.quota.help')}
                    />
                    <TextField select fullWidth sx={{ mt: 2 }} label={t('sharingKeys.quota.resetMode')}
                        value={resetMode} onChange={(event) => setResetMode(event.target.value as typeof resetMode)} disabled={savingQuota}>
                        <MenuItem value="provider">{t('sharingKeys.quota.followProvider')}</MenuItem>
                        <MenuItem value="manual">{t('sharingKeys.quota.manualReset')}</MenuItem>
                        {resetMode === 'legacy' && <MenuItem value="legacy">{t('sharingKeys.quota.legacyReset')}</MenuItem>}
                    </TextField>
                    {resetMode === 'provider' && <TextField select fullWidth sx={{ mt: 2 }}
                        label={t('sharingKeys.quota.provider')} value={resetProvider}
                        onChange={(event) => setResetProvider(event.target.value)} disabled={savingQuota}
                        helperText={t(providerLoadFailed ? 'sharingKeys.quota.providerLoadFailed' : 'sharingKeys.quota.providerHelp')}>
                        <MenuItem value="">{t('sharingKeys.quota.autoProvider')}</MenuItem>
                        {resetProvider && !providers.some((provider) => provider.uuid === resetProvider) && <MenuItem value={resetProvider}>{resetProvider}</MenuItem>}
                        {providers.map((provider) => <MenuItem key={provider.uuid} value={provider.uuid}>{provider.name}</MenuItem>)}
                    </TextField>}
                    {resetMode === 'manual' && <TextField fullWidth sx={{ mt: 2 }} type="datetime-local"
                        label={t('sharingKeys.quota.anchor')} value={resetAnchor} disabled={savingQuota}
                        slotProps={{ inputLabel: { shrink: true }, htmlInput: { step: 1 } }} error={!validReset}
                        onChange={(event) => setResetAnchor(event.target.value)} helperText={t('sharingKeys.quota.manualHelp')} />}
                    <Typography variant="caption" sx={{ mt: 1, display: 'block' }}>
                        {quotaKey?.quota_resets_at ? t('sharingKeys.quota.resets', {time: new Date(quotaKey.quota_resets_at).toLocaleString()}) : t('sharingKeys.quota.unknownReset')}
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button disabled={savingQuota} onClick={() => setQuotaKey(null)}>{t('common.cancel')}</Button>
                    <Button variant="contained" disabled={savingQuota || !validLimit || !validReset} onClick={handleSaveQuota}>
                        {t('common.save')}
                    </Button>
                </DialogActions>
            </Dialog>
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
                        <TextField type="number" label={t('sharingKeys.quota.limit')} fullWidth
                            value={weeklyLimit} onChange={(event) => setWeeklyLimit(event.target.value)}
                            error={!validLimit} helperText={t('sharingKeys.quota.help')} />
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
                        disabled={creatingToken || !newTokenName.trim() || !validLimit}
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

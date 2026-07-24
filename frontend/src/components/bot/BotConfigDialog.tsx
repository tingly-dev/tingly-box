import BotAuthForm from './BotAuthForm';
import BotPlatformSelector from './BotPlatformSelector';
import { api } from '@/services/api';
import type { BotPlatformConfig, BotSettings } from '@/types/bot';
import { Box, Button, Modal, Stack, TextField, Typography } from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

export type BotConfigDialogNotify = (
    message: string,
    severity?: 'success' | 'error' | 'info' | 'warning'
) => void;

interface BotConfigDialogProps {
    open: boolean;
    mode: 'add' | 'edit';
    /** Bot to edit (edit mode). May be updated internally by the QR flow. */
    editUuid?: string | null;
    /** Platform the dialog is scoped to (selector stays locked in add mode). */
    platformId: string;
    /** Current bot list — used for edit prefill and QR orphan reuse. */
    bots: BotSettings[];
    /**
     * Whether the platform selector stays locked to `platformId` in add mode.
     * Defaults to true (existing per-platform host pages already know which
     * platform they're adding). Pages with no fixed platform (Overview) pass
     * false so the user picks one in the dialog itself.
     */
    lockPlatform?: boolean;
    onClose: () => void;
    /** Called after a successful save (and after QR binding) so the host page reloads. */
    onSaved: () => void | Promise<void>;
    notify: BotConfigDialogNotify;
}

// BotConfigDialog is the shared add/edit interaction for the bot RESOURCE
// (platform, auth, alias, proxy). Both twin sections use it — the Bots pages
// for direct management, and the Remote pages so "Add Bot" works in place
// without bouncing the user to the Bots section. It owns its platform-config
// loading and drafts; hosts only supply the bot list and a reload callback.
const BotConfigDialog: React.FC<BotConfigDialogProps> = ({
    open,
    mode,
    editUuid = null,
    platformId,
    bots,
    lockPlatform = true,
    onClose,
    onSaved,
    notify,
}) => {
    const { t } = useTranslation();

    const [botPlatforms, setBotPlatforms] = useState<BotPlatformConfig[]>([]);
    const [platformsLoading, setPlatformsLoading] = useState(false);
    const [currentPlatformConfig, setCurrentPlatformConfig] = useState<BotPlatformConfig | null>(null);

    const [dialogMode, setDialogMode] = useState<'add' | 'edit'>(mode);
    const [targetUuid, setTargetUuid] = useState<string | null>(editUuid);
    const [nameDraft, setNameDraft] = useState('');
    const [platformDraft, setPlatformDraft] = useState(platformId);
    const [authDraft, setAuthDraft] = useState<Record<string, string>>({});
    const [proxyDraft, setProxyDraft] = useState('');
    const [saving, setSaving] = useState(false);

    // Load platform configs once (first open).
    useEffect(() => {
        if (!open || botPlatforms.length > 0 || platformsLoading) return;
        (async () => {
            try {
                setPlatformsLoading(true);
                const data = await api.getImBotPlatforms();
                if (data?.success && data?.platforms) {
                    setBotPlatforms(data.platforms);
                }
            } catch (err) {
                console.error('Failed to load bot platforms:', err);
            } finally {
                setPlatformsLoading(false);
            }
        })();
    }, [open, botPlatforms.length, platformsLoading]);

    // (Re)initialize drafts each time the dialog opens or platform configs arrive.
    useEffect(() => {
        if (!open) return;

        if (mode === 'edit' && editUuid) {
            const bot = bots.find(b => b.uuid === editUuid);
            if (bot) {
                setDialogMode('edit');
                setTargetUuid(editUuid);
                setNameDraft(bot.name || '');
                setPlatformDraft(bot.platform || platformId);
                setAuthDraft(bot.auth ? { ...bot.auth } : {});
                setProxyDraft(bot.proxy_url || '');
                setCurrentPlatformConfig(botPlatforms.find(p => p.platform === bot.platform) ?? null);
            }
            return;
        }

        setDialogMode('add');
        setTargetUuid(null);
        setNameDraft('');
        setPlatformDraft(platformId);
        setAuthDraft({});
        setProxyDraft('');
        const config = botPlatforms.find(p => p.platform === platformId) ?? null;
        setCurrentPlatformConfig(config);
        // For QR auth: reuse an existing orphan bot (one that was created by a
        // previous QR binding but whose frontend session failed before cleanup)
        if (config?.auth_type === 'qr') {
            const orphan = bots.find(
                b => b.platform === platformId && b.auth_type === 'qr' && !b.auth?.token
            );
            if (orphan?.uuid) {
                setTargetUuid(orphan.uuid);
                notify(t('remoteControl.notify.unboundReuse', { defaultValue: 'Found an unbound bot, reusing it for QR binding' }), 'info');
            }
        }
        // Intentionally keyed on `open` + configs: drafts reset on every open,
        // not on unrelated parent re-renders while the dialog is up.
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [open, mode, editUuid, botPlatforms]);

    const handleSave = useCallback(async () => {
        setSaving(true);
        try {
            const platformConfig = botPlatforms.find(p => p.platform === platformDraft);
            if (!platformConfig) {
                notify(t('remoteControl.notify.unknownPlatform', { defaultValue: 'Unknown platform: {{platform}}', platform: platformDraft }), 'error');
                return;
            }

            // For QR auth type, auth is handled by QR flow, no validation needed
            // For other auth types, validate required fields
            if (platformConfig.auth_type === 'qr') {
                if (!targetUuid) {
                    notify(t('remoteControl.notify.qrBindRequired', { defaultValue: 'Please complete WeChat QR binding before saving' }), 'error');
                    return;
                }
            } else {
                const missingFields = platformConfig.fields
                    .filter(f => f.required && !authDraft[f.key]?.trim())
                    .map(f => f.label);
                if (missingFields.length > 0) {
                    notify(t('remoteControl.notify.missingFields', { defaultValue: 'Missing required fields: {{fields}}', fields: missingFields.join(', ') }), 'error');
                    return;
                }
            }

            const data = {
                name: nameDraft.trim() || `${platformDraft} Bot`,
                platform: platformDraft,
                auth_type: platformConfig.auth_type,
                auth: authDraft,
                proxy_url: proxyDraft.trim(),
                enabled: true, // Enable the bot after saving
            };

            const result = dialogMode === 'edit' && targetUuid
                ? await api.updateImBotSetting(targetUuid, data)
                : await api.createImBotSetting(data);

            if (result?.success === false) {
                notify(result.error || t('remoteControl.notify.saveFailed', { defaultValue: 'Failed to save bot settings' }), 'error');
                return;
            }

            await onSaved();
            notify(
                dialogMode === 'edit'
                    ? t('remoteControl.notify.botUpdated', { defaultValue: 'Bot updated successfully.' })
                    : t('remoteControl.notify.botCreated', { defaultValue: 'Bot created successfully.' }),
                'success'
            );
            onClose();
        } catch (err) {
            console.error('Failed to save bot settings:', err);
            notify(t('remoteControl.notify.saveFailed', { defaultValue: 'Failed to save bot settings' }), 'error');
        } finally {
            setSaving(false);
        }
    }, [botPlatforms, platformDraft, targetUuid, authDraft, nameDraft, proxyDraft, dialogMode, onSaved, onClose, notify, t]);

    return (
        <Modal open={open} onClose={onClose}>
            <Box
                sx={{
                    position: 'absolute',
                    top: '50%',
                    left: '50%',
                    transform: 'translate(-50%, -50%)',
                    width: 600,
                    maxWidth: '80vw',
                    maxHeight: '80vh',
                    bgcolor: 'background.paper',
                    boxShadow: 24,
                    borderRadius: 2,
                    display: 'flex',
                    flexDirection: 'column',
                    overflow: 'hidden',
                }}
            >
                <Stack
                    sx={{
                        overflowY: 'auto',
                        p: 4,
                        gap: 2,
                        flex: 1,
                    }}
                >
                    <Typography variant="h6">
                        {dialogMode === 'edit'
                            ? t('remoteControl.dialog.editTitle', { defaultValue: 'Edit Bot Configuration' })
                            : t('remoteControl.dialog.addTitle', { defaultValue: 'Add Bot Configuration' })}
                    </Typography>
                    <Stack spacing={2}>
                        <Stack spacing={1}>
                            <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                {t('remoteControl.dialog.platform', { defaultValue: 'Platform' })}
                            </Typography>
                            <BotPlatformSelector
                                value={platformDraft}
                                onChange={(platform) => {
                                    setPlatformDraft(platform);
                                    // Clear auth draft when platform changes
                                    setAuthDraft({});
                                    setCurrentPlatformConfig(botPlatforms.find(p => p.platform === platform) ?? null);
                                }}
                                platforms={botPlatforms}
                                loading={platformsLoading}
                                disabled={saving || (dialogMode === 'add' && lockPlatform)}
                            />
                        </Stack>

                        {currentPlatformConfig && (
                            <BotAuthForm
                                platform={platformDraft}
                                authType={currentPlatformConfig.auth_type}
                                fields={currentPlatformConfig.fields}
                                authData={authDraft}
                                onChange={(key, value) => setAuthDraft(prev => ({ ...prev, [key]: value }))}
                                disabled={saving}
                                botUUID={targetUuid ?? undefined}
                                botName={nameDraft || `${platformDraft} Bot`}
                                onBindingComplete={async (realUUID) => {
                                    // After QR scan: set the real UUID and reload credentials
                                    setTargetUuid(realUUID);
                                    setDialogMode('edit');
                                    try {
                                        const data = await api.getImBotSetting(realUUID);
                                        if (data?.settings?.auth) {
                                            setAuthDraft(data.settings.auth);
                                        }
                                    } catch (err) {
                                        console.error('Failed to reload bot after binding:', err);
                                    }
                                    await onSaved();
                                }}
                            />
                        )}

                        <TextField
                            label={t('remoteControl.dialog.alias', { defaultValue: 'Alias' })}
                            placeholder="My Bot"
                            value={nameDraft}
                            onChange={(e) => setNameDraft(e.target.value)}
                            fullWidth
                            size="small"
                            helperText={t('remoteControl.dialog.aliasHelper', { defaultValue: 'Optional: a friendly name for this bot configuration.' })}
                            disabled={saving}
                        />

                        <TextField
                            label={t('remoteControl.dialog.proxyUrl', { defaultValue: 'Proxy URL' })}
                            placeholder="http://user:pass@host:port"
                            value={proxyDraft}
                            onChange={(e) => setProxyDraft(e.target.value)}
                            fullWidth
                            size="small"
                            helperText={t('remoteControl.dialog.proxyUrlHelper', { defaultValue: 'Optional HTTP/HTTPS proxy for bot API requests.' })}
                            disabled={saving}
                        />
                    </Stack>

                    <Stack direction="row" spacing={2} sx={{ justifyContent: 'flex-end' }}>
                        <Button onClick={onClose} color="inherit" disabled={saving}>
                            {t('remoteControl.dialog.cancel', { defaultValue: 'Cancel' })}
                        </Button>
                        <Button variant="contained" onClick={handleSave} disabled={saving}>
                            {saving
                                ? t('remoteControl.dialog.saving', { defaultValue: 'Saving...' })
                                : t('remoteControl.dialog.save', { defaultValue: 'Save Configuration' })}
                        </Button>
                    </Stack>
                </Stack>
            </Box>
        </Modal>
    );
};

export default BotConfigDialog;

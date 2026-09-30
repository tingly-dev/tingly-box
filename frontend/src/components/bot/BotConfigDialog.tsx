import BotAuthForm from './BotAuthForm';
import BotPlatformSelector from './BotPlatformSelector';
import { ExpandMore } from '@/components/icons';
import { api } from '@/services/api';
import { BOT_PLATFORM_IDS, usePlatformGuide } from '@/constants/platformGuides';
import GuideAction from '@/components/GuideAction';
import type { BotPlatformConfig, BotSettings } from '@/types/bot';
import { Accordion, AccordionDetails, AccordionSummary, Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, TextField, Typography } from '@mui/material';
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
    const [bashAllowlistDraft, setBashAllowlistDraft] = useState('');
    const [saving, setSaving] = useState(false);
    const platformGuide = usePlatformGuide(platformDraft);

    // Load platform configs once (first open).
    useEffect(() => {
        if (!open || botPlatforms.length > 0 || platformsLoading) return;
        (async () => {
            try {
                setPlatformsLoading(true);
                const data = await api.getImBotPlatforms();
                if (data?.success && data?.platforms) {
                    // The backend still registers adapters the UI doesn't
                    // support yet (Slack, Discord); only offer the platforms
                    // the rest of the UI can actually show and manage.
                    setBotPlatforms((data.platforms as BotPlatformConfig[]).filter(p =>
                        (BOT_PLATFORM_IDS as readonly string[]).includes(p.platform)));
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
                setBashAllowlistDraft((bot.bash_allowlist || []).join('\n'));
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
        setBashAllowlistDraft('');
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
                ...(dialogMode === 'edit' ? {
                    bash_allowlist: bashAllowlistDraft
                        .split(/[\n,]+/)
                        .map(command => command.trim())
                        .filter(Boolean),
                } : {}),
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
    }, [botPlatforms, platformDraft, targetUuid, authDraft, nameDraft, proxyDraft, bashAllowlistDraft, dialogMode, onSaved, onClose, notify, t]);

    return (
        <Dialog open={open} onClose={saving ? undefined : onClose} fullWidth maxWidth="sm">
            <DialogTitle>
                {dialogMode === 'edit'
                    ? t('remoteControl.dialog.editTitle', { defaultValue: 'Edit bot' })
                    : t('remoteControl.dialog.addTitle', { defaultValue: 'Connect a bot' })}
            </DialogTitle>
            <DialogContent dividers>
                <Stack spacing={2}>
                    <Typography variant="body2" color="text.secondary">
                        {dialogMode === 'edit'
                            ? t('remoteControl.dialog.editSubtitle', {defaultValue: 'Update this connection. Capabilities and people are managed from Access.'})
                            : t('remoteControl.dialog.addSubtitle', {defaultValue: 'Choose a messaging platform and provide the credentials needed to connect it.'})}
                    </Typography>
                    <Stack spacing={2}>
                        <Stack spacing={1}>
                            <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
                                <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                                    {t('remoteControl.dialog.platform', { defaultValue: 'Platform' })}
                                </Typography>
                                {/* The setup guide for whichever platform is picked —
                                    creating the bot on the platform side is the step
                                    people get stuck on, and it happens right here. */}
                                {dialogMode === 'add' && platformGuide?.guide && (
                                    <GuideAction
                                        label={t('remoteControl.guide.action', { defaultValue: 'Setup guide' })}
                                        title={t('remoteControl.guide.title', { defaultValue: '{{platform}} Setup Guide', platform: platformGuide.name })}
                                        description={t('remoteControl.guide.drawerHint', { defaultValue: 'Connection steps, credentials, and examples' })}
                                    >
                                        {platformGuide.guide}
                                    </GuideAction>
                                )}
                            </Box>
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

                        {dialogMode === 'edit' && (
                            <Accordion disableGutters elevation={0} sx={{border: 1, borderColor: 'divider', '&:before': {display: 'none'}}}>
                                <AccordionSummary expandIcon={<ExpandMore fontSize="small"/>}>
                                    <Box>
                                        <Typography variant="body2" sx={{fontWeight: 600}}>
                                            {t('remoteControl.dialog.advancedAgentPolicy', {defaultValue: 'Advanced agent policy'})}
                                        </Typography>
                                        <Typography variant="caption" color="text.secondary">
                                            {t('remoteControl.dialog.advancedAgentPolicyHelper', {defaultValue: 'Limits what an authorized controller may execute; it does not grant access.'})}
                                        </Typography>
                                    </Box>
                                </AccordionSummary>
                                <AccordionDetails>
                                    <TextField
                                        label={t('remoteControl.dialog.bashAllowlist', { defaultValue: 'Bash Allowlist' })}
                                        placeholder={'cd\nls\npwd'}
                                        value={bashAllowlistDraft}
                                        onChange={(event) => setBashAllowlistDraft(event.target.value)}
                                        fullWidth
                                        multiline
                                        minRows={3}
                                        size="small"
                                        helperText={t('remoteControl.dialog.bashAllowlistHelper', { defaultValue: 'Allowlisted /bash subcommands. Default: cd, ls, pwd.' })}
                                        disabled={saving}
                                    />
                                </AccordionDetails>
                            </Accordion>
                        )}
                    </Stack>
                </Stack>
            </DialogContent>
            <DialogActions sx={{px: 3, py: 2}}>
                <Button onClick={onClose} color="inherit" disabled={saving}>
                    {t('remoteControl.dialog.cancel', { defaultValue: 'Cancel' })}
                </Button>
                <Button variant="contained" onClick={handleSave} disabled={saving}>
                    {saving
                        ? t('remoteControl.dialog.saving', { defaultValue: 'Saving...' })
                        : dialogMode === 'edit'
                            ? t('remoteControl.dialog.save', { defaultValue: 'Save changes' })
                            : t('remoteControl.dialog.connect', {defaultValue: 'Connect bot'})}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default BotConfigDialog;

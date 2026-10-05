import { Check as IconCheck, Computer as IconComputer } from '@/components/icons';
import { Box, Button, CircularProgress, Divider, Paper, Stack, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CopyIconButton } from '@/components/CopyIconButton';
import { useNotify } from '@/hooks/useNotify.ts';
import { api } from '@/services/api.ts';
import { host } from '@/host';
import { fontMono, fontSizes } from '@/theme/fonts';

/**
 * Whether the shortcut section has anything useful to show. Wails GUI users
 * already have a native window/icon, so the whole section — header included —
 * should be skipped rather than rendered with an empty body. The caller
 * (HelpPage) checks this before rendering ShortcutCard inside its own
 * accordion header.
 */
export const shouldShowShortcutCard = () => host.kind !== 'desktop';

/**
 * ShortcutCard — content for the "create a desktop / start-menu shortcut"
 * action. Rendered inside a CollapsibleCard on HelpPage; title/description
 * live in that shared accordion header, not here.
 *
 * Re-entrant on purpose (no "done, hide the button" state): the action is
 * idempotent, so it stays available to recover a deleted shortcut, or to
 * re-point it after an upgrade or a different launch method.
 */
export const ShortcutCard = () => {
    const { t } = useTranslation();
    const notify = useNotify();
    const [shortcutStatus, setShortcutStatus] = useState<{ exists: boolean; created: string[]; scriptPath: string } | null>(null);
    const [shortcutCreating, setShortcutCreating] = useState(false);
    const [shortcutError, setShortcutError] = useState<string | null>(null);

    useEffect(() => {
        loadShortcutStatus();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const loadShortcutStatus = async () => {
        const result = await api.getShortcutStatus();
        if (result.success) {
            setShortcutStatus({
                exists: result.exists,
                created: result.data?.created ?? [],
                scriptPath: result.data?.script_path ?? '',
            });
        }
    };

    const handleCreateShortcut = async () => {
        setShortcutCreating(true);
        setShortcutError(null);
        const result = await api.createShortcut();
        if (result.success) {
            setShortcutStatus({
                exists: true,
                created: result.data?.created ?? [],
                scriptPath: result.data?.script_path ?? '',
            });
            notify.success(t('help.shortcut.title'));
        } else {
            setShortcutError(result.error || 'Unknown error');
        }
        setShortcutCreating(false);
    };

    return (
        <Stack spacing={1.5}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                <Button
                    variant="contained"
                    size="small"
                    startIcon={shortcutCreating ? <CircularProgress size={14} color="inherit" /> : <IconComputer sx={{ fontSize: 16 }} />}
                    onClick={handleCreateShortcut}
                    disabled={shortcutCreating}
                >
                    {shortcutCreating
                        ? t('help.shortcut.creating')
                        : shortcutStatus?.exists ? t('help.shortcut.recreate') : t('help.shortcut.create')}
                </Button>
                {shortcutStatus?.exists && !shortcutCreating && (
                    <Typography variant="caption" sx={{ color: 'success.main', display: 'flex', alignItems: 'center', gap: 0.5 }}>
                        <IconCheck sx={{ fontSize: 14 }} /> {t('help.shortcut.alreadyCreated')}
                    </Typography>
                )}
            </Box>

            {shortcutError && (
                <Typography variant="caption" sx={{ color: 'error.main' }}>
                    {t('help.shortcut.createFailed', { error: shortcutError })}
                </Typography>
            )}

            {shortcutStatus && shortcutStatus.created.length > 0 && (
                <Box>
                    <Divider sx={{ mb: 1.5 }} />
                    <Stack spacing={0.5} sx={{ mb: 1 }}>
                        {shortcutStatus.created.map((p) => (
                            <Typography
                                key={p}
                                variant="caption"
                                sx={{ fontFamily: fontMono, color: 'text.secondary', wordBreak: 'break-all' }}
                            >
                                {p}
                            </Typography>
                        ))}
                    </Stack>

                    {shortcutStatus.scriptPath ? (
                        <Box>
                            <Typography variant="caption" sx={{ color: 'text.secondary', display: 'block', mb: 0.5 }}>
                                {t('help.shortcut.runHeadless')}
                            </Typography>
                            <Paper
                                variant="outlined"
                                sx={{ p: 1.5, bgcolor: 'background.default', position: 'relative' }}
                            >
                                <Typography
                                    variant="body2"
                                    sx={{ fontFamily: fontMono, fontSize: fontSizes.md, pr: 5, wordBreak: 'break-all' }}
                                >
                                    $ {shortcutStatus.scriptPath}
                                </Typography>
                                <CopyIconButton
                                    value={shortcutStatus.scriptPath}
                                    label={t('common.copy')}
                                    copiedLabel={t('common.copied')}
                                    tooltipPlacement="top"
                                    tooltipArrow
                                    iconSize={16}
                                    sx={{ position: 'absolute', right: 8, top: '50%', transform: 'translateY(-50%)' }}
                                    onCopied={() => notify.success(t('common.copied'))}
                                />
                            </Paper>
                        </Box>
                    ) : (
                        <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                            {t('help.shortcut.doubleClick')}
                        </Typography>
                    )}
                </Box>
            )}
        </Stack>
    );
};

export default ShortcutCard;

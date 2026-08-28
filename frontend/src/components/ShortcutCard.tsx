import UnifiedCard from '@/components/UnifiedCard.tsx';
import { Check as IconCheck, Computer as IconComputer, ContentCopy as IconContentCopy } from '@/components/icons';
import { Box, Button, CircularProgress, Divider, IconButton, Paper, Stack, Tooltip, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNotify } from '@/hooks/useNotify.ts';
import { api } from '@/services/api.ts';
import { isGuiMode } from '@/utils/protocol.ts';
import { fontMono } from '@/theme/fonts';

interface ShortcutCardProps {
    /** Cap the card content's width; the card itself stays full-width. */
    contentMaxWidth?: number | string;
}

/**
 * ShortcutCard — "create a desktop / start-menu shortcut" action.
 *
 * Wails GUI users already have a native window/icon and don't need this;
 * the component renders nothing there. Re-entrant on purpose (no "done, hide
 * the button" state): the action is idempotent, so it stays available to
 * recover a deleted shortcut, or to re-point it after an upgrade or a
 * different launch method.
 */
export const ShortcutCard = ({ contentMaxWidth }: ShortcutCardProps) => {
    const { t } = useTranslation();
    const notify = useNotify();
    const [shortcutStatus, setShortcutStatus] = useState<{ exists: boolean; created: string[]; scriptPath: string } | null>(null);
    const [shortcutCreating, setShortcutCreating] = useState(false);
    const [shortcutError, setShortcutError] = useState<string | null>(null);
    const [copiedShortcutScript, setCopiedShortcutScript] = useState(false);
    const show = !isGuiMode();

    useEffect(() => {
        if (!show) return;
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

    const handleCopyShortcutScript = () => {
        if (!shortcutStatus?.scriptPath) return;
        navigator.clipboard.writeText(shortcutStatus.scriptPath).then(() => {
            setCopiedShortcutScript(true);
            notify.success(t('common.copied'));
            setTimeout(() => setCopiedShortcutScript(false), 2000);
        });
    };

    if (!show) return null;

    return (
        <UnifiedCard title={t('help.shortcut.title')} size="full" contentMaxWidth={contentMaxWidth}>
            <Stack spacing={1.5}>
                <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                    {t('help.shortcut.description')}
                </Typography>

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
                                        sx={{ fontFamily: fontMono, fontSize: '0.8rem', pr: 5, wordBreak: 'break-all' }}
                                    >
                                        $ {shortcutStatus.scriptPath}
                                    </Typography>
                                    <Tooltip title={copiedShortcutScript ? t('common.copied') : t('common.copy')} placement="top" arrow>
                                        <IconButton
                                            size="small"
                                            onClick={handleCopyShortcutScript}
                                            sx={{ position: 'absolute', right: 8, top: '50%', transform: 'translateY(-50%)', color: copiedShortcutScript ? 'success.main' : 'text.secondary' }}
                                        >
                                            {copiedShortcutScript ? <IconCheck sx={{ fontSize: 16 }} /> : <IconContentCopy sx={{ fontSize: 16 }} />}
                                        </IconButton>
                                    </Tooltip>
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
        </UnifiedCard>
    );
};

export default ShortcutCard;

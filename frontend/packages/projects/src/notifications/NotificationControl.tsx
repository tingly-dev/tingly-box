// The notifications control at the top of the workspace home: one quiet line,
// "Notifications" and whatever the one next step is. A switch while the
// browser permits; "Enable" while it has not been asked (the browser's dialog
// opens from that click, never on load); how to unblock when it refuses; a
// plain word when it cannot.
import { useState } from 'react';
import { Box, Button, CircularProgress, Popover, Switch, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { getReadableAccent } from '@tingly/ui/theme/status';
import { radius } from '@tingly/ui/theme/radius';
import { Bell, BellOff, Info } from '../components/icons';
import { useNotifications } from './useNotifications';

export function NotificationControl() {
    const { t } = useTranslation('projects');
    const { status, asking, turnOn, turnOff } = useNotifications();
    const [help, setHelp] = useState<HTMLElement | null>(null);
    const label = t('notifyControl.label');
    const live = status === 'on' || status === 'ask';

    return (
        <Box
            role="group"
            aria-label={label}
            sx={{ display: 'inline-flex', alignItems: 'center', gap: 1, minHeight: 36, pl: 1.25, pr: 0.75, border: 1, borderColor: 'divider', borderRadius: radius.row, flexShrink: 0, maxWidth: '100%' }}
        >
            <Tooltip title={t('notifyControl.scope')} placement="bottom-start">
                <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.75, minWidth: 0, color: live ? 'text.primary' : 'text.secondary', fontSize: fontSizes.base }}>
                    {live ? <Bell sx={{ fontSize: 18, color: status === 'on' ? 'primary.main' : 'inherit' }} /> : <BellOff sx={{ fontSize: 18 }} />}
                    {label}
                </Box>
            </Tooltip>

            {(status === 'on' || status === 'off') && (
                <Switch
                    size="small"
                    checked={status === 'on'}
                    onChange={(_, on) => (on ? void turnOn() : turnOff())}
                    slotProps={{ input: { 'aria-label': label } }}
                />
            )}

            {status === 'ask' && (
                <Button
                    size="small"
                    disabled={asking}
                    startIcon={asking ? <CircularProgress size={12} /> : undefined}
                    onClick={() => void turnOn()}
                >
                    {t('notifyControl.enable')}
                </Button>
            )}

            {status === 'blocked' && (
                <>
                    <Box component="span" sx={{ color: (theme) => getReadableAccent(theme, 'warning'), fontSize: fontSizes.sm }}>{t('notifyControl.blocked')}</Box>
                    <Button size="small" color="inherit" sx={{ color: 'text.secondary' }} onClick={(e) => setHelp(e.currentTarget)}>{t('notifyControl.howTo')}</Button>
                    <Popover
                        open={help != null}
                        anchorEl={help}
                        onClose={() => setHelp(null)}
                        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
                        slotProps={{ paper: { sx: { width: 320, maxWidth: '92vw', p: 1.5 } } }}
                    >
                        <Box sx={{ display: 'flex', gap: 0.75, alignItems: 'flex-start' }}>
                            <Info sx={{ fontSize: 16, mt: 0.25, color: 'text.secondary' }} />
                            <Box>
                                <Typography sx={{ fontSize: fontSizes.base, fontWeight: 600, color: 'text.primary' }}>{t('notifyControl.blockedTitle')}</Typography>
                                <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>{t('notifyControl.blockedBody')}</Typography>
                            </Box>
                        </Box>
                    </Popover>
                </>
            )}

            {status === 'unsupported' && (
                <Box component="span" sx={{ color: 'text.secondary', fontSize: fontSizes.sm, pr: 0.5 }}>{t('notifyControl.unsupported')}</Box>
            )}
        </Box>
    );
}

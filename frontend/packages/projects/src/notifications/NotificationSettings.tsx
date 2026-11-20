// The Notifications settings card of the standalone app (the workspace home
// carries the compact NotificationControl instead): the same status, with the
// promise it keeps spelled out.
import { Alert, Box, Button, CircularProgress, Paper, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { radius } from '@tingly/ui/theme/radius';
import { Bell } from '../components/icons';
import { useNotifications, type NotificationStatus } from './useNotifications';

const STATUS_LINE: Record<NotificationStatus, string> = {
    on: 'settings.notificationsOn',
    off: 'settings.notificationsOff',
    ask: 'settings.notificationsAsk',
    blocked: 'settings.notificationsOff',
    insecure: 'settings.notificationsOff',
    unsupported: 'settings.notificationsUnsupported',
};

export function NotificationSettingsCard() {
    const { t } = useTranslation('projects');
    const { status, asking, turnOn, turnOff, testing, sendTest } = useNotifications();
    const on = status === 'on';

    return (
        <Paper variant="outlined" sx={{ borderRadius: radius.card, p: { xs: 2, md: 3 }, mb: 3 }}>
            <Typography variant="h4">{t('settings.notificationsTitle')}</Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>{t('settings.notificationsIntro')}</Typography>
            <Box sx={{ mt: 2.5, display: 'flex', alignItems: 'center', gap: 2 }}>
                <Bell sx={{ color: on ? 'primary.main' : 'text.secondary', fontSize: 22 }} />
                <Box sx={{ flex: 1 }}>
                    <Typography sx={{ color: 'text.primary' }}>{t('settings.notificationsEnable')}</Typography>
                    <Typography variant="body2" color="text.secondary">
                        {t(STATUS_LINE[status])}
                    </Typography>
                    {on && <Typography variant="caption" color="text.secondary">{t('settings.notificationsScope')}</Typography>}
                </Box>
                {(status === 'on' || status === 'off' || status === 'ask') && (
                    <Button
                        variant="text"
                        color="inherit"
                        disabled={testing}
                        startIcon={testing ? <CircularProgress size={14} /> : undefined}
                        onClick={() => void sendTest()}
                    >
                        {t('settings.notificationsTest')}
                    </Button>
                )}
                <Button
                    variant={on ? 'contained' : 'outlined'}
                    disabled={status === 'unsupported' || status === 'insecure' || status === 'blocked' || asking}
                    startIcon={asking ? <CircularProgress size={14} /> : undefined}
                    onClick={() => (on ? turnOff() : void turnOn())}
                >
                    {on ? t('common.turnOff') : t('common.turnOn')}
                </Button>
            </Box>
            {status === 'blocked' && <Alert severity="warning" sx={{ mt: 2 }}>{t('settings.notificationsBlocked')}</Alert>}
            {status === 'insecure' && <Alert severity="warning" sx={{ mt: 2 }}>{t('settings.notificationsInsecure')}</Alert>}
        </Paper>
    );
}

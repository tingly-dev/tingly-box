// The Notifications settings card: one switch, the browser's permission
// asked at the click that turns it on (a user gesture — the only moment
// browsers accept the request), a clear word when the browser refuses or
// cannot, and a note of the promise it keeps (background only).
import { useState } from 'react';
import { Alert, Box, Button, CircularProgress, Paper, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { radius } from '@tingly/ui/theme/radius';
import { Bell } from '../components/icons';
import { notificationsEnabled, setNotificationsEnabled } from './engine';
import { notificationPermission, notificationsSupported, requestNotificationPermission } from './browser';

export function NotificationSettingsCard() {
    const { t } = useTranslation('projects');
    const supported = notificationsSupported();
    // The click's request is still out.
    const [asking, setAsking] = useState(false);
    const [enabled, setEnabled] = useState(notificationsEnabled);

    const turn = async (on: boolean) => {
        if (!on) {
            setNotificationsEnabled(false);
            setEnabled(false);
            return;
        }
        setAsking(true);
        // The browser answers with its own dialog; whatever it grants is what
        // the feature may use. Denied: the switch stays off and says where to
        // fix it — asking again would only wear the welcome out.
        const permission = await requestNotificationPermission();
        setAsking(false);
        const granted = permission === 'granted';
        setNotificationsEnabled(granted);
        setEnabled(granted);
    };

    const on = enabled && notificationPermission() === 'granted';
    const blocked = supported && notificationPermission() === 'denied';

    return (
        <Paper variant="outlined" sx={{ borderRadius: radius.card, p: { xs: 2, md: 3 }, mb: 3 }}>
            <Typography variant="h4">{t('settings.notificationsTitle')}</Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>{t('settings.notificationsIntro')}</Typography>
            <Box sx={{ mt: 2.5, display: 'flex', alignItems: 'center', gap: 2 }}>
                <Bell sx={{ color: on ? 'primary.main' : 'text.secondary', fontSize: 22 }} />
                <Box sx={{ flex: 1 }}>
                    <Typography sx={{ color: 'text.primary' }}>{t('settings.notificationsEnable')}</Typography>
                    <Typography variant="body2" color="text.secondary">
                        {supported ? t(on ? 'settings.notificationsOn' : 'settings.notificationsOff') : t('settings.notificationsUnsupported')}
                    </Typography>
                    {on && <Typography variant="caption" color="text.secondary">{t('settings.notificationsScope')}</Typography>}
                </Box>
                <Button
                    variant={on ? 'contained' : 'outlined'}
                    disabled={!supported || asking}
                    startIcon={asking ? <CircularProgress size={14} /> : undefined}
                    onClick={() => void turn(!on)}
                >
                    {on ? t('common.turnOff') : t('common.turnOn')}
                </Button>
            </Box>
            {blocked && <Alert severity="warning" sx={{ mt: 2 }}>{t('settings.notificationsBlocked')}</Alert>}
        </Paper>
    );
}

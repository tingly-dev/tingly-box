import { Alert, AlertTitle, Button, Collapse, IconButton, Stack, Typography } from '@mui/material';
import { Close, GitHub, ShoppingCart } from '@mui/icons-material';
import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { useVersion } from '../contexts/VersionContext';

export const UpdateNotification: React.FC = () => {
    const { t } = useTranslation();
    const { showNotification, currentVersion, latestVersion, releaseURL, checking } = useVersion();
    const [open, setOpen] = useState(true);

    // Reset open state when showNotification changes from false to true
    useEffect(() => {
        if (showNotification) {
            setOpen(true);
        }
    }, [showNotification]);

    if (!showNotification || !open) {
        return null;
    }

    const handleGitHubRelease = () => {
        window.open('https://github.com/tingly-dev/tingly-box/releases', '_blank', 'noopener,noreferrer');
    };

    const handleNpm = () => {
        window.open('https://www.npmjs.com/package/tingly-box', '_blank', 'noopener,noreferrer');
    };

    return (
        <Collapse in={open}>
            <Alert
                severity="info"
                action={
                    <IconButton
                        aria-label="close"
                        color="inherit"
                        size="small"
                        onClick={() => setOpen(false)}
                    >
                        <Close fontSize="inherit" />
                    </IconButton>
                }
                sx={{ mb: 2 }}
            >
                <AlertTitle>{t('update.newVersionAvailable')}</AlertTitle>
                <Typography variant="body2" sx={{ mb: 1 }}>
                    {t('update.versionAvailable', { latest: latestVersion, current: currentVersion })}
                </Typography>
                <Typography variant="body2" component="div" sx={{ fontFamily: 'monospace', mb: 1.5 }}>
                    npx tingly-box@latest
                </Typography>
                <Stack direction="row" spacing={1}>
                    <Button
                        color="inherit"
                        size="small"
                        startIcon={<GitHub />}
                        onClick={handleGitHubRelease}
                        disabled={checking}
                    >
                        GitHub
                    </Button>
                    <Button
                        color="inherit"
                        size="small"
                        startIcon={<ShoppingCart />}
                        onClick={handleNpm}
                        disabled={checking}
                    >
                        npm
                    </Button>
                </Stack>
            </Alert>
        </Collapse>
    );
};

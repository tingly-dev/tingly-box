import { Box, IconButton, Tooltip } from '@mui/material';
import { IconAlertCircle, IconStar } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { useHealth } from '../contexts/HealthContext';
import { useVersion as useAppVersion } from '../contexts/VersionContext';
import { Z_INDEX } from '../constants/zIndex';

export const FloatingStatusIndicators = () => {
    const { t } = useTranslation();
    const { hasUpdate, showUpdateDialog } = useAppVersion();
    const { isHealthy, showDisconnectDialog } = useHealth();

    const showError = !isHealthy || import.meta.env.DEV;
    const showUpdate = hasUpdate || import.meta.env.DEV;

    if (!showError && !showUpdate) return null;

    return (
        <Box
            sx={{
                position: 'fixed',
                bottom: 16,
                right: 16,
                display: 'flex',
                flexDirection: 'column',
                gap: 1,
                zIndex: Z_INDEX.popover,
            }}
        >
            {showError && (
                <Tooltip
                    title={
                        import.meta.env.DEV && isHealthy
                            ? t('layout.activityBar.disconnectedDebug')
                            : t('layout.activityBar.disconnected')
                    }
                    placement="left"
                    arrow
                >
                    <IconButton
                        onClick={showDisconnectDialog}
                        size="small"
                        sx={{
                            width: 40,
                            height: 40,
                            bgcolor: 'background.paper',
                            color: 'error.main',
                            border: '1px solid',
                            borderColor: 'divider',
                            boxShadow: 2,
                            '&:hover': {
                                bgcolor: 'action.hover',
                                color: 'error.dark',
                            },
                        }}
                    >
                        <IconAlertCircle size={20} />
                    </IconButton>
                </Tooltip>
            )}

            {showUpdate && (
                <Tooltip
                    title={
                        import.meta.env.DEV && !hasUpdate
                            ? t('layout.activityBar.devMode')
                            : t('layout.activityBar.newVersionAvailable')
                    }
                    placement="left"
                    arrow
                >
                    <IconButton
                        onClick={showUpdateDialog}
                        size="small"
                        sx={{
                            width: 40,
                            height: 40,
                            bgcolor: 'background.paper',
                            color: import.meta.env.DEV && !hasUpdate ? 'success.main' : 'info.main',
                            border: '1px solid',
                            borderColor: 'divider',
                            boxShadow: 2,
                            '&:hover': {
                                bgcolor: 'action.hover',
                            },
                        }}
                    >
                        <IconStar size={20} />
                    </IconButton>
                </Tooltip>
            )}
        </Box>
    );
};

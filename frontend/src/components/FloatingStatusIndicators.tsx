import { Box, IconButton, Tooltip } from '@mui/material';
import { Error as IconAlertCircle } from '@/components/icons';
import { useTranslation } from 'react-i18next';
import { useHealth } from '../contexts/HealthContext';
import { useVersion as useAppVersion } from '../contexts/VersionContext';
import { Z_INDEX } from '../constants/zIndex';

export const FloatingStatusIndicators = () => {
    const { t } = useTranslation();
    const { hasUpdate } = useAppVersion();
    const { isHealthy, showDisconnectDialog } = useHealth();

    const showError = !isHealthy;
    const showUpdate = hasUpdate || import.meta.env.DEV;

    if (!showError && !showUpdate) return null;

    return (
        <Box
            sx={{
                // Desktop shows this in the rail (layout/ActivityBar); only the
                // mobile layout, whose rail is inside a drawer, floats it.
                display: { xs: 'flex', md: 'none' },
                position: 'fixed',
                top: { xs: 8, md: 'auto' },
                right: { xs: 8, md: 16 },
                bottom: { xs: 'auto', md: 16 },
                flexDirection: { xs: 'row', md: 'column' },
                gap: 1,
                zIndex: Z_INDEX.popover,
            }}
        >
            {showError && (
                <Tooltip
                    title={t('layout.activityBar.disconnected')}
                    placement="left"
                    arrow
                >
                    <IconButton
                        onClick={showDisconnectDialog}
                        size="small"
                        sx={{
                            width: { xs: 44, md: 40 },
                            height: { xs: 44, md: 40 },
                            bgcolor: { xs: 'transparent', md: 'background.paper' },
                            color: 'error.main',
                            border: { xs: 'none', md: '1px solid' },
                            borderColor: 'divider',
                            boxShadow: { xs: 0, md: 2 },
                            '&:hover': {
                                bgcolor: 'action.hover',
                                color: 'error.dark',
                            },
                        }}
                    >
                        <IconAlertCircle sx={{ fontSize: 20 }} />
                    </IconButton>
                </Tooltip>
            )}
        </Box>
    );
};

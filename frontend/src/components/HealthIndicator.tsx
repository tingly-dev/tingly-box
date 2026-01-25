import { Box, Tooltip, Typography, CircularProgress } from '@mui/material';
import { CheckCircle, Error, Refresh } from '@mui/icons-material';
import { useTranslation } from 'react-i18next';
import { useHealth } from '../contexts/HealthContext';

export const HealthIndicator: React.FC = () => {
    const { t } = useTranslation();
    const { isHealthy, lastCheck, checking, checkHealth } = useHealth();

    const formatLastCheck = (date: Date | null): string => {
        if (!date) return t('health.never');
        return date.toLocaleTimeString();
    };

    return (
        <Tooltip title={t('health.lastChecked', { time: formatLastCheck(lastCheck) })}>
            <Box
                sx={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 0.5,
                    cursor: 'pointer',
                    px: 1,
                    py: 0.5,
                    borderRadius: 1,
                    transition: 'background-color 0.2s',
                    '&:hover': {
                        backgroundColor: 'action.hover',
                    },
                }}
                onClick={checkHealth}
            >
                {checking ? (
                    <CircularProgress size={16} thickness={2} />
                ) : isHealthy ? (
                    <CheckCircle color="success" fontSize="small" />
                ) : (
                    <Error color="error" fontSize="small" />
                )}
                <Typography variant="caption" color="text.secondary">
                    {checking ? t('health.checking') : isHealthy ? t('health.connected') : t('health.disconnected')}
                </Typography>
                <Refresh
                    sx={{
                        fontSize: 14,
                        color: 'text.secondary',
                        opacity: 0.6,
                        transition: 'opacity 0.2s',
                        '&:hover': {
                            opacity: 1,
                        },
                    }}
                />
            </Box>
        </Tooltip>
    );
};

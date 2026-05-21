import { Box, Card, CardContent, Typography, Tooltip, Divider } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { SCENARIOS, useHiddenScenarios } from '@/pages/scenario/AgentOverviewPage';

const QUICK_NAV_ICON_SIZE = 20;

const AgentQuickNav: React.FC = () => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { isHidden } = useHiddenScenarios();

    const visibleScenarios = SCENARIOS.filter((s) => !isHidden(s.id));

    return (
        <Card
            sx={{
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
                boxShadow: 'none',
                height: '100%',
                display: 'flex',
                flexDirection: 'column',
            }}
        >
            <CardContent sx={{ p: 2, height: '100%', display: 'flex', flexDirection: 'column' }}>
                {/* Header */}
                <Box sx={{ mb: 1.5 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, fontSize: '0.8rem' }}>
                        {t('dashboard.agentNav.title', { defaultValue: 'Quick Start' })}
                    </Typography>
                    <Typography variant="caption" sx={{ color: 'text.secondary', fontSize: '0.7rem' }}>
                        {t('dashboard.agentNav.description', { defaultValue: 'Jump to agent' })}
                    </Typography>
                </Box>

                <Divider sx={{ mb: 1.5 }} />

                {/* Agent List */}
                <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', gap: 0.5 }}>
                    {visibleScenarios.map((s) => (
                        <Tooltip
                            key={s.id}
                            title={t(s.descKey)}
                            arrow
                            placement="right"
                        >
                            <Box
                                onClick={() => navigate(s.path)}
                                sx={{
                                    display: 'flex',
                                    alignItems: 'center',
                                    gap: 1,
                                    py: 1.25,
                                    px: 1,
                                    borderRadius: 1.25,
                                    cursor: 'pointer',
                                    transition: 'background-color 0.18s ease-out, color 0.18s ease-out, border-color 0.18s ease-out, transform 0.18s ease-out',
                                    border: '1px solid transparent',
                                    position: 'relative',
                                    color: 'text.secondary',
                                    '&:hover': {
                                        bgcolor: 'primary.main',
                                        color: 'primary.contrastText',
                                        borderColor: 'primary.light',
                                        transform: 'translateX(4px)',
                                        '& .MuiTypography-root': {
                                            color: 'primary.contrastText',
                                        },
                                        '& svg': {
                                            filter: 'none !important',
                                        },
                                    },
                                }}
                            >
                                {/* Icon */}
                                <Box
                                    sx={{
                                        display: 'flex',
                                        alignItems: 'center',
                                        justifyContent: 'center',
                                        width: 32,
                                        height: 32,
                                        borderRadius: 1,
                                        bgcolor: 'action.hover',
                                        flexShrink: 0,
                                    }}
                                >
                                    {s.icon(QUICK_NAV_ICON_SIZE)}
                                </Box>

                                {/* Label */}
                                <Typography
                                    variant="caption"
                                    sx={{
                                        fontWeight: 500,
                                        fontSize: '0.75rem',
                                        color: 'text.primary',
                                        flex: 1,
                                        lineHeight: 1.3,
                                    }}
                                >
                                    {t(s.labelKey)}
                                </Typography>
                            </Box>
                        </Tooltip>
                    ))}
                </Box>
            </CardContent>
        </Card>
    );
};

export default AgentQuickNav;

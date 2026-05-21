// Shared components for TokenHistoryChart

import { Box, Typography, useTheme } from '@mui/material';
import { getThemeChartStyles } from '../chartStyles';
import type { LegendItemProps, ChartDataPoint } from './types';

export function LegendItem({ label, color, visible, onToggle }: LegendItemProps) {
    return (
        <Box
            onClick={onToggle}
            sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 1.5,
                cursor: 'pointer',
                userSelect: 'none',
                opacity: visible ? 1 : 0.4,
                transition: 'opacity 0.18s ease-out, background-color 0.18s ease-out',
                px: 1.5,
                py: 0.5,
                borderRadius: 1.5,
                '&:hover': {
                    opacity: visible ? 0.8 : 0.5,
                    backgroundColor: 'action.hover',
                },
            }}
        >
            <Box
                sx={{
                    width: 14,
                    height: 14,
                    borderRadius: 2.5,
                    backgroundColor: color,
                    border: '2px solid',
                    borderColor: visible ? color : 'transparent',
                }}
            />
            <Typography
                variant="caption"
                sx={{
                    fontSize: '0.8rem',
                    color: 'text.secondary',
                    fontWeight: 500,
                }}
            >
                {label}
            </Typography>
        </Box>
    );
}

export function CustomTooltip({ active, payload }: any) {
    const theme = useTheme();
    const chartStyles = getThemeChartStyles(theme);

    if (!active || !payload || !payload.length) return null;

    const data = payload[0].payload as ChartDataPoint;
    return (
        <Box
            sx={{
                backgroundColor: 'background.paper',
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 2,
                p: 2,
                boxShadow: 'none',
            }}
        >
            <Typography
                variant="caption"
                sx={{ fontWeight: 600, color: 'text.primary', display: 'block', mb: 1 }}
            >
                {data.timeFull}
            </Typography>
            {payload.map((entry: any, index: number) => (
                <Box
                    key={index}
                    sx={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        gap: 2,
                        mb: 0.5,
                    }}
                >
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Box
                            sx={{
                                width: 10,
                                height: 10,
                                borderRadius: 2,
                                backgroundColor: entry.color,
                            }}
                        />
                        <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                            {entry.name}:
                        </Typography>
                    </Box>
                    <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.primary' }}>
                        {entry.value.toLocaleString()}
                    </Typography>
                </Box>
            ))}
        </Box>
    );
}

interface ChartWrapperProps {
    title: string;
    hasData: boolean;
    children: React.ReactNode;
}

export function ChartWrapper({ title, hasData, children }: ChartWrapperProps) {
    const theme = useTheme();
    const chartStyles = getThemeChartStyles(theme);

    return (
        <Box
            sx={{
                p: 3,
                borderRadius: 3,
                border: '1px solid',
                borderColor: 'divider',
                flexGrow: 1,
                backgroundColor: 'background.paper',
                boxShadow: 'none',
                display: 'flex',
                flexDirection: 'column',
            }}
        >
            <Box sx={{ mb: 2.5 }}>
                <Typography
                    variant="h6"
                    sx={{
                        fontWeight: 600,
                        fontSize: '1rem',
                        letterSpacing: '-0.01em',
                        color: 'text.primary',
                    }}
                >
                    {title}
                </Typography>
            </Box>
            {!hasData ? (
                <Box
                    sx={{
                        flex: 1,
                        minHeight: 280,
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        justifyContent: 'center',
                        color: 'text.secondary',
                    }}
                >
                    <Box
                        sx={{
                            width: 48,
                            height: 48,
                            borderRadius: 2,
                            backgroundColor: chartStyles.statCard.emptyIconBg,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            mb: 2,
                        }}
                    >
                        <Box
                            sx={{
                                width: 24,
                                height: 24,
                                borderRadius: '50%',
                                backgroundColor: 'text.disabled',
                                opacity: 0.3,
                            }}
                        />
                    </Box>
                    <Typography variant="body1" color="text.secondary">
                        No data available
                    </Typography>
                    <Typography variant="caption" color="text.disabled" sx={{ mt: 0.5 }}>
                        Select a different time range or check back later
                    </Typography>
                </Box>
            ) : (
                <Box sx={{ flex: 1, minHeight: 280 }}>
                    {children}
                </Box>
            )}
        </Box>
    );
}

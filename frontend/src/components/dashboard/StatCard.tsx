import { Box, Paper, Typography, alpha, useTheme } from '@mui/material';
import type { ReactNode } from 'react';

interface StatCardProps {
    title: string;
    value: string | number;
    subtitle?: string;
    icon?: ReactNode;
    color?: 'primary' | 'success' | 'info' | 'warning' | 'error' | 'secondary';
}

export default function StatCard({ title, value, subtitle, icon, color = 'primary' }: StatCardProps) {
    const theme = useTheme();

    // Get theme-aware colors for stat cards
    const getColorMap = () => {
        const palette = theme.palette as any;

        // For sunlit theme, use sky blue tones
        if (palette.isSunlit) {
            return {
                primary: { bg: 'rgba(14, 165, 233, 0.1)', text: '#0ea5e9', hover: 'rgba(14, 165, 233, 0.15)' },
                success: { bg: 'rgba(34, 197, 94, 0.1)', text: '#22c55e', hover: 'rgba(34, 197, 94, 0.15)' },
                info: { bg: 'rgba(6, 182, 212, 0.1)', text: '#06b6d4', hover: 'rgba(6, 182, 212, 0.15)' },
                warning: { bg: 'rgba(245, 158, 11, 0.1)', text: '#f59e0b', hover: 'rgba(245, 158, 11, 0.15)' },
                error: { bg: 'rgba(239, 68, 68, 0.1)', text: '#ef4444', hover: 'rgba(239, 68, 68, 0.15)' },
                secondary: { bg: 'rgba(99, 102, 241, 0.1)', text: '#6366f1', hover: 'rgba(99, 102, 241, 0.15)' },
            };
        }

        // For dark theme
        if (palette.mode === 'dark') {
            return {
                primary: { bg: 'rgba(96, 165, 250, 0.15)', text: '#60A5FA', hover: 'rgba(96, 165, 250, 0.25)' },
                success: { bg: 'rgba(52, 211, 153, 0.15)', text: '#34D399', hover: 'rgba(52, 211, 153, 0.25)' },
                info: { bg: 'rgba(14, 165, 233, 0.15)', text: '#0ea5e9', hover: 'rgba(14, 165, 233, 0.25)' },
                warning: { bg: 'rgba(245, 158, 11, 0.15)', text: '#f59e0b', hover: 'rgba(245, 158, 11, 0.25)' },
                error: { bg: 'rgba(248, 113, 113, 0.15)', text: '#f87171', hover: 'rgba(248, 113, 113, 0.25)' },
                secondary: { bg: 'rgba(148, 163, 184, 0.15)', text: '#94a3b8', hover: 'rgba(148, 163, 184, 0.25)' },
            };
        }

        // Default light theme
        return {
            primary: { bg: 'rgba(37, 99, 235, 0.08)', text: '#2563eb', hover: 'rgba(37, 99, 235, 0.12)' },
            success: { bg: 'rgba(5, 150, 105, 0.08)', text: '#059669', hover: 'rgba(5, 150, 105, 0.12)' },
            info: { bg: 'rgba(14, 165, 233, 0.08)', text: '#0ea5e9', hover: 'rgba(14, 165, 233, 0.12)' },
            warning: { bg: 'rgba(245, 158, 11, 0.08)', text: '#f59e0b', hover: 'rgba(245, 158, 11, 0.12)' },
            error: { bg: 'rgba(220, 38, 38, 0.08)', text: '#dc2626', hover: 'rgba(220, 38, 38, 0.12)' },
            secondary: { bg: 'rgba(100, 116, 139, 0.08)', text: '#64748b', hover: 'rgba(100, 116, 139, 0.12)' },
        };
    };

    const colorMap = getColorMap();
    const colors = colorMap[color];
    const hoverBgAlpha = theme.palette.mode === 'dark' ? 0.12 : 0.075;
    const baseBgAlpha = theme.palette.mode === 'dark' ? 0.045 : 0.025;

    return (
        <Paper
            elevation={0}
            sx={{
                p: 2,
                borderRadius: 2,
                border: '1px solid',
                borderColor: alpha(colors.text, 0.18),
                height: '100%',
                transition: 'border-color 0.18s ease-out, background-color 0.18s ease-out',
                backgroundColor: alpha(colors.text, baseBgAlpha),
                boxShadow: 'none',
                position: 'relative',
                overflow: 'hidden',
                '&:hover': {
                    borderColor: alpha(colors.text, 0.55),
                    backgroundColor: alpha(colors.text, hoverBgAlpha),
                },
            }}
        >
            <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
                <Box
                    sx={{
                        display: 'grid',
                        gridTemplateColumns: 'minmax(0, 1fr) 28px',
                        alignItems: 'flex-start',
                        gap: 1,
                        mb: 1,
                        minHeight: '2.7em',
                    }}
                >
                    <Typography
                        variant="caption"
                        sx={{
                            fontWeight: 600,
                            color: 'text.secondary',
                            fontSize: '0.8125rem',
                            lineHeight: 1.35,
                            display: '-webkit-box',
                            WebkitLineClamp: 2,
                            WebkitBoxOrient: 'vertical',
                            overflow: 'hidden',
                            overflowWrap: 'normal',
                        }}
                    >
                        {title}
                    </Typography>
                    {icon && (
                        <Box
                            sx={{
                                width: 28,
                                height: 28,
                                borderRadius: 1.5,
                                backgroundColor: colors.bg,
                                color: colors.text,
                                display: 'flex',
                                alignItems: 'center',
                                justifyContent: 'center',
                                flexShrink: 0,
                                opacity: 0.9,
                                transition: 'background-color 0.18s ease-out, color 0.18s ease-out, opacity 0.18s ease-out',
                                '.MuiPaper-root:hover &': {
                                    backgroundColor: colors.text,
                                    color: '#fff',
                                    opacity: 1,
                                },
                                '& svg': {
                                    fontSize: 16,
                                },
                            }}
                        >
                            {icon}
                        </Box>
                    )}
                </Box>
                <Typography
                    variant="h4"
                    sx={{
                        fontWeight: 700,
                        fontSize: { xs: '1.375rem', sm: '1.5rem' },
                        lineHeight: 1.2,
                        color: 'text.primary',
                        mb: 0.25,
                        fontVariantNumeric: 'tabular-nums',
                    }}
                >
                    {value}
                </Typography>
                {subtitle && (
                    <Typography
                        variant="caption"
                        sx={{
                            color: 'text.secondary',
                            fontSize: '0.75rem',
                            whiteSpace: 'pre-line',
                            lineHeight: 1.3,
                        }}
                    >
                        {subtitle}
                    </Typography>
                )}
            </Box>
        </Paper>
    );
}

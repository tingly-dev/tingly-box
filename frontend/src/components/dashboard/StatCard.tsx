import { Box, Paper, Typography, alpha, useTheme } from '@mui/material';
import type { ReactNode } from 'react';
import { fontSizes } from '@/theme/fonts';
import { getReadableAccent } from '@/theme/status';

interface StatCardProps {
    title: string;
    value: string | number;
    subtitle?: string;
    icon?: ReactNode;
    // Health indicator instead of an icon: a small dot in the accent colour.
    statusDot?: boolean;
    color?: 'primary' | 'success' | 'info' | 'warning' | 'error' | 'secondary';
}

export default function StatCard({ title, value, subtitle, icon, statusDot, color = 'primary' }: StatCardProps) {
    const theme = useTheme();

    // Accent comes from the active palette, so every theme (incl. claude/ds)
    // gets its own status colours; getReadableAccent keeps it legible on the card.
    const colors = { text: getReadableAccent(theme, color) };
    const iconBgAlpha = theme.palette.mode === 'dark' ? 0.15 : 0.08;
    // Colour is for status only: a neutral card, and the value takes the
    // accent only when the metric carries a health judgment that is not "fine".
    const judged = statusDot && color !== 'success' && color !== 'secondary';

    return (
        <Paper
            elevation={0}
            sx={{
                p: 1.5,
                px: 2,
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
                height: '100%',
                transition: 'border-color 0.18s ease-out',
                backgroundColor: 'background.paper',
                boxShadow: 'none',
                position: 'relative',
                overflow: 'hidden',
                '&:hover': { borderColor: 'text.disabled' },
            }}
        >
            <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
                <Box
                    sx={{
                        display: 'grid',
                        gridTemplateColumns: icon ? 'minmax(0, 1fr) 28px' : 'minmax(0, 1fr) auto',
                        alignItems: 'flex-start',
                        gap: 1,
                        mb: 0.5,
                        minHeight: '1.4em',
                    }}
                >
                    <Typography
                        variant="caption"
                        sx={{
                            fontWeight: 600,
                            color: 'text.secondary',
                            fontSize: fontSizes.md,
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
                                backgroundColor: alpha(colors.text, iconBgAlpha),
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
                    {!icon && statusDot && (
                        <Box
                            aria-hidden="true"
                            sx={{ width: 10, height: 10, mt: 0.5, borderRadius: '50%', backgroundColor: colors.text }}
                        />
                    )}
                </Box>
                <Typography
                    variant="h4"
                    sx={{
                        fontWeight: 700,
                        fontSize: { xs: '1.375rem', sm: '1.5rem' },
                        lineHeight: 1.2,
                        color: judged ? colors.text : 'text.primary',
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
                            fontSize: fontSizes.sm,
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

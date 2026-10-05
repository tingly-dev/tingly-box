import { Box, Paper, Typography, alpha, useTheme } from '@mui/material';
import type { ReactNode } from 'react';
import { fontSizes } from '@/theme/fonts';
import { getReadableAccent } from '@/theme/status';

interface StatCardProps {
    title: string;
    value: string | number;
    subtitle?: string;
    icon?: ReactNode;
    color?: 'primary' | 'success' | 'info' | 'warning' | 'error' | 'secondary';
}

export default function StatCard({ title, value, subtitle, icon, color = 'primary' }: StatCardProps) {
    const theme = useTheme();

    // Accent comes from the active palette, so every theme (incl. claude/ds)
    // gets its own status colours; getReadableAccent keeps it legible on the card.
    const colors = { text: getReadableAccent(theme, color) };
    const iconBgAlpha = theme.palette.mode === 'dark' ? 0.15 : 0.08;
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
                transition: 'border-color 0.18s ease-out, background-image 0.18s ease-out',
                // Layered on top of the theme's own Paper backdrop (solid on most
                // themes, translucent-over-gradient on ds) rather than replacing it —
                // otherwise this low-alpha tint reveals whatever's behind the page
                // itself instead of reading as a flat color, which is invisible on a
                // solid page background but shows through visibly on ds's gradient one.
                backgroundColor: 'background.paper',
                backgroundImage: `linear-gradient(${alpha(colors.text, baseBgAlpha)}, ${alpha(colors.text, baseBgAlpha)})`,
                boxShadow: 'none',
                position: 'relative',
                overflow: 'hidden',
                '&:hover': {
                    borderColor: alpha(colors.text, 0.55),
                    backgroundImage: `linear-gradient(${alpha(colors.text, hoverBgAlpha)}, ${alpha(colors.text, hoverBgAlpha)})`,
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

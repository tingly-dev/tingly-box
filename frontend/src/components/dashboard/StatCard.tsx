import { Box, Paper, Typography, alpha } from '@mui/material';
import type { ReactNode } from 'react';

interface StatCardProps {
    title: string;
    value: string | number;
    subtitle?: string;
    icon?: ReactNode;
    color?: 'primary' | 'success' | 'info' | 'warning' | 'error' | 'secondary';
}

export default function StatCard({ title, value, subtitle, icon, color = 'primary' }: StatCardProps) {
    const colorMap = {
        primary: { bg: 'rgba(37, 99, 235, 0.08)', text: '#2563eb', hover: 'rgba(37, 99, 235, 0.12)' },
        success: { bg: 'rgba(5, 150, 105, 0.08)', text: '#059669', hover: 'rgba(5, 150, 105, 0.12)' },
        info: { bg: 'rgba(14, 165, 233, 0.08)', text: '#0ea5e9', hover: 'rgba(14, 165, 233, 0.12)' },
        warning: { bg: 'rgba(245, 158, 11, 0.08)', text: '#f59e0b', hover: 'rgba(245, 158, 11, 0.12)' },
        error: { bg: 'rgba(220, 38, 38, 0.08)', text: '#dc2626', hover: 'rgba(220, 38, 38, 0.12)' },
        secondary: { bg: 'rgba(100, 116, 139, 0.08)', text: '#64748b', hover: 'rgba(100, 116, 139, 0.12)' },
    };

    const colors = colorMap[color];

    return (
        <Paper
            elevation={0}
            sx={{
                p: 2,
                pl: 2.5,
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
                height: '100%',
                transition: 'all 0.2s ease-in-out',
                backgroundColor: 'background.paper',
                boxShadow: '0 2px 4px rgba(0, 0, 0, 0.1)',
                position: 'relative',
                overflow: 'hidden',
                '&:hover': {
                    borderColor: alpha(colors.text, 0.3),
                    boxShadow: '0 4px 12px rgba(0, 0, 0, 0.15)',
                    transform: 'translateY(-1px)',
                },
                '&::before': {
                    content: '""',
                    position: 'absolute',
                    left: 0,
                    top: 0,
                    bottom: 0,
                    width: 3,
                    background: `linear-gradient(180deg, ${colors.text} 0%, ${alpha(colors.text, 0.6)} 100%)`,
                },
            }}
        >
            <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', pl: 0.5 }}>
                <Typography
                    variant="caption"
                    sx={{
                        fontWeight: 600,
                        color: 'text.secondary',
                        textTransform: 'uppercase',
                        letterSpacing: '0.05em',
                        fontSize: '0.7rem',
                        lineHeight: 1.3,
                        whiteSpace: 'pre-line',
                        minHeight: '1.7em',
                        mb: 0.5,
                    }}
                >
                    {title}
                </Typography>
                <Typography
                    variant="h4"
                    sx={{
                        fontWeight: 700,
                        fontSize: '1.5rem',
                        lineHeight: 1.2,
                        color: 'text.primary',
                        mb: 0.25,
                    }}
                >
                    {value}
                </Typography>
                {subtitle && (
                    <Typography
                        variant="caption"
                        sx={{
                            color: 'text.secondary',
                            fontSize: '0.7rem',
                            whiteSpace: 'pre-line',
                            lineHeight: 1.3,
                        }}
                    >
                        {subtitle}
                    </Typography>
                )}
                {icon && (
                    <Box
                        sx={{
                            position: 'absolute',
                            top: 8,
                            right: 8,
                            width: 24,
                            height: 24,
                            borderRadius: 1.5,
                            backgroundColor: colors.bg,
                            color: colors.text,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            opacity: 0.7,
                            transition: 'all 0.2s ease',
                            '.MuiPaper-root:hover &': {
                                backgroundColor: colors.hover,
                                opacity: 1,
                                transform: 'scale(1.1)',
                            },
                            '& svg': {
                                fontSize: 14,
                            },
                        }}
                    >
                        {icon}
                    </Box>
                )}
            </Box>
        </Paper>
    );
}

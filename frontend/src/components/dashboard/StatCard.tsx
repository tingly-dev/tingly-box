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
                p: 3,
                borderRadius: 2.5,
                border: '1px solid',
                borderColor: 'divider',
                height: '100%',
                transition: 'all 0.2s ease-in-out',
                backgroundColor: 'background.paper',
                boxShadow: '0 1px 3px 0 rgba(0, 0, 0, 0.1), 0 1px 2px 0 rgba(0, 0, 0, 0.06)',
                '&:hover': {
                    borderColor: alpha(colors.text, 0.3),
                    boxShadow: '0 4px 12px rgba(0, 0, 0, 0.08)',
                    transform: 'translateY(-2px)',
                },
                position: 'relative',
                overflow: 'hidden',
            }}
        >
            {/* Decorative gradient bar */}
            <Box
                sx={{
                    position: 'absolute',
                    top: 0,
                    left: 0,
                    right: 0,
                    height: 3,
                    background: `linear-gradient(90deg, ${colors.text} 0%, ${alpha(colors.text, 0.3)} 100%)`,
                }}
            />
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mt: 1 }}>
                <Box sx={{ flex: 1 }}>
                    <Typography
                        variant="body2"
                        sx={{
                            mb: 2,
                            fontWeight: 600,
                            color: 'text.secondary',
                            textTransform: 'uppercase',
                            letterSpacing: '0.05em',
                            fontSize: '0.75rem',
                        }}
                    >
                        {title}
                    </Typography>
                    <Typography
                        variant="h3"
                        sx={{
                            fontWeight: 700,
                            mb: 0.5,
                            fontSize: '1.75rem',
                            lineHeight: 1.2,
                            color: 'text.primary',
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
                            }}
                        >
                            {subtitle}
                        </Typography>
                    )}
                </Box>
                {icon && (
                    <Box
                        sx={{
                            p: 1.75,
                            borderRadius: 2.5,
                            backgroundColor: colors.bg,
                            color: colors.text,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            transition: 'all 0.2s ease',
                            '.MuiPaper-root:hover &': {
                                backgroundColor: colors.hover,
                                transform: 'scale(1.05)',
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

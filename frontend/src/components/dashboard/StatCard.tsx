import { Box, Paper, Typography } from '@mui/material';
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
        primary: { bg: '#e3f2fd', text: '#1976d2' },
        success: { bg: '#e8f5e9', text: '#2e7d32' },
        info: { bg: '#e1f5fe', text: '#0288d1' },
        warning: { bg: '#fff3e0', text: '#f57c00' },
        error: { bg: '#ffebee', text: '#d32f2f' },
        secondary: { bg: '#f3e5f5', text: '#7b1fa2' },
    };

    const colors = colorMap[color];

    return (
        <Paper
            elevation={0}
            sx={{
                p: 3,
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
                height: '100%',
            }}
        >
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <Box>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                        {title}
                    </Typography>
                    <Typography variant="h4" sx={{ fontWeight: 600, mb: 0.5 }}>
                        {value}
                    </Typography>
                    {subtitle && (
                        <Typography variant="body2" color="text.secondary">
                            {subtitle}
                        </Typography>
                    )}
                </Box>
                {icon && (
                    <Box
                        sx={{
                            p: 1.5,
                            borderRadius: 2,
                            backgroundColor: colors.bg,
                            color: colors.text,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                        }}
                    >
                        {icon}
                    </Box>
                )}
            </Box>
        </Paper>
    );
}

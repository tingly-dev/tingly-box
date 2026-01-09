import { Box, Paper, Typography } from '@mui/material';
import type { ReactNode } from 'react';

interface StatCardProps {
    title: string;
    value: string | number;
    subtitle?: string;
    icon?: ReactNode;
    color?: 'primary' | 'success' | 'warning' | 'error' | 'info';
}

const colorMap = {
    primary: '#2563eb',
    success: '#059669',
    warning: '#d97706',
    error: '#dc2626',
    info: '#0891b2',
};

export default function StatCard({ title, value, subtitle, icon, color = 'primary' }: StatCardProps) {
    return (
        <Paper
            elevation={0}
            sx={{
                p: 3,
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 2,
                height: '100%',
            }}
        >
            <Box sx={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between' }}>
                <Box>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                        {title}
                    </Typography>
                    <Typography
                        variant="h4"
                        sx={{
                            fontWeight: 600,
                            color: colorMap[color],
                        }}
                    >
                        {value}
                    </Typography>
                    {subtitle && (
                        <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5, display: 'block' }}>
                            {subtitle}
                        </Typography>
                    )}
                </Box>
                {icon && (
                    <Box
                        sx={{
                            p: 1,
                            borderRadius: 1,
                            bgcolor: `${colorMap[color]}15`,
                            color: colorMap[color],
                        }}
                    >
                        {icon}
                    </Box>
                )}
            </Box>
        </Paper>
    );
}

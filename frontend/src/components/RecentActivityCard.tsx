import { Box, Button, Stack, Typography } from '@mui/material';
import UnifiedCard from './UnifiedCard';

interface RecentActivityCardProps {
    recentActivity: any[];
}

const RecentActivityCard = ({ recentActivity }: RecentActivityCardProps) => {
    return (
        <UnifiedCard
            title="Recent Activity"
            subtitle="Latest system actions and events"
            size="medium"
        >
            <Stack spacing={1}>
                <Button
                    variant="outlined"
                    onClick={() => window.location.href = '/history'}
                    sx={{ mb: 2 }}
                >
                    View Full History
                </Button>
                <Box
                    sx={{
                        flex: 1,
                        overflowY: 'auto',
                        border: '1px solid',
                        borderColor: 'divider',
                        borderRadius: 1,
                        p: 1.5,
                        backgroundColor: 'grey.50',
                        minHeight: 120,
                    }}
                >
                    {recentActivity.length > 0 ? (
                        recentActivity.map((entry, index) => (
                            <Box key={index} mb={0.5}>
                                <Typography variant="caption" sx={{ fontSize: '0.75rem' }}>
                                    {new Date(entry.timestamp).toLocaleTimeString()}{' '}
                                    {entry.success ? 'Success' : 'Failed'}: {entry.action}
                                </Typography>
                            </Box>
                        ))
                    ) : (
                        <Typography color="text.secondary">No recent activity</Typography>
                    )}
                </Box>
            </Stack>
        </UnifiedCard>
    );
};

export default RecentActivityCard;

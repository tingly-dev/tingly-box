import { CardGrid, CardGridItem } from './CardGrid';
import UnifiedCard from './UnifiedCard';
import { Typography, Box } from '@mui/material';

interface HistoryStatsProps {
    stats: {
        total: number;
        success: number;
        error: number;
        today: number;
    };
}

const HistoryStats = ({ stats }: HistoryStatsProps) => {
    return (
        <UnifiedCard
            size="header"
            width="100%"
            title="Statistics Overview" subtitle="Overall action statistics">
            <Box sx={{ display: 'flex', justifyContent: 'space-around', flexWrap: 'wrap', gap: 3 }}>
                <Box sx={{ textAlign: 'center', minWidth: 100 }}>
                    <Typography variant="h4" color="primary" sx={{ fontWeight: 'bold' }}>
                        {stats.total}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Total
                    </Typography>
                </Box>
                <Box sx={{ textAlign: 'center', minWidth: 100 }}>
                    <Typography variant="h4" color="success.main" sx={{ fontWeight: 'bold' }}>
                        {stats.success}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Successful
                    </Typography>
                </Box>
                <Box sx={{ textAlign: 'center', minWidth: 100 }}>
                    <Typography variant="h4" color="error.main" sx={{ fontWeight: 'bold' }}>
                        {stats.error}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Failed
                    </Typography>
                </Box>
                <Box sx={{ textAlign: 'center', minWidth: 100 }}>
                    <Typography variant="h4" color="info.main" sx={{ fontWeight: 'bold' }}>
                        {stats.today}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Today
                    </Typography>
                </Box>
            </Box>
        </UnifiedCard>
    );
};

export default HistoryStats;
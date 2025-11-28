import { Button, Stack, Typography } from '@mui/material';
import UnifiedCard from './UnifiedCard';

interface ProvidersSummaryCardProps {
    providersStatus: any;
}

const ProvidersSummaryCard = ({ providersStatus }: ProvidersSummaryCardProps) => {
    return (
        <UnifiedCard
            title="Providers"
            subtitle="Overview of configured providers"
            size="medium"
        >
            {providersStatus ? (
                <Stack spacing={2}>
                    <Typography variant="body2">
                        <strong>Total Providers:</strong> {providersStatus.length}
                    </Typography>
                    <Typography variant="body2">
                        <strong>Enabled:</strong> {providersStatus.filter((p: any) => p.enabled).length}
                    </Typography>
                    <Button
                        variant="contained"
                        onClick={() => window.location.href = '/providers'}
                    >
                        Manage Providers
                    </Button>
                </Stack>
            ) : (
                <Typography color="text.secondary">Loading...</Typography>
            )}
        </UnifiedCard>
    );
};

export default ProvidersSummaryCard;

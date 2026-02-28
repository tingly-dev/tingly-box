import { AutoAwesome } from '@mui/icons-material';
import { Box, Typography } from '@mui/material';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';

const AgentPage = () => {
    return (
        <PageLayout>
            <UnifiedCard
                title="Agents"
                subtitle="AI agent configuration"
                size="full"
            >
                <Box textAlign="center" py={8} width="100%">
                    <Box
                        sx={{
                            display: 'flex',
                            justifyContent: 'center',
                            mb: 3,
                        }}
                    >
                        <AutoAwesome sx={{ fontSize: 64, color: 'text.disabled' }} />
                    </Box>
                    <Typography variant="h5" sx={{ fontWeight: 600, mb: 2 }}>
                        Agent Configuration
                    </Typography>
                    <Typography variant="body1" color="text.secondary" sx={{ maxWidth: 500, mx: 'auto' }}>
                        Configure and manage AI agents for remote control. This feature is coming soon.
                    </Typography>
                </Box>
            </UnifiedCard>
        </PageLayout>
    );
};

export default AgentPage;

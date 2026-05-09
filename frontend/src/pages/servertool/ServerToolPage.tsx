import { PageLayout } from '@/components/PageLayout';
import { Box, Typography } from '@mui/material';
import { IconTools } from '@tabler/icons-react';

const ServerToolPage = () => {
    return (
        <PageLayout loading={false}>
            <Box
                sx={{
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    minHeight: 320,
                    gap: 2,
                    color: 'text.disabled',
                }}
            >
                <IconTools size={48} stroke={1.2} />
                <Typography variant="h6" color="text.secondary" fontWeight={600}>
                    Server Tool
                </Typography>
                <Typography variant="body2" color="text.disabled">
                    Coming soon
                </Typography>
            </Box>
        </PageLayout>
    );
};

export default ServerToolPage;

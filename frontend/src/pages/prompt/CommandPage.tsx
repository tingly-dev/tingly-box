import { Box, Typography } from '@mui/material';
import PageLayout from '@/components/PageLayout';
import { useTranslation } from 'react-i18next';

const CommandPage = () => {
  const { t } = useTranslation();

  return (
    <PageLayout loading={false}>
      <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>
        <Typography variant="h4" sx={{ fontWeight: 600, mb: 2, color: 'text.primary' }}>
          Commands
        </Typography>
        <Typography variant="body1" color="text.secondary">
          Command management feature coming soon...
        </Typography>
      </Box>
    </PageLayout>
  );
};

export default CommandPage;

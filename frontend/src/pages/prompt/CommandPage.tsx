import { Box, Typography } from '@mui/material';
import PageLayout from '@/components/PageLayout';
import { useTranslation } from 'react-i18next';

const CommandPage = () => {
  const { t } = useTranslation();

  return (
    <PageLayout loading={false}>
      {/* minHeight (not height: 100%): PageLayout grows with content now, so a
          percentage height no longer resolves — pin a viewport-based minimum
          to keep the placeholder vertically centered. */}
      <Box sx={{ minHeight: '60vh', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>
        <Typography component="h1" variant="h4" sx={{ fontWeight: 600, mb: 2, color: 'text.primary' }}>
          Commands
        </Typography>
        <Typography variant="body1" sx={{
          color: "text.secondary"
        }}>
          Command management feature coming soon...
        </Typography>
      </Box>
    </PageLayout>
  );
};

export default CommandPage;

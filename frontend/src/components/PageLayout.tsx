import { CircularProgress, Box, Alert, IconButton } from '@mui/material';

interface PageLoadingProps {
  minHeight?: number;
}

export const PageLoading: React.FC<PageLoadingProps> = ({ minHeight = 400 }) => {
  return (
    <Box display="flex" justifyContent="center" alignItems="center" minHeight={minHeight}>
      <CircularProgress />
    </Box>
  );
};

interface NotificationConfig {
  open: boolean;
  message?: string;
  severity?: 'success' | 'info' | 'warning' | 'error';
  autoHideDuration?: number;
  customContent?: React.ReactNode;
  onClose?: () => void;
}

interface PageLayoutProps {
  loading: boolean;
  children: React.ReactNode;
  loadingMinHeight?: number;
  notification?: NotificationConfig;
}

export const PageLayout: React.FC<PageLayoutProps> = ({
  loading,
  children,
  loadingMinHeight = 400,
  notification,
}) => {
  if (loading) {
    return <PageLoading minHeight={loadingMinHeight} />;
  }

  return (
    <Box position="relative" minHeight="100vh">
      <Box>{children}</Box>

      {/* Unified Notification System */}
      {notification?.open && (
        <Box
          sx={{
            position: 'fixed',
            bottom: 24,
            right: 24,
            width: 'calc(100% - 32px)',
            maxWidth: '600px',
            display: 'flex',
            justifyContent: 'flex-end',
            zIndex: (theme) => theme.zIndex.snackbar,
          }}
        >
          {notification.customContent ? (
            <Alert
              severity={notification.severity || 'info'}
              sx={{
                mb: 0,
                width: '100%',
                '& .MuiAlert-message': {
                  width: '100%'
                }
              }}
              action={
                <IconButton
                  aria-label="close"
                  color="inherit"
                  size="small"
                  onClick={notification.onClose}
                >
                  ×
                </IconButton>
              }
            >
              {notification.customContent}
            </Alert>
          ) : (
            <Alert
              severity={notification.severity || 'info'}
              onClose={notification.onClose}
              sx={{
                mb: 0,
                width: '100%',
              }}
            >
              {notification.message}
            </Alert>
          )}
        </Box>
      )}
    </Box>
  );
};

export default PageLayout;
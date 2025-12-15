import { CircularProgress, Box, Alert, IconButton } from '@mui/material';
import { useEffect, useRef } from 'react';

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
  const timeoutRef = useRef<NodeJS.Timeout | null>(null);

  // Auto-hide notification after specified duration
  useEffect(() => {
    if (notification?.open && notification.autoHideDuration && notification.autoHideDuration > 0) {
      timeoutRef.current = setTimeout(() => {
        if (notification.onClose) {
          notification.onClose();
        }
      }, notification.autoHideDuration);
    }

    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
        timeoutRef.current = null;
      }
    };
  }, [notification?.open, notification?.autoHideDuration, notification?.onClose]);

  // Close notification on page refresh/unload
  useEffect(() => {
    const handleBeforeUnload = () => {
      if (notification?.open && notification.onClose) {
        notification.onClose();
      }
    };

    window.addEventListener('beforeunload', handleBeforeUnload);

    return () => {
      window.removeEventListener('beforeunload', handleBeforeUnload);
    };
  }, [notification?.open, notification?.onClose]);

  if (loading) {
    return <PageLoading minHeight={loadingMinHeight} />;
  }

  return (
    <Box
      position="relative"
      sx={{
        width: '100%',
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      <Box sx={{ flex: 1 }}>{children}</Box>

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
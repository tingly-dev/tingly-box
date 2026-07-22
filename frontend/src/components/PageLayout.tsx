import { CircularProgress, Box, Alert, IconButton } from '@mui/material';
import { useEffect, useRef, useState } from 'react';

type TimerId = ReturnType<typeof setTimeout>;

// A loading indicator (spinner or skeleton) shown for less than this only
// reads as a flash/glitch, not as "loading" — especially a skeleton, whose
// grey placeholder blocks contrast much more with real content than a
// spinner does. Below this threshold we show nothing at all rather than
// pop the indicator in and immediately back out.
const LOADING_SHOW_DELAY_MS = 200;

interface PageLoadingProps {
  minHeight?: number;
}

export const PageLoading: React.FC<PageLoadingProps> = ({ minHeight = 400 }) => {
  return (
    <Box
      sx={{
        display: "flex",
        justifyContent: "center",
        alignItems: "center",
        minHeight: minHeight
      }}>
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
  /**
   * Rendered instead of the default centered spinner while `loading` is
   * true. Use a skeleton that mirrors the page's real layout (see
   * ScenarioPageSkeleton) to avoid a jarring blank-to-content pop once
   * data arrives.
   */
  loadingContent?: React.ReactNode;
  notification?: NotificationConfig;
  title?: string;
  subtitle?: string;
  rightAction?: React.ReactNode;
}

export const PageLayout: React.FC<PageLayoutProps> = ({
  loading,
  children,
  loadingMinHeight = 400,
  loadingContent,
  notification,
  title,
  subtitle,
  rightAction,
}) => {
  const timeoutRef = useRef<TimerId | null>(null);

  // Only start showing the loading state once `loading` has been true for
  // LOADING_SHOW_DELAY_MS. If the data arrives before that, nothing is
  // ever shown — a brief blank moment reads as instant, while a skeleton
  // that appears and disappears within a couple hundred ms reads as a
  // visual glitch.
  const [showLoading, setShowLoading] = useState(false);
  useEffect(() => {
    if (!loading) {
      setShowLoading(false);
      return;
    }
    const t = setTimeout(() => setShowLoading(true), LOADING_SHOW_DELAY_MS);
    return () => clearTimeout(t);
  }, [loading]);

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
    if (!showLoading) return null;
    return loadingContent ? <>{loadingContent}</> : <PageLoading minHeight={loadingMinHeight} />;
  }

  return (
    <Box
      sx={{
        position: "relative",
        width: '100%',
        height: '100%',
        display: 'flex',
        flexDirection: 'column'
      }}>
      {(title || subtitle || rightAction) && (
        <Box
          sx={{
            display: 'flex',
            alignItems: 'flex-start',
            justifyContent: 'space-between',
            gap: 2,
            mb: 2,
          }}>
          <Box sx={{ minWidth: 0 }}>
            {title && (
              <Box sx={{ typography: 'h6', fontWeight: 600 }}>{title}</Box>
            )}
            {subtitle && (
              <Box sx={{ typography: 'body2', color: 'text.secondary', mt: title ? 0.5 : 0 }}>
                {subtitle}
              </Box>
            )}
          </Box>
          {rightAction && <Box sx={{ flexShrink: 0 }}>{rightAction}</Box>}
        </Box>
      )}
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
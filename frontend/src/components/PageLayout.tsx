import { CircularProgress, Box, Alert } from '@mui/material';
import type { AlertProps } from '@mui/material';

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

interface PageMessageProps extends AlertProps {
  message?: { type: 'success' | 'error'; text: string } | null;
  onClearMessage?: () => void;
}

export const PageMessage: React.FC<PageMessageProps> = ({
  message,
  onClearMessage,
  ...alertProps
}) => {
  if (!message) return null;

  return (
    <Alert
      severity={message.type}
      sx={{ mb: 2 }}
      onClose={onClearMessage}
      {...alertProps}
    >
      {message.text}
    </Alert>
  );
};

interface PageLayoutProps {
  loading: boolean;
  children: React.ReactNode;
  message?: { type: 'success' | 'error'; text: string } | null;
  onClearMessage?: () => void;
  loadingMinHeight?: number;
}

export const PageLayout: React.FC<PageLayoutProps> = ({
  loading,
  children,
  message,
  onClearMessage,
  loadingMinHeight = 400,
}) => {
  if (loading) {
    return <PageLoading minHeight={loadingMinHeight} />;
  }

  return (
    <Box>
      <PageMessage message={message} onClearMessage={onClearMessage} />
      {children}
    </Box>
  );
};

export default PageLayout;
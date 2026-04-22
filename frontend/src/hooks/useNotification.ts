/**
 * Shared hook for snackbar/notification state management.
 * Standardizes notification patterns across 30+ files.
 */
import { useState, useCallback, type Dispatch, type SetStateAction } from 'react';

export interface NotificationState {
  open: boolean;
  message: string;
  severity: 'success' | 'error' | 'warning' | 'info';
}

export interface UseNotificationResult {
  notification: NotificationState;
  showNotification: (message: string, severity?: NotificationState['severity']) => void;
  showSuccess: (message: string) => void;
  showError: (message: string) => void;
  showWarning: (message: string) => void;
  showInfo: (message: string) => void;
  hideNotification: () => void;
  setNotification: Dispatch<SetStateAction<NotificationState>>;
}

export function useNotification(): UseNotificationResult {
  const [notification, setNotification] = useState<NotificationState>({
    open: false,
    message: '',
    severity: 'info',
  });

  const showNotification = useCallback((
    message: string,
    severity: NotificationState['severity'] = 'info'
  ) => {
    setNotification({ open: true, message, severity });
  }, []);

  const showSuccess = useCallback((message: string) => {
    showNotification(message, 'success');
  }, [showNotification]);

  const showError = useCallback((message: string) => {
    showNotification(message, 'error');
  }, [showNotification]);

  const showWarning = useCallback((message: string) => {
    showNotification(message, 'warning');
  }, [showNotification]);

  const showInfo = useCallback((message: string) => {
    showNotification(message, 'info');
  }, [showNotification]);

  const hideNotification = useCallback(() => {
    setNotification(prev => ({ ...prev, open: false }));
  }, []);

  return {
    notification,
    showNotification,
    showSuccess,
    showError,
    showWarning,
    showInfo,
    hideNotification,
    setNotification,
  };
}

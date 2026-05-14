/**
 * Backward-compatible shim over the global notification system (src/lib/notify.ts).
 *
 * Existing callers keep the same `showSuccess` / `showError` / ... API, but the
 * notifications now render through the unified NotificationProvider. The returned
 * `notification` object is always closed — components no longer need to feed it
 * into PageLayout or a local Snackbar.
 *
 * New code should prefer `useNotify` from './useNotify' directly.
 */
import { useCallback, useMemo, type Dispatch, type SetStateAction } from 'react';
import { notify } from '@/utils/notify';

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

const CLOSED_STATE: NotificationState = { open: false, message: '', severity: 'info' };

export function useNotification(): UseNotificationResult {
  const showNotification = useCallback(
    (message: string, severity: NotificationState['severity'] = 'info') => {
      notify.show(severity, message);
    },
    [],
  );
  const showSuccess = useCallback((message: string) => notify.success(message), []);
  const showError = useCallback((message: string) => notify.error(message), []);
  const showWarning = useCallback((message: string) => notify.warning(message), []);
  const showInfo = useCallback((message: string) => notify.info(message), []);
  const noop = useCallback(() => {}, []);

  return useMemo(
    () => ({
      notification: CLOSED_STATE,
      showNotification,
      showSuccess,
      showError,
      showWarning,
      showInfo,
      hideNotification: noop,
      setNotification: noop as Dispatch<SetStateAction<NotificationState>>,
    }),
    [showNotification, showSuccess, showError, showWarning, showInfo, noop],
  );
}

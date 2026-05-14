/**
 * useNotify — the standard way to raise notifications from React components.
 *
 * Notifications are rendered globally by NotificationProvider, so there is no
 * state to wire up: just call `success`, `error`, `warning`, `info`, or `notify`.
 */
import { useMemo } from 'react';
import { notify, type NotifyOptions, type NotifySeverity } from '@/utils/notify';

export interface UseNotifyResult {
  notify: (severity: NotifySeverity, message: string, options?: NotifyOptions) => string;
  success: (message: string, options?: NotifyOptions) => string;
  error: (message: string, options?: NotifyOptions) => string;
  warning: (message: string, options?: NotifyOptions) => string;
  info: (message: string, options?: NotifyOptions) => string;
  dismiss: (id: string) => void;
  clear: () => void;
}

export function useNotify(): UseNotifyResult {
  return useMemo(
    () => ({
      notify: notify.show,
      success: notify.success,
      error: notify.error,
      warning: notify.warning,
      info: notify.info,
      dismiss: notify.dismiss,
      clear: notify.clear,
    }),
    [],
  );
}

export default useNotify;

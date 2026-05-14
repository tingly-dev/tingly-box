/**
 * NotificationProvider renders the global, antd-style notification stack.
 *
 * Mount it once near the app root. Notifications are pushed through the
 * `useNotify` hook or the `notify` singleton (see src/lib/notify.ts) — callers
 * never need to wire notification state into their own components.
 */
import { useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from 'react';
import { Alert, AlertTitle, Box, Collapse, Slide } from '@mui/material';
import {
  type NotifyItem,
  dismissNotify,
  getNotifyItems,
  subscribeNotify,
} from '@/utils/notify';

const EXIT_TRANSITION_MS = 200;

function useNotifyItems(): NotifyItem[] {
  return useSyncExternalStore(subscribeNotify, getNotifyItems, getNotifyItems);
}

function NotificationToast({ item }: { item: NotifyItem }) {
  const [open, setOpen] = useState(false);
  const removeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Trigger the enter transition once mounted.
  useEffect(() => {
    setOpen(true);
  }, []);

  useEffect(() => {
    return () => {
      if (removeTimerRef.current) clearTimeout(removeTimerRef.current);
    };
  }, []);

  const handleClose = () => {
    setOpen(false);
    if (removeTimerRef.current) clearTimeout(removeTimerRef.current);
    removeTimerRef.current = setTimeout(() => dismissNotify(item.id), EXIT_TRANSITION_MS);
  };

  // Auto-dismiss after the item's duration (0 keeps it until closed manually).
  useEffect(() => {
    if (!item.duration || item.duration <= 0) return;
    const timer = setTimeout(handleClose, item.duration);
    return () => clearTimeout(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [item.duration]);

  return (
    <Collapse in={open} appear>
      <Box sx={{ mb: 1.5 }}>
        <Slide direction="left" in={open} appear>
          <Alert
            severity={item.severity}
            variant="filled"
            onClose={handleClose}
            sx={{
              width: '100%',
              boxShadow: 6,
              alignItems: 'flex-start',
              '& .MuiAlert-message': { overflowWrap: 'anywhere', minWidth: 0 },
            }}
          >
            {item.title && <AlertTitle sx={{ fontWeight: 600 }}>{item.title}</AlertTitle>}
            {item.message}
          </Alert>
        </Slide>
      </Box>
    </Collapse>
  );
}

function NotificationStack() {
  const items = useNotifyItems();
  if (items.length === 0) return null;
  return (
    <Box
      sx={{
        position: 'fixed',
        top: 24,
        right: 24,
        zIndex: (theme) => theme.zIndex.snackbar + 1,
        width: 'calc(100% - 48px)',
        maxWidth: 400,
        display: 'flex',
        flexDirection: 'column',
        pointerEvents: 'none',
        '& > *': { pointerEvents: 'auto' },
      }}
    >
      {items.map((item) => (
        <NotificationToast key={item.id} item={item} />
      ))}
    </Box>
  );
}

export function NotificationProvider({ children }: { children: ReactNode }) {
  return (
    <>
      {children}
      <NotificationStack />
    </>
  );
}

export default NotificationProvider;

/**
 * NotificationProvider renders the global, antd-style notification stack.
 *
 * Mount it once near the app root. Notifications are pushed through the
 * `useNotify` hook or the `notify` singleton (see src/lib/notify.ts) — callers
 * never need to wire notification state into their own components.
 */
import { useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from 'react';
import { Alert, AlertTitle, Box, Collapse, IconButton, Link, Slide } from '@mui/material';
import { Close } from '@/components/icons';
import { CopyIconButton } from '@/components/CopyIconButton';
import {
  type NotifyItem,
  dismissNotify,
  getNotifyItems,
  subscribeNotify,
} from '@/utils/notify';

const EXIT_TRANSITION_MS = 200;

// Above this, a toast collapses behind a "Show more" toggle instead of
// dumping its full text in the user's face.
const LONG_MESSAGE_CHARS = 240;
const COLLAPSED_LINES = 4;

function isLongMessage(message: string): boolean {
  return message.length > LONG_MESSAGE_CHARS || message.split('\n').length > COLLAPSED_LINES;
}

function useNotifyItems(): NotifyItem[] {
  return useSyncExternalStore(subscribeNotify, getNotifyItems, getNotifyItems);
}

function NotificationToast({ item }: { item: NotifyItem }) {
  const [open, setOpen] = useState(false);
  const [expanded, setExpanded] = useState(false);
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

  const isError = item.severity === 'error';
  // What lands on the clipboard when the user copies an error for a report.
  const reportText = item.title ? `${item.title}\n${item.message}` : item.message;
  const long = isLongMessage(item.message);

  return (
    <Collapse in={open} appear>
      <Box sx={{ mb: 1.5 }}>
        <Slide direction="left" in={open} appear>
          <Alert
            severity={item.severity}
            variant="filled"
            onClose={isError ? undefined : handleClose}
            action={
              isError ? (
                <>
                  <CopyIconButton
                    value={reportText}
                    label="Copy for report"
                    copiedLabel="Copied"
                    color="inherit"
                    copiedColor="inherit"
                    aria-label="copy error"
                  />
                  <IconButton
                    aria-label="close"
                    color="inherit"
                    size="small"
                    onClick={handleClose}
                  >
                    <Close fontSize="small" />
                  </IconButton>
                </>
              ) : undefined
            }
            sx={{
              width: '100%',
              boxShadow: 6,
              alignItems: 'flex-start',
              '& .MuiAlert-message': { overflowWrap: 'anywhere', minWidth: 0 },
            }}
          >
            {item.title && <AlertTitle sx={{ fontWeight: 600 }}>{item.title}</AlertTitle>}
            <Box
              sx={
                long && !expanded
                  ? {
                      display: '-webkit-box',
                      WebkitLineClamp: COLLAPSED_LINES,
                      WebkitBoxOrient: 'vertical',
                      overflow: 'hidden',
                    }
                  : { whiteSpace: 'pre-wrap' }
              }
            >
              {item.message}
            </Box>
            {long && (
              <Link
                component="button"
                type="button"
                color="inherit"
                underline="always"
                onClick={() => setExpanded((v) => !v)}
                sx={{ fontSize: '0.75rem', mt: 0.5, display: 'block', opacity: 0.85 }}
              >
                {expanded ? 'Show less' : 'Show more'}
              </Link>
            )}
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

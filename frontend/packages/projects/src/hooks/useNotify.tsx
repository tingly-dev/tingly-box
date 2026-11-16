import { createContext, useCallback, useContext, useState, type ReactNode } from 'react';
import { Alert, Snackbar } from '@mui/material';

export type Severity = 'success' | 'error' | 'info' | 'warning';
export type Notify = (message: string, severity?: Severity) => void;

const Ctx = createContext<Notify>(() => {});

/** Global toast host; use `useNotify()` instead of a local Snackbar. */
export function NotificationProvider({ children }: { children: ReactNode }) {
    const [state, setState] = useState<{ message: string; severity: Severity; key: number } | null>(null);
    const notify = useCallback<Notify>((message, severity = 'info') => {
        setState({ message, severity, key: Date.now() });
    }, []);
    return (
        <Ctx.Provider value={notify}>
            {children}
            <Snackbar
                key={state?.key}
                open={!!state}
                autoHideDuration={state?.severity === 'error' ? 8000 : 3500}
                onClose={(_, reason) => reason !== 'clickaway' && setState(null)}
                anchorOrigin={{ vertical: 'top', horizontal: 'center' }}
            >
                {state ? (
                    <Alert severity={state.severity} variant="filled" onClose={() => setState(null)} sx={{ maxWidth: 560 }}>
                        {state.message}
                    </Alert>
                ) : undefined}
            </Snackbar>
        </Ctx.Provider>
    );
}

/** Routes the pages' toasts to a host's own notifications instead of this Snackbar. */
export function HostNotifyProvider({ notify, children }: { notify: Notify; children: ReactNode }) {
    return <Ctx.Provider value={notify}>{children}</Ctx.Provider>;
}

export const useNotify = () => useContext(Ctx);

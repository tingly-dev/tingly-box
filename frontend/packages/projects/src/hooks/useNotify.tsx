import { createContext, useCallback, useContext, useState, type ReactNode } from 'react';
import { Alert, Snackbar } from '@mui/material';

type Severity = 'success' | 'error' | 'info' | 'warning';
type Notify = (message: string, severity?: Severity) => void;

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

export const useNotify = () => useContext(Ctx);

// Small pieces shared by the Tasks pages: the status chip, relative time,
// and the polling hook that keeps a session and its event log fresh.
//
// Polling, not SSE, on purpose for now: the browser EventSource cannot carry
// the Authorization header the control plane requires, and the backend's
// own stream is a 1s poll today. The `after` cursor is the same on both
// endpoints, so switching to a fetch-streamed SSE later is a transport
// change, not a page change.
import {useCallback, useEffect, useRef, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {Chip, FormControl, InputLabel, ListItemText, MenuItem, Select, Typography, type ChipProps, type SxProps, type Theme} from '@mui/material';
import {formatDistanceToNowStrict} from 'date-fns';
import {agentApi, isActiveStatus, type AgentEvent, type AgentSession, type AgentWorkspace, type PermissionMode} from '@/services/agentApi';

export const STATUS_COLOR: Record<string, ChipProps['color']> = {
    queued: 'default',
    running: 'info',
    waiting_input: 'warning',
    idle: 'success',
    done: 'success',
    failed: 'error',
    archived: 'default',
};

export const StatusChip = ({status, size = 'small'}: {status: string; size?: ChipProps['size']}) => {
    const {t} = useTranslation();
    return (
        <Chip
            size={size}
            color={STATUS_COLOR[status] ?? 'default'}
            variant={status === 'running' || status === 'waiting_input' ? 'filled' : 'outlined'}
            label={t(`tasks.status.${status}`, {defaultValue: status})}
        />
    );
};

export const relativeTime = (iso?: string): string => {
    if (!iso) return '';
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return '';
    return formatDistanceToNowStrict(d, {addSuffix: true});
};

export const shortId = (id?: string): string => (id ? id.replace(/-/g, '').slice(0, 8) : '');

// Poll cadence follows what the user is waiting for: fast while the agent
// is active, slow (but not off — IM can wake it) once it settles.
const ACTIVE_INTERVAL_MS = 1500;
const SETTLED_INTERVAL_MS = 8000;

export interface SessionPoll {
    session?: AgentSession;
    workspace?: AgentWorkspace;
    events: AgentEvent[];
    loading: boolean;
    error?: string;
    notFound: boolean;
    refresh: () => Promise<void>;
    setSession: (s: AgentSession) => void;
}

export const useSessionPoll = (sessionId: string | undefined): SessionPoll => {
    const [session, setSession] = useState<AgentSession>();
    const [workspace, setWorkspace] = useState<AgentWorkspace>();
    const [events, setEvents] = useState<AgentEvent[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string>();
    const [notFound, setNotFound] = useState(false);
    const cursor = useRef(0);
    const inFlight = useRef(false);
    // The poll loop reads the latest status through a ref: the effect that
    // owns the timer must not re-run on every status change (that would
    // reset the cursor), yet the cadence has to follow the live status.
    const statusRef = useRef<string | undefined>(undefined);

    const refresh = useCallback(async () => {
        if (!sessionId || inFlight.current) return;
        inFlight.current = true;
        try {
            const [detail, page] = await Promise.all([
                agentApi.getSession(sessionId),
                agentApi.listEvents(sessionId, cursor.current),
            ]);
            if (!detail.ok) {
                if (detail.status === 404) setNotFound(true);
                setError(detail.error);
                return;
            }
            setError(undefined);
            statusRef.current = detail.data.session.status;
            setSession(detail.data.session);
            setWorkspace(detail.data.workspace);
            if (page.ok && page.data.events.length > 0) {
                cursor.current = page.data.next;
                setEvents((prev) => [...prev, ...page.data.events]);
            }
        } finally {
            inFlight.current = false;
            setLoading(false);
        }
    }, [sessionId]);

    useEffect(() => {
        cursor.current = 0;
        setEvents([]);
        setSession(undefined);
        setLoading(true);
        setNotFound(false);
        let timer: ReturnType<typeof setTimeout> | undefined;
        let stopped = false;
        statusRef.current = undefined;
        const tick = async () => {
            await refresh();
            if (stopped) return;
            const status = statusRef.current;
            const active = status === undefined || isActiveStatus(status);
            timer = setTimeout(tick, active ? ACTIVE_INTERVAL_MS : SETTLED_INTERVAL_MS);
        };
        tick();
        return () => {
            stopped = true;
            if (timer) clearTimeout(timer);
        };
    }, [sessionId, refresh]);

    return {session, workspace, events, loading, error, notFound, refresh, setSession};
};

// The modes in the order the backend lists them; the empty value inherits
// the settings file's defaultMode. Labels and one-line descriptions come
// from i18n so the picker teaches what each mode does (ux §8).
export const PERMISSION_MODES: PermissionMode[] = ['', 'default', 'acceptEdits', 'auto', 'plan', 'dontAsk', 'bypassPermissions'];

export const permissionModeKey = (m: PermissionMode | string | undefined): string => (m ? m : 'inherit');

interface PermissionModeSelectProps {
    value: PermissionMode | string;
    onChange: (mode: PermissionMode) => void;
    // inheritLabel names what "" means in this context (e.g. the environment's mode).
    inheritHint?: string;
    disabled?: boolean;
    size?: 'small' | 'medium';
    sx?: SxProps<Theme>;
}

export const PermissionModeSelect = ({value, onChange, inheritHint, disabled, size = 'small', sx}: PermissionModeSelectProps) => {
    const {t} = useTranslation();
    return (
        <FormControl size={size} disabled={disabled} sx={sx}>
            <InputLabel id="permission-mode">{t('tasks.mode.label')}</InputLabel>
            <Select
                labelId="permission-mode"
                label={t('tasks.mode.label')}
                value={value ?? ''}
                displayEmpty
                onChange={(e) => onChange(e.target.value as PermissionMode)}
                renderValue={(v) => {
                    const key = permissionModeKey(v as string);
                    return key === 'inherit' && inheritHint ? `${t('tasks.mode.inherit')} · ${inheritHint}` : t(`tasks.mode.${key}`);
                }}
            >
                {PERMISSION_MODES.map((m) => {
                    const key = permissionModeKey(m);
                    return (
                        <MenuItem key={key} value={m}>
                            <ListItemText
                                primary={t(`tasks.mode.${key}`)}
                                secondary={<Typography variant="caption" color="text.secondary">{t(`tasks.mode.${key}Help`)}</Typography>}
                            />
                        </MenuItem>
                    );
                })}
            </Select>
        </FormControl>
    );
};

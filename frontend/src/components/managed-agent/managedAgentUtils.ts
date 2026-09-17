import type {MessageInfo, SessionInfo} from '@/services/managedAgentApi';

export type SessionStatus = SessionInfo['status'];

export const STATUS_COLOR: Record<string, 'default' | 'info' | 'success' | 'error' | 'warning'> = {
    pending: 'info',
    running: 'info',
    completed: 'success',
    failed: 'error',
    expired: 'default',
    closed: 'default',
};

export const isBusyStatus = (status: string): boolean => status === 'running' || status === 'pending';

// A pending request is an approval_request / ask_request whose request_id
// has no later *_response entry — the one thing on screen the user can
// still act on (ux-principles #11: hand over the artifact for the next
// action, don't just report a message went by).
export const findPendingRequest = (messages: MessageInfo[]): MessageInfo | undefined => {
    const answered = new Set<string>();
    for (const m of messages) {
        if ((m.kind === 'approval_response' || m.kind === 'ask_response') && m.request_id) {
            answered.add(m.request_id);
        }
    }
    for (let i = messages.length - 1; i >= 0; i--) {
        const m = messages[i];
        if ((m.kind === 'approval_request' || m.kind === 'ask_request') && m.request_id && !answered.has(m.request_id)) {
            return m;
        }
    }
    return undefined;
};

export const folderName = (path: string): string => path.split('/').filter(Boolean).pop() || path;

export const timeAgo = (iso: string, now = Date.now()): string => {
    const t = new Date(iso).getTime();
    if (Number.isNaN(t)) return '';
    const seconds = Math.max(0, Math.round((now - t) / 1000));
    if (seconds < 5) return 'just now';
    if (seconds < 60) return `${seconds}s ago`;
    const minutes = Math.round(seconds / 60);
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.round(minutes / 60);
    if (hours < 24) return `${hours}h ago`;
    const days = Math.round(hours / 24);
    return `${days}d ago`;
};

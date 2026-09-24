import type {MessageInfo, SessionInfo} from '@/services/deskApi';

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

export {timeAgo} from '@/utils/timeAgo';

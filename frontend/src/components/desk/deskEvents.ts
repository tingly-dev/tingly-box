import type {SessionInfo} from '@/services/deskApi';

// Desk's page and the Projects sidebar (which lists Desk's working folders)
// are mounted apart, so they keep each other current through window events
// rather than a shared store.

/** Desk loaded a fresh session list; detail is SessionInfo[]. */
export const DESK_SESSIONS_EVENT = 'tingly:desk-sessions';
/** A working-folder shortcut was added or removed in this tab. */
export const DESK_FOLDERS_EVENT = 'tingly:desk-folders-changed';

export const publishDeskSessions = (sessions: SessionInfo[]) => {
    window.dispatchEvent(new CustomEvent<SessionInfo[]>(DESK_SESSIONS_EVENT, {detail: sessions}));
};

// The folder Desk is showing: the one asked for in the URL, else the folder
// of the session that is open.
export const deskScope = (params: URLSearchParams, sessions: SessionInfo[]): string | undefined => {
    const folder = params.get('folder');
    if (folder) return folder;
    const id = params.get('session');
    return id ? sessions.find((s) => s.id === id)?.project : undefined;
};

import { createContext, useCallback, useContext, type ReactNode } from 'react';
import { setApiFetch, type Thread, type ThreadEvent } from '../client';
import { HostNotifyProvider, type Notify } from '../hooks/useNotify';

/**
 * What the Projects pages need from the app they run in. The standalone app
 * and an embedding host (Tingly Box) each provide one, so the pages never
 * assume where they are mounted or how requests are authenticated.
 */
export interface ProjectsHost {
    /**
     * fetch with the host's authentication. Every Projects request goes
     * through it, including the event stream and library downloads.
     * Defaults to the browser's fetch.
     */
    fetch?: typeof fetch;
    /** Where the Projects pages live in the host's router. */
    paths: {
        list: string;
        project: (id: string) => string;
        /** A working folder, optionally scoped to a path and an open session (the page reads `path`, `session`, `new` and `add` from its query). */
        folder: (path?: string) => string;
        /** The IM settings view. Only with `im`. */
        im?: string;
    };
    /** A project was created, changed or deleted: refresh any navigation that lists projects. */
    onProjectsChanged?: () => void;
    /**
     * Shows a toast in the host's own notification system. Without it the
     * pages use their nearest NotificationProvider (the standalone app's).
     */
    notify?: Notify;
    /**
     * The pages sit inside the host's own layout, which already pads for its
     * navigation and has no Projects sidebar. The pages then drop the room
     * they keep for the standalone app's floating sidebar button and link
     * back to the project list themselves.
     */
    embedded?: boolean;
    /** The host's parts of a folder session; none in the standalone app. */
    session?: SessionSlots;
    /**
     * IM chats (TEP #2143): the bots and chats a host can reach. Without it
     * (the standalone app) Projects shows no IM binding UI at all.
     */
    im?: ImHost;
}

/** An IM bot the host has connected. */
export interface ImBot {
    uuid: string;
    name: string;
    platform: string;
    /** Whether the bot is running and can receive messages. */
    online: boolean;
}

/** A chat a bot has seen: a person's private chat or a group. */
export interface ImChat {
    botUuid: string;
    platform: string;
    /** The platform's chat id; with `platform` it is the binding's key. */
    chatId: string;
    title: string;
    kind: 'direct' | 'group';
    /** Whether the bot may use the chat: `ok`, `blocked` in access control, or a private chat not yet `unpaired`. */
    state: 'ok' | 'blocked' | 'unpaired';
}

/**
 * What an embedding host knows about IM: its bots and the chats they have
 * seen (Tingly Box's Remote data). Bindings themselves are Projects' own.
 */
export interface ImHost {
    listBots: () => Promise<ImBot[]>;
    listChats: (bot: ImBot) => Promise<ImChat[]>;
    /** A platform's display name ("Telegram"); defaults to the id capitalised. */
    platformLabel?: (platform: string) => string;
    /** A platform's logo at `size` px; defaults to a coloured initial. */
    platformIcon?: (platform: string, size: number) => ReactNode;
    /** The host's access-control dialog for a bot (who may use it); the "Access" button shows only with it. */
    accessDialog?: (p: { bot: ImBot; onClose: () => void }) => ReactNode;
}

/** How a folder session launches: a host's profile and the model it asks for ("" is the default). */
export interface SessionLaunch {
    profile: string;
    model: string;
}

/**
 * A host's own parts of a folder session. Projects knows a profile only as a
 * name; a host with profiles (Tingly Box) supplies the pickers and the status
 * line that understand them.
 */
export interface SessionSlots {
    /** Pickers for how a session launches, in the message box strip of a new session and of an open one. */
    workerLaunchPickers?: (p: { profileOverride: string | null; projectProfile: string; model: string; onChange: (profile: string | null, model: string) => void; disabled?: boolean }) => ReactNode;
    launchPickers?: (p: { value: SessionLaunch; onChange: (next: SessionLaunch) => void; disabled?: boolean }) => ReactNode;
    /** Under an open session's message box. */
    statusLine?: (p: { session: Thread; events: ThreadEvent[] }) => ReactNode;
}

const Ctx = createContext<ProjectsHost | null>(null);

export function ProjectsHostProvider({ host, children }: { host: ProjectsHost; children: ReactNode }) {
    // The API client is module-level, so it learns the host's fetch here,
    // before any page below renders and requests anything.
    setApiFetch(host.fetch);
    const content = <Ctx.Provider value={host}>{children}</Ctx.Provider>;
    return host.notify ? <HostNotifyProvider notify={host.notify}>{content}</HostNotifyProvider> : content;
}

/**
 * Fired on window whenever a page reports a project change, so the package's
 * own navigation (ProjectsSidebar) refreshes without the host relaying it.
 */
export const PROJECTS_CHANGED = 'tingly-projects:changed';

export function useProjectsHost(): ProjectsHost & { onProjectsChanged: () => void; embedded: boolean } {
    const host = useContext(Ctx);
    const hostChanged = host?.onProjectsChanged;
    // Stable while the host's callback is: pages list it in effect dependencies.
    const onProjectsChanged = useCallback(() => {
        window.dispatchEvent(new Event(PROJECTS_CHANGED));
        hostChanged?.();
    }, [hostChanged]);
    if (!host) throw new Error('useProjectsHost outside ProjectsHostProvider');
    return { ...host, onProjectsChanged, embedded: host.embedded ?? false };
}

import {createContext, useCallback, useContext, useMemo, useSyncExternalStore} from 'react';
import type {ComponentType, ReactNode, SetStateAction} from 'react';

// DraftStorage is where the transcript keeps what the user typed into a
// request card, and whether it was already sent, so both survive the card
// unmounting (switching sessions) — and, if the host's storage persists, a
// reload. read returns what was last written, as written; decode turns what
// a reload brings back (or nothing, for a new key) into a draft. It must
// return the same value until the key is written again.
export interface DraftStorage {
    read: (key: string, decode: (saved: unknown) => unknown) => unknown;
    write: (key: string, value: unknown) => void;
    subscribe: (key: string, listener: () => void) => () => void;
}

// createMemoryDraftStorage keeps drafts for as long as the page lives.
export const createMemoryDraftStorage = (): DraftStorage => {
    const values = new Map<string, unknown>();
    const listeners = new Map<string, Set<() => void>>();
    return {
        read: (key, decode) => {
            if (!values.has(key)) values.set(key, decode(undefined));
            return values.get(key);
        },
        write: (key, value) => {
            values.set(key, value);
            listeners.get(key)?.forEach((notify) => notify());
        },
        subscribe: (key, listener) => {
            const set = listeners.get(key) ?? new Set();
            set.add(listener);
            listeners.set(key, set);
            return () => {set.delete(listener);};
        },
    };
};

const memoryDrafts = createMemoryDraftStorage();

// CodeBlockComponent renders a fenced code block in a reply. The host passes
// its own (highlighting, copy button); without one, the code is shown plain.
export type CodeBlockComponent = ComponentType<{code: string; language: string}>;

// AttachmentsComponent renders the files a user message carries. The host
// passes its own (thumbnails fetched with its auth); without one, names only.
export type AttachmentsComponent = ComponentType<{items: {name: string; path: string; type: string; size: number}[]}>;

/** Where a subagent's steps stand: not read yet, or read with or without older ones left. */
export type AgentSteps = {loaded: false} | {loaded: true; more: boolean};

// SubagentSteps reads a subagent's own steps when its card is opened: a
// transcript read a page at a time leaves them out (one subagent can run
// hundreds of steps). Without it, the steps are expected to be in the
// transcript already.
export interface SubagentSteps {
    steps: (call: string) => AgentSteps;
    // load reads the latest steps, or with earlier the page before the first loaded one.
    load: (call: string, earlier?: boolean) => Promise<void>;
}

interface TranscriptHost {
    drafts: DraftStorage;
    subagents?: SubagentSteps;
    codeBlock?: CodeBlockComponent;
    attachments?: AttachmentsComponent;
    rememberLabel?: (tool: string) => string;
}

const HostContext = createContext<TranscriptHost>({drafts: memoryDrafts});

export const TranscriptHostProvider = ({drafts, subagents, codeBlock, attachments, rememberLabel, children}: {
    drafts?: DraftStorage;
    subagents?: SubagentSteps;
    codeBlock?: CodeBlockComponent;
    attachments?: AttachmentsComponent;
    rememberLabel?: (tool: string) => string;
    children: ReactNode;
}) => {
    const value = useMemo(() => ({drafts: drafts ?? memoryDrafts, subagents, codeBlock, attachments, rememberLabel}), [drafts, subagents, codeBlock, attachments, rememberLabel]);
    return <HostContext.Provider value={value}>{children}</HostContext.Provider>;
};

export const useTranscriptHost = (): TranscriptHost => useContext(HostContext);

// useDraft reads and writes one draft in the host's storage: the value, a
// setter, and a reader for the latest stored value (so a handler can check
// what was already sent without waiting for a render).
export function useDraft<T>(key: string, decode: (value: unknown) => T) {
    const {drafts} = useTranscriptHost();
    const subscribe = useCallback((listener: () => void) => drafts.subscribe(key, listener), [drafts, key]);
    const read = useCallback(() => drafts.read(key, decode) as T, [decode, drafts, key]);
    const value = useSyncExternalStore(subscribe, read);
    const update = useCallback((change: SetStateAction<T>) => {
        drafts.write(key, typeof change === 'function' ? (change as (value: T) => T)(read()) : change);
    }, [drafts, key, read]);
    return [value, update, read] as const;
}

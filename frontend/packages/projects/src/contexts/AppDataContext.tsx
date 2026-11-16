import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { api, unwrap, type Conversation, type Settings } from '../client';

/**
 * Data the shell (sidebar) shows on every page: recent and starred chats and
 * the connection settings (projects and folders come from the shared
 * project list, useProjectList). Pages call `refresh()` after
 * a change the sidebar should reflect.
 */
interface AppData {
    recents: Conversation[];
    starredChats: Conversation[];
    settings: Settings | null;
    refresh: () => void;
    refreshSettings: () => void;
}

const Ctx = createContext<AppData | null>(null);

export function AppDataProvider({ children }: { children: ReactNode }) {
    const [recents, setRecents] = useState<Conversation[]>([]);
    const [starredChats, setStarredChats] = useState<Conversation[]>([]);
    const [settings, setSettings] = useState<Settings | null>(null);
    const [tick, setTick] = useState(0);
    const [settingsTick, setSettingsTick] = useState(0);

    useEffect(() => {
        let alive = true;
        Promise.all([
            unwrap(api.GET('/api/v1/conversations', { params: { query: { limit: 20 } } })),
            unwrap(api.GET('/api/v1/conversations', { params: { query: { starred: true } } })),
        ]).then(([r, s]) => {
            if (!alive) return;
            setRecents(r.conversations);
            setStarredChats(s.conversations);
        }).catch(() => {
            // The sidebar stays as it was; pages surface their own errors.
        });
        return () => {
            alive = false;
        };
    }, [tick]);

    useEffect(() => {
        unwrap(api.GET('/api/v1/settings')).then(setSettings).catch(() => {});
    }, [settingsTick]);

    const refresh = useCallback(() => setTick((t) => t + 1), []);
    const refreshSettings = useCallback(() => setSettingsTick((t) => t + 1), []);
    const value = useMemo(
        () => ({ recents, starredChats, settings, refresh, refreshSettings }),
        [recents, starredChats, settings, refresh, refreshSettings],
    );
    return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useAppData() {
    const v = useContext(Ctx);
    if (!v) throw new Error('useAppData outside AppDataProvider');
    return v;
}

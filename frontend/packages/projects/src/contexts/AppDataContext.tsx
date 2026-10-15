import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { api, unwrap, type Conversation, type ProjectSummary, type Settings } from '../client';

/**
 * Data the shell (sidebar) shows on every page: projects (with their
 * waiting-on-you counts), recent and starred chats, and the connection
 * settings. Pages call `refresh()` after
 * a change the sidebar should reflect.
 */
interface AppData {
    recents: Conversation[];
    starredChats: Conversation[];
    projects: ProjectSummary[];
    settings: Settings | null;
    refresh: () => void;
    refreshSettings: () => void;
}

const Ctx = createContext<AppData | null>(null);

export function AppDataProvider({ children }: { children: ReactNode }) {
    const [recents, setRecents] = useState<Conversation[]>([]);
    const [starredChats, setStarredChats] = useState<Conversation[]>([]);
    const [projects, setProjects] = useState<ProjectSummary[]>([]);
    const [settings, setSettings] = useState<Settings | null>(null);
    const [tick, setTick] = useState(0);
    const [settingsTick, setSettingsTick] = useState(0);

    useEffect(() => {
        let alive = true;
        Promise.all([
            unwrap(api.GET('/api/v1/conversations', { params: { query: { limit: 20 } } })),
            unwrap(api.GET('/api/v1/conversations', { params: { query: { starred: true } } })),
            unwrap(api.GET('/api/v1/projects')),
        ]).then(([r, s, p]) => {
            if (!alive) return;
            setRecents(r.conversations);
            setStarredChats(s.conversations);
            setProjects(p.projects);
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
        () => ({ recents, starredChats, projects, settings, refresh, refreshSettings }),
        [recents, starredChats, projects, settings, refresh, refreshSettings],
    );
    return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useAppData() {
    const v = useContext(Ctx);
    if (!v) throw new Error('useAppData outside AppDataProvider');
    return v;
}

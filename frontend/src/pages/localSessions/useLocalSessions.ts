import { useCallback, useEffect, useRef, useState } from 'react';
import { localSessionsApi, type SessionListing, type SessionPage, type SessionStatus } from '@/services/localSessionsApi';

const message = (error: unknown) => error instanceof Error ? error.message : 'Unable to load local sessions.';

export function useLocalSessions() {
    const [status, setStatus] = useState<SessionStatus>();
    const [error, setError] = useState('');
    const [projects, setProjects] = useState<SessionListing>();
    const [sessions, setSessions] = useState<SessionListing>();
    const [project, setProject] = useState('');
    const [session, setSession] = useState('');
    const [direction, setDirection] = useState<'forward' | 'backward'>('backward');
    const [previewResult, setPreviewResult] = useState<{ session: string; data: SessionPage }>();
    const [loading, setLoading] = useState({ status: true, projects: false, sessions: false, preview: false });
    const [projectTrail, setProjectTrail] = useState<string[]>([]);
    const [sessionTrail, setSessionTrail] = useState<string[]>([]);
    const [previewTrail, setPreviewTrail] = useState<string[]>([]);
    const requests = useRef<Record<string, AbortController>>({});
    const cursors = useRef({ projects: '', sessions: '', preview: '' });
    const allowed = !!status?.local_binding && !!status?.local_access && !!status?.enabled;

    const begin = useCallback((kind: keyof typeof loading) => {
        requests.current[kind]?.abort();
        const controller = new AbortController();
        requests.current[kind] = controller;
        setLoading(prev => ({ ...prev, [kind]: true }));
        setError('');
        return controller;
    }, []);
    const finish = useCallback((kind: keyof typeof loading, controller: AbortController) => {
        if (!controller.signal.aborted) setLoading(prev => ({ ...prev, [kind]: false }));
    }, []);

    const loadStatus = useCallback(async () => {
        const controller = begin('status');
        try { const data = await localSessionsApi.status(controller.signal); if (!controller.signal.aborted) setStatus(data); }
        catch (e) { if (!controller.signal.aborted) setError(message(e)); }
        finally { finish('status', controller); }
    }, [begin, finish]);
    const loadProjects = useCallback(async (cursor = '') => {
        const controller = begin('projects');
        try {
            const data = await localSessionsApi.projects(cursor, controller.signal);
            if (controller.signal.aborted) return;
            cursors.current.projects = cursor;
            setProjects(data);
            setProject(prev => data.entries.some(p => p.id === prev) ? prev : data.entries[0]?.id || '');
        } catch (e) { if (!controller.signal.aborted) setError(message(e)); }
        finally { finish('projects', controller); }
    }, [begin, finish]);
    const loadSessions = useCallback(async (projectId: string, cursor = '') => {
        const controller = begin('sessions');
        setSession(''); setSessions(undefined);
        try {
            const data = await localSessionsApi.sessions(projectId, cursor, controller.signal);
            if (controller.signal.aborted) return;
            cursors.current.sessions = cursor;
            setSessions(data); setSession(data.entries[0]?.id || '');
        } catch (e) { if (!controller.signal.aborted) setError(message(e)); }
        finally { finish('sessions', controller); }
    }, [begin, finish]);
    const loadPreview = useCallback(async (id: string, cursor = '') => {
        const controller = begin('preview'); setPreviewResult(undefined);
        try {
            const data = await localSessionsApi.preview(id, cursor, direction, controller.signal);
            if (controller.signal.aborted) return;
            cursors.current.preview = cursor; setPreviewResult({ session: id, data });
        } catch (e) { if (!controller.signal.aborted) setError(message(e)); }
        finally { finish('preview', controller); }
    }, [begin, finish, direction]);

    useEffect(() => { void loadStatus(); return () => { Object.values(requests.current).forEach(c => c.abort()); }; }, [loadStatus]);
    useEffect(() => { if (allowed) void loadProjects(); }, [allowed, loadProjects]);
    useEffect(() => {
        requests.current.sessions?.abort(); requests.current.preview?.abort();
        setSession(''); setSessions(undefined); setPreviewResult(undefined); setSessionTrail([]); setPreviewTrail([]);
        setLoading(prev => ({ ...prev, sessions: false, preview: false }));
        if (allowed && project) void loadSessions(project);
        return () => { requests.current.sessions?.abort(); requests.current.preview?.abort(); };
    }, [allowed, project, loadSessions]);
    useEffect(() => {
        requests.current.preview?.abort(); setPreviewResult(undefined); setPreviewTrail([]);
        setLoading(prev => ({ ...prev, preview: false }));
        if (allowed && session) void loadPreview(session);
        return () => { requests.current.preview?.abort(); };
    }, [allowed, session, loadPreview]);

    const preview = previewResult?.session === session ? previewResult.data : undefined;
    const nextProjects = () => { if (projects?.next_cursor) { setProjectTrail(p => [...p.slice(-49), cursors.current.projects]); void loadProjects(projects.next_cursor); } };
    const prevProjects = () => { const previous = projectTrail.at(-1); if (previous !== undefined) { setProjectTrail(p => p.slice(0, -1)); void loadProjects(previous); } };
    const nextSessions = () => { if (sessions?.next_cursor) { setSessionTrail(p => [...p.slice(-49), cursors.current.sessions]); void loadSessions(project, sessions.next_cursor); } };
    const prevSessions = () => { const previous = sessionTrail.at(-1); if (previous !== undefined) { setSessionTrail(p => p.slice(0, -1)); void loadSessions(project, previous); } };
    const nextPreview = () => { if (preview?.next_cursor) { setPreviewTrail(p => [...p.slice(-49), cursors.current.preview]); void loadPreview(session, preview.next_cursor); } };
    const prevPreview = () => { const previous = previewTrail.at(-1); if (previous !== undefined) { setPreviewTrail(p => p.slice(0, -1)); void loadPreview(session, previous); } };
    const jumpTo = (mode: 'forward' | 'backward') => {
        setPreviewTrail([]);
        if (mode === direction && session) void loadPreview(session);
        else setDirection(mode);
    };
    const refreshLists = () => { setProjectTrail([]); setSessionTrail([]); setPreviewTrail([]); void loadProjects(); if (project) void loadSessions(project); };
    const refresh = () => {
        if (!status || !allowed) void loadStatus();
        else if (session) { setPreviewTrail([]); void loadPreview(session); }
        else refreshLists();
    };

    return { status, allowed, error, projects, sessions, project, setProject, session, setSession, direction, jumpTo, preview, loading, refresh, refreshLists,
        nextProjects, prevProjects, nextSessions, prevSessions, nextPreview, prevPreview,
        hasPrevProjects: projectTrail.length > 0, hasPrevSessions: sessionTrail.length > 0, hasPrevPreview: previewTrail.length > 0 };
}

import { useEffect, useRef, useState } from 'react';
import { Typography } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type ProjectSummary, type Thread } from '../client';
import { sessionHref } from './NavTree';
import { sessionGroups, sessionRow } from './workdir/sessionRows';
import { StatusGroups } from './workspace/StatusGroups';

/** The sessions of every folder, read again when a folder's counts change. */
function useAllSessions(folders: ProjectSummary[], revisions: Record<string, number>, reconnect: number): Record<string, Thread[]> {
    const [sessions, setSessions] = useState<Record<string, Thread[]>>({});
    const requested = useRef<Record<string, string>>({});
    const live = useRef(true);
    useEffect(() => {
        live.current = true;
        return () => {
            live.current = false;
        };
    }, []);
    const signatures = folders.map((f) => {
        const c = f.threads;
        return `${f.id}:${c.working}/${c.waiting}/${c.failed}/${c.idle}/${c.ready}/${(revisions[f.id] ?? 0) + reconnect}`;
    });
    const key = signatures.join('|');
    useEffect(() => {
        folders.forEach((f, i) => {
            const sig = signatures[i];
            if (requested.current[f.id] === sig) return;
            requested.current[f.id] = sig;
            unwrap(api.GET('/api/v1/projects/{id}/threads', { params: { path: { id: f.id } } }))
                .then((r) => {
                    if (live.current && requested.current[f.id] === sig) setSessions((s) => ({ ...s, [f.id]: r.threads ?? [] }));
                })
                .catch(() => {
                    // The board still shows the other folders.
                });
        });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [key]);
    return sessions;
}

/**
 * The sidebar's sessions as a kanban board: the folder page's status groups
 * (same groups, labels and rows), drawn as stacked lanes of cards because the
 * sidebar is too narrow for side-by-side columns. Cards come from every
 * working folder and open a session exactly as its row in the folder tree does.
 */
export function NavKanban({ items, activeId, openSession, to, onNavigate, revisions, reconnect }: {
    items: ProjectSummary[];
    /** Id of the folder whose page is open. */
    activeId?: string;
    openSession: string | null;
    to: (item: ProjectSummary) => string;
    onNavigate?: () => void;
    revisions: Record<string, number>;
    reconnect: number;
}) {
    const { t, i18n } = useTranslation('projects');
    const navigate = useNavigate();
    const sessions = useAllSessions(items, revisions, reconnect);
    const cards = items.flatMap((folder) => (sessions[folder.id] ?? []).map((thread) => ({ folder, thread })));
    if (cards.length === 0) {
        return items.every((f) => sessions[f.id]) ? <Typography variant="body2" color="text.secondary" sx={{ px: 3, py: 1 }}>{t('sidebar.noSessions')}</Typography> : null;
    }
    const rows = cards.map(({ folder, thread }) => sessionRow(thread, { withFolder: true, short: true, locale: i18n.language, selected: folder.id === activeId && thread.id === openSession }));
    const open = (id: string) => {
        const card = cards.find((c) => c.thread.id === id);
        if (!card) return;
        navigate(sessionHref(to(card.folder), id));
        onNavigate?.();
    };
    return <StatusGroups outlined groups={sessionGroups((key) => t(`session.group.${key}`))} rows={rows} onOpen={open} />;
}

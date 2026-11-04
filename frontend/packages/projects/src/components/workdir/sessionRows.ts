import type { Thread } from '../../client';
import { folderName, timeAgo } from '../../utils/format';
import { needsUser } from '../project/threadStatus';
import type { StatusGroup, StatusRow } from '../workspace/StatusGroups';

/** Session groups, most urgent first; finished and archived work starts folded. */
export const SESSION_GROUPS: { key: string; collapsed?: boolean }[] = [
    { key: 'waiting' },
    { key: 'failed' },
    { key: 'working' },
    { key: 'idle' },
    { key: 'resolved', collapsed: true },
    { key: 'archived', collapsed: true },
];

export const sessionGroups = (label: (key: string) => string): StatusGroup[] =>
    SESSION_GROUPS.map((g) => ({ key: g.key, label: label(g.key), collapsed: g.collapsed }));

/** The group a session belongs to: ready-for-review sits with idle, archived apart. */
export const sessionGroup = (s: Thread) => (s.archived_at ? 'archived' : s.status === 'ready' ? 'idle' : s.status);

/** A session as a row of the status groups; `withFolder` names its working folder. */
export function sessionRow(s: Thread, opts: { withFolder: boolean; locale: string; selected: boolean; /** Narrow time ("2m ago"), for the sidebar. */ short?: boolean }): StatusRow {
    return {
        id: s.id,
        group: sessionGroup(s),
        title: s.title,
        subtitle: [opts.withFolder ? folderName(s.cwd) : '', !s.archived_at && needsUser(s.status) ? s.wait_reason : ''].filter(Boolean).join(' · ') || undefined,
        meta: timeAgo(s.updated_at, opts.locale, opts.short),
        selected: opts.selected,
    };
}

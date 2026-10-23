import { Fragment, useEffect, useState, type ReactNode } from 'react';
import { Badge, Box, Divider, IconButton, List, ListItemButton, ListItemIcon, ListItemText, Tooltip, Typography } from '@mui/material';
import { NAV_ROW_SX, navRowSelectedSx, navRowTextSlotProps } from '@tingly/ui/theme/nav';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { NavLink, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useProjectList } from '../hooks/useProjectList';
import { api, unwrap, type ProjectSummary, type Thread } from '../client';
import { folderName, timeAgo } from '../utils/format';
import { Add, ChatBubble, Folder, FolderOpen, Star } from './icons';
import { useProjectsHost } from '../host/ProjectsHost';
import { needsUser } from './project/threadStatus';

/** One row of a sidebar section: a place the user can open. */
export interface ProjectsSidebarRow {
    key: string;
    label: string;
    /** Hover text, e.g. a folder's full path. */
    title?: string;
    icon?: ReactNode;
    /** The second line: what waits here, where it is. */
    subtitle?: ReactNode;
    /** Router path the row opens. */
    to: string;
    selected?: boolean;
    /** Something here waits on the user: a dot on the icon. */
    attention?: boolean;
    /** Work runs here and nothing waits: a quieter dot. */
    running?: boolean;
    /** Rows nested under this one (a folder's recent sessions). */
    children?: ProjectsSidebarRow[];
    /** Trailing content: a count, a status mark. */
    end?: ReactNode;
}

/** Running is neutral: only what needs the user is coloured (amber). */
const RUNNING_DOT = { '& .MuiBadge-badge': { backgroundColor: 'text.secondary' } } as const;

/** A titled group of rows. Hosts pass their own (e.g. Tingly Box's working folders). */
export interface ProjectsSidebarSection {
    key: string;
    title: string;
    /** Where the title links to: the section's overview page. */
    to?: string;
    /** A "+" next to the title. */
    action?: { label: string; onClick: () => void };
    rows: ProjectsSidebarRow[];
    /** Shown when there are no rows. */
    empty?: ReactNode;
}

export interface ProjectsSidebarProps {
    /** List the user's projects (default true). Off when the host shows only its own sections. */
    showProjects?: boolean;
    /** List the working folders (default false): the saved ones and any folder with a session, with what waits there. */
    showWorkdirs?: boolean;
    /** Host sections, rendered after Projects. */
    sections?: ProjectsSidebarSection[];
    /** Show the IM entry at the bottom when the host has IM (default true). */
    showIm?: boolean;
    /** Called after a row or link is followed (closes a host's mobile drawer). */
    onNavigate?: () => void;
}

// The folder a working directory sits in, home shortened to ~.
const parentPath = (path: string) => path.replace(/[\\/]+$/, '').replace(/[\\/][^\\/]*$/, '').replace(/^\/(Users|home)\/[^/]+/, '~') || '/';

/** How many IM bots the host has, for the sidebar's IM row; null until known or when there is no IM. */
function useImBotCount(enabled: boolean): number | null {
    const { im } = useProjectsHost();
    const [count, setCount] = useState<number | null>(null);
    useEffect(() => {
        if (!enabled || !im) return;
        let alive = true;
        im.listBots().then((b) => alive && setCount(b.length)).catch(() => {
            // The row still opens the IM view; it just has no count.
        });
        return () => {
            alive = false;
        };
    }, [enabled, im]);
    return count;
}

/** What a project or folder row says about its threads: running and waiting, else what waits. */
function useActivity() {
    const { t } = useTranslation('projects');
    return (p: ProjectSummary, waitingKey: 'sidebar.waiting' | 'sidebar.sessionsWaiting') => {
        const { working } = p.threads;
        const waiting = p.threads.waiting + p.threads.failed;
        if (working > 0 && waiting > 0) return t('sidebar.activity', { running: working, waiting });
        if (working > 0) return t('sidebar.running', { count: working });
        return waiting > 0 ? t(waitingKey, { count: waiting }) : undefined;
    };
}

const MAX_RECENT = 3;

/**
 * The open folder's latest live sessions, listed under it: what runs and what
 * waits first. One request, for the open folder only, read again when its
 * counts change.
 */
function useRecentSessions(folder: ProjectSummary | undefined): Thread[] {
    const [threads, setThreads] = useState<Thread[]>([]);
    const id = folder?.id;
    const c = folder?.threads;
    const signature = c ? `${c.working}/${c.waiting}/${c.failed}/${c.idle}/${c.ready}` : '';
    useEffect(() => {
        if (!id) {
            setThreads([]);
            return;
        }
        let alive = true;
        unwrap(api.GET('/api/v1/projects/{id}/threads', { params: { path: { id } } }))
            .then((r) => alive && setThreads(r.threads ?? []))
            .catch(() => {
                // The folder row still works without its sessions.
            });
        return () => {
            alive = false;
        };
    }, [id, signature]);
    return threads;
}

function SectionView({ section, onNavigate }: { section: ProjectsSidebarSection; onNavigate?: () => void }) {
    const title = (
        <Typography noWrap sx={{ flex: 1, fontSize: fontSizes.sm, fontWeight: 600, color: 'text.secondary' }}>
            {section.title}
        </Typography>
    );
    return (
        <Box component="section" aria-label={section.title || undefined}>
            {section.title && <Box sx={{ display: 'flex', alignItems: 'center', pl: 3, pr: 1.5, minHeight: 32 }}>
                {section.to ? (
                    <Box component={NavLink} to={section.to} onClick={onNavigate} sx={{ flex: 1, minWidth: 0, display: 'flex', textDecoration: 'none', '&:hover .MuiTypography-root': { color: 'text.primary' } }}>
                        {title}
                    </Box>
                ) : title}
                {section.action && (
                    <Tooltip title={section.action.label}>
                        <IconButton size="small" aria-label={section.action.label} onClick={section.action.onClick} sx={{ color: 'text.secondary', '&:hover': { color: 'primary.main' } }}>
                            <Add sx={{ fontSize: 18 }} />
                        </IconButton>
                    </Tooltip>
                )}
            </Box>}
            <List disablePadding>
                {section.rows.length === 0 && section.empty && (
                    <Typography variant="body2" color="text.secondary" sx={{ px: 3, py: 1 }}>{section.empty}</Typography>
                )}
                {section.rows.map((row) => (
                    <Fragment key={row.key}>
                    <ListItemButton
                        component={NavLink}
                        to={row.to}
                        onClick={onNavigate}
                        title={row.title}
                        aria-current={row.selected ? 'page' : undefined}
                        sx={{
                            ...NAV_ROW_SX,
                            color: 'text.secondary',
                            '&:hover': { backgroundColor: 'action.hover', color: 'text.primary' },
                            ...(row.selected && navRowSelectedSx),
                            '& .row-end, & .row-end *': row.selected ? { color: 'inherit' } : {},
                        }}
                    >
                        <ListItemIcon sx={{ minWidth: 32, color: 'inherit', '& svg': { fontSize: 20 } }}>
                            <Badge color="warning" variant="dot" invisible={!row.attention && !row.running} sx={row.attention ? undefined : RUNNING_DOT}>
                                {row.icon ?? <Folder />}
                            </Badge>
                        </ListItemIcon>
                        <ListItemText primary={row.label} secondary={row.subtitle} slotProps={navRowTextSlotProps(!!row.selected)} sx={{ minWidth: 0, my: 0 }} />
                        {row.end != null && <Box className="row-end" sx={{ ml: 1, flexShrink: 0, display: 'flex', alignItems: 'center' }}>{row.end}</Box>}
                    </ListItemButton>
                    {row.children?.map((c) => (
                        <ListItemButton
                            key={c.key}
                            component={NavLink}
                            to={c.to}
                            onClick={onNavigate}
                            title={c.title}
                            aria-current={c.selected ? 'page' : undefined}
                            sx={{
                                ...NAV_ROW_SX,
                                ml: 4.5, pl: 1.25, py: 0.25, minHeight: 28,
                                color: 'text.secondary',
                                '&:hover': { backgroundColor: 'action.hover', color: 'text.primary' },
                                ...(c.selected && { backgroundColor: 'action.selected', color: 'text.primary' }),
                            }}
                        >
                            <Badge color="warning" variant="dot" invisible={!c.attention && !c.running} sx={[{ mr: 1.25, '& .MuiBadge-badge': { position: 'static', transform: 'none' } }, c.attention ? {} : RUNNING_DOT]} />
                            <Typography noWrap sx={{ flex: 1, minWidth: 0, fontSize: fontSizes.sm, color: 'inherit' }}>{c.label}</Typography>
                            {c.end != null && <Box sx={{ ml: 1, flexShrink: 0, fontSize: fontSizes.xs, color: 'text.disabled' }}>{c.end}</Box>}
                        </ListItemButton>
                    ))}
                    </Fragment>
                ))}
            </List>
        </Box>
    );
}

/**
 * The Projects navigation: the user's projects (with how many threads wait on
 * them), followed by whatever sections the host adds. It knows nothing about
 * the host; Tingly Box passes its working folders in as a section.
 */
export function ProjectsSidebar({ showProjects = true, showWorkdirs = false, showIm = true, sections = [], onNavigate }: ProjectsSidebarProps) {
    const { t, i18n } = useTranslation('projects');
    const navigate = useNavigate();
    const location = useLocation();
    const { paths, im } = useProjectsHost();
    const botCount = useImBotCount(showIm && !!im);
    // One list serves both groups: Projects and Folders are its two types.
    const { projects, folders } = useProjectList(showProjects || showWorkdirs);
    const [params] = useSearchParams();
    const scope = location.pathname === paths.folder().split('?')[0] ? params.get('path') ?? '' : undefined;
    const activity = useActivity();
    const openFolder = scope ? (folders ?? []).find((f) => f.path === scope) : undefined;
    const recent = useRecentSessions(openFolder);
    const openSession = params.get('session');

    const projectSection: ProjectsSidebarSection | null = showProjects ? {
        key: 'projects',
        title: t('nav.projects'),
        to: paths.list,
        action: {
            label: t('projects.new'),
            onClick: () => {
                navigate(`${paths.list}?new=1`);
                onNavigate?.();
            },
        },
        rows: (projects ?? []).map((p) => {
            const to = paths.project(p.id);
            return {
                key: p.id,
                label: p.name,
                to,
                selected: location.pathname === to,
                attention: p.threads.waiting + p.threads.failed > 0,
                running: p.threads.working > 0,
                icon: p.starred ? <Star /> : undefined,
                subtitle: activity(p, 'sidebar.waiting'),
            };
        }),
        empty: projects ? t('sidebar.noProjects') : undefined,
    } : null;

    // The open folder's sessions that run or wait come first, then the latest; archived ones stay out.
    const rank = (th: Thread) => (needsUser(th.status) ? 0 : th.status === 'working' ? 1 : 2);
    const recentRows = (path: string): ProjectsSidebarRow[] => recent
        .filter((th) => th.status !== 'resolved')
        .sort((a, b) => rank(a) - rank(b) || b.updated_at.localeCompare(a.updated_at))
        .slice(0, MAX_RECENT)
        .map((th) => ({
            key: th.id,
            label: th.title,
            title: th.title,
            to: `${paths.folder(path)}${paths.folder(path).includes('?') ? '&' : '?'}session=${encodeURIComponent(th.id)}`,
            selected: th.id === openSession,
            attention: needsUser(th.status),
            running: th.status === 'working',
            end: th.status === 'working' || needsUser(th.status) ? undefined : timeAgo(th.updated_at, i18n.language),
        }));

    const workdirSection: ProjectsSidebarSection | null = showWorkdirs ? {
        key: 'workdirs',
        title: t('workdir.title'),
        to: paths.folder(),
        action: {
            label: t('workdir.add'),
            onClick: () => {
                navigate(`${paths.folder()}${paths.folder().includes('?') ? '&' : '?'}add=1`);
                onNavigate?.();
            },
        },
        rows: (folders ?? []).map((f) => {
            const path = f.path ?? '';
            return {
                key: f.id,
                label: folderName(path),
                title: path,
                icon: <FolderOpen />,
                // What waits here, else where the folder is.
                subtitle: activity(f, 'sidebar.sessionsWaiting') ?? parentPath(path),
                to: paths.folder(path),
                selected: scope === path && !openSession,
                attention: f.threads.waiting + f.threads.failed > 0,
                running: f.threads.working > 0,
                children: f.id === openFolder?.id ? recentRows(path) : undefined,
            };
        }),
        empty: folders ? t('sidebar.noWorkdirs') : undefined,
    } : null;

    // IM is configuration, not content: one row pinned at the bottom (TEP #2143).
    const imSection: ProjectsSidebarSection | null = showIm && im && paths.im ? {
        key: 'im',
        title: '',
        rows: [{
            key: 'im',
            label: t('im.title'),
            icon: <ChatBubble />,
            subtitle: botCount == null ? undefined : t('im.bots', { count: botCount }),
            to: paths.im,
            selected: location.pathname === paths.im,
        }],
    } : null;

    return (
        <Box component="nav" aria-label={t('nav.projects')} sx={{ py: 1, minHeight: '100%', boxSizing: 'border-box', display: 'flex', flexDirection: 'column' }}>
            <Box sx={{ flex: 1 }}>
                {[...(projectSection ? [projectSection] : []), ...(workdirSection ? [workdirSection] : []), ...sections].map((s, i) => (
                    <Box key={s.key}>
                        {i > 0 && <Divider sx={{ mx: 2, my: 1 }} />}
                        <SectionView section={s} onNavigate={onNavigate} />
                    </Box>
                ))}
            </Box>
            {imSection && (
                <Box sx={{ position: 'sticky', bottom: 0, bgcolor: 'background.paper', mt: 1 }}>
                    <Divider sx={{ mx: 2, mb: 1 }} />
                    <SectionView section={imSection} onNavigate={onNavigate} />
                </Box>
            )}
        </Box>
    );
}

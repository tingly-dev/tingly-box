import { useEffect, useState, type ReactNode } from 'react';
import { Badge, Box, Divider, IconButton, List, ListItemButton, ListItemIcon, ListItemText, ToggleButton, ToggleButtonGroup, Tooltip, Typography } from '@mui/material';
import { NAV_ROW_SX, navRowSelectedSx, navRowTextSlotProps } from '@tingly/ui/theme/nav';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { NavLink, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import type { ProjectSummary } from '../client';
import { useProjectList } from '../hooks/useProjectList';
import { Add, ChatBubble, Folder, Kanban, Stack, Star, ViewList } from './icons';
import { useProjectsHost } from '../host/ProjectsHost';
import { NavKanban } from './NavKanban';
import { NavTree } from './NavTree';
import { readLocal, writeLocal } from '../utils/storage';

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
    /** Controls between the title and the "+" (a view switch). */
    extra?: ReactNode;
    /** A "+" next to the title. */
    action?: { label: string; onClick: () => void };
    rows: ProjectsSidebarRow[];
    /** Shown when there are no rows. */
    empty?: ReactNode;
    /** Replaces the generic rows (the working folders draw a tree). */
    content?: ReactNode;
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
    /** Host-specific destinations displayed below the IM chats entry. */
    imRows?: ProjectsSidebarRow[];
    /** Called after a row or link is followed (closes a host's mobile drawer). */
    onNavigate?: () => void;
}

export type SessionsView = 'list' | 'kanban';
const VIEW_KEY = 'nav.sessionsView';

/** How the sidebar lays out sessions, remembered per browser; the list until chosen. */
function useSessionsView(): [SessionsView, (v: SessionsView) => void] {
    const [view, setView] = useState<SessionsView>(() => (readLocal(VIEW_KEY) === 'kanban' ? 'kanban' : 'list'));
    return [view, (v) => {
        writeLocal(VIEW_KEY, v);
        setView(v);
    }];
}

function ViewSwitch({ view, onChange }: { view: SessionsView; onChange: (v: SessionsView) => void }) {
    const { t } = useTranslation('projects');
    const button = (value: SessionsView, label: string, icon: ReactNode) => (
        <Tooltip title={label}>
            <ToggleButton value={value} aria-label={label} sx={{ p: 0.375, border: 0, borderRadius: 1, color: 'text.disabled', '& svg': { fontSize: 16 }, '&.Mui-selected': { color: 'text.primary', bgcolor: 'action.selected' } }}>
                {icon}
            </ToggleButton>
        </Tooltip>
    );
    return (
        <ToggleButtonGroup exclusive size="small" value={view} aria-label={t('sidebar.view.label')} onChange={(_, v: SessionsView | null) => v && onChange(v)} sx={{ gap: 0.25, mr: 0.5 }}>
            {button('list', t('sidebar.view.list'), <ViewList />)}
            {button('kanban', t('sidebar.view.kanban'), <Kanban />)}
        </ToggleButtonGroup>
    );
}

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
                {section.extra}
                {section.action && (
                    <Tooltip title={section.action.label}>
                        <IconButton size="small" aria-label={section.action.label} onClick={section.action.onClick} sx={{ color: 'text.secondary', '&:hover': { color: 'primary.main' } }}>
                            <Add sx={{ fontSize: 18 }} />
                        </IconButton>
                    </Tooltip>
                )}
            </Box>}
            <List disablePadding>
                {section.content}
                {section.rows.length === 0 && !section.content && section.empty && (
                    <Typography variant="body2" color="text.secondary" sx={{ px: 3, py: 1 }}>{section.empty}</Typography>
                )}
                {section.rows.map((row) => (
                    <Box key={row.key}>
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
                    </Box>
                ))}
            </List>
        </Box>
    );
}

/**
 * The Projects navigation: the user's projects and working folders, one flat
 * row each. Folders can expand to show their sessions, followed by
 * whatever sections the host adds. It knows nothing about the host; Tingly Box
 * passes its working folders in as a section.
 */
export function ProjectsSidebar({ showProjects = true, showWorkdirs = false, showIm = true, imRows = [], sections = [], onNavigate }: ProjectsSidebarProps) {
    const { t } = useTranslation('projects');
    const navigate = useNavigate();
    const location = useLocation();
    const { paths, im } = useProjectsHost();
    const botCount = useImBotCount(showIm && !!im);
    // One list serves both groups: Projects and Folders are its two types.
    const [revisions, setRevisions] = useState<Record<string, number>>({});
    const [reconnect, setReconnect] = useState(0);
    const { projects, folders } = useProjectList(showProjects || showWorkdirs, (ev) => {
        if (ev.type === 'thread') setRevisions((r) => ({ ...r, [ev.project_id]: (r[ev.project_id] ?? 0) + 1 }));
    }, () => setReconnect((n) => n + 1));
    const [sessionsView, setSessionsView] = useSessionsView();
    const [params] = useSearchParams();
    const scope = location.pathname === paths.folder().split('?')[0] ? params.get('path') ?? '' : undefined;

    // What waits on the user, or works, dots the icon; waiting wins the colour.
    const state = (p: { threads: { waiting: number; failed: number; working: number } }) => ({
        attention: p.threads.waiting + p.threads.failed > 0,
        running: p.threads.working > 0,
    });

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
        rows: (projects ?? []).map((p) => ({
            key: p.id,
            label: p.name,
            title: p.name,
            icon: p.starred ? <Star /> : <Stack />,
            to: paths.project(p.id),
            selected: location.pathname === paths.project(p.id),
            ...state(p),
        })),
        empty: projects ? t('sidebar.noProjects') : undefined,
    } : null;

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
        extra: folders?.length ? <ViewSwitch view={sessionsView} onChange={setSessionsView} /> : undefined,
        rows: [],
        content: folders?.length ? (() => {
            const tree = {
                items: folders, activeId: folders.find((f) => f.path === scope)?.id,
                openSession: scope === undefined ? null : params.get('session'), to: (f: ProjectSummary) => paths.folder(f.path ?? ''),
                onNavigate, revisions, reconnect,
            };
            return sessionsView === 'kanban' ? <NavKanban {...tree} /> : <NavTree {...tree} />;
        })() : undefined,
        empty: folders ? t('sidebar.noWorkdirs') : undefined,
    } : null;

    // IM is configuration, not content: one row pinned at the bottom (TEP #2143).
    const imSection: ProjectsSidebarSection | null = showIm && im && paths.im ? {
        key: 'im',
        title: '',
        rows: [...imRows, {
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

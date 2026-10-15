import { useEffect, useState, type ReactNode } from 'react';
import { Badge, Box, Divider, IconButton, List, ListItemButton, ListItemIcon, ListItemText, Tooltip, Typography } from '@mui/material';
import { NAV_ROW_SX, navRowSelectedSx, navRowTextSlotProps } from '@tingly/ui/theme/nav';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { NavLink, useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type ProjectSummary } from '../client';
import { Add, Folder, Star } from './icons';
import { PROJECTS_CHANGED, useProjectsHost } from '../host/ProjectsHost';

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
    /** Trailing content: a count, a status mark. */
    end?: ReactNode;
}

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
    /** Host sections, rendered after Projects. */
    sections?: ProjectsSidebarSection[];
    /** Called after a row or link is followed (closes a host's mobile drawer). */
    onNavigate?: () => void;
}

const POLL_MS = 15_000;

/** The active projects, starred first, refreshed when a page changes one and on a slow poll. */
function useSidebarProjects(enabled: boolean): ProjectSummary[] | null {
    const [projects, setProjects] = useState<ProjectSummary[] | null>(null);
    useEffect(() => {
        if (!enabled) return;
        // Only the latest request may land, and none after unmount.
        let latest = 0;
        const load = () => {
            if (document.visibilityState === 'hidden') return;
            const request = ++latest;
            unwrap(api.GET('/api/v1/projects'))
                .then((r) => {
                    if (request === latest) setProjects([...r.projects].sort((a, b) => Number(b.starred) - Number(a.starred)));
                })
                .catch(() => {
                    // Unreachable: keep the last list.
                });
        };
        load();
        const timer = window.setInterval(load, POLL_MS);
        window.addEventListener(PROJECTS_CHANGED, load);
        document.addEventListener('visibilitychange', load);
        return () => {
            latest = -1;
            window.clearInterval(timer);
            window.removeEventListener(PROJECTS_CHANGED, load);
            document.removeEventListener('visibilitychange', load);
        };
    }, [enabled]);
    return projects;
}

function SectionView({ section, onNavigate }: { section: ProjectsSidebarSection; onNavigate?: () => void }) {
    const title = (
        <Typography noWrap sx={{ flex: 1, fontSize: fontSizes.sm, fontWeight: 600, color: 'text.secondary' }}>
            {section.title}
        </Typography>
    );
    return (
        <Box component="section" aria-label={section.title}>
            <Box sx={{ display: 'flex', alignItems: 'center', pl: 3, pr: 1.5, minHeight: 32 }}>
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
            </Box>
            <List disablePadding>
                {section.rows.length === 0 && section.empty && (
                    <Typography variant="body2" color="text.secondary" sx={{ px: 3, py: 1 }}>{section.empty}</Typography>
                )}
                {section.rows.map((row) => (
                    <ListItemButton
                        key={row.key}
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
                            <Badge color="warning" variant="dot" invisible={!row.attention}>
                                {row.icon ?? <Folder />}
                            </Badge>
                        </ListItemIcon>
                        <ListItemText primary={row.label} secondary={row.subtitle} slotProps={navRowTextSlotProps(!!row.selected)} sx={{ minWidth: 0, my: 0 }} />
                        {row.end != null && <Box className="row-end" sx={{ ml: 1, flexShrink: 0, display: 'flex', alignItems: 'center' }}>{row.end}</Box>}
                    </ListItemButton>
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
export function ProjectsSidebar({ showProjects = true, sections = [], onNavigate }: ProjectsSidebarProps) {
    const { t } = useTranslation('projects');
    const navigate = useNavigate();
    const location = useLocation();
    const { paths } = useProjectsHost();
    const projects = useSidebarProjects(showProjects);

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
                attention: p.threads.waiting > 0,
                icon: p.starred ? <Star /> : undefined,
                subtitle: p.threads.waiting > 0 ? t('sidebar.waiting', { count: p.threads.waiting }) : undefined,
            };
        }),
        empty: projects ? t('sidebar.noProjects') : undefined,
    } : null;

    return (
        <Box component="nav" aria-label={t('nav.projects')} sx={{ py: 1 }}>
            {[...(projectSection ? [projectSection] : []), ...sections].map((s, i) => (
                <Box key={s.key}>
                    {i > 0 && <Divider sx={{ mx: 2, my: 1 }} />}
                    <SectionView section={s} onNavigate={onNavigate} />
                </Box>
            ))}
        </Box>
    );
}

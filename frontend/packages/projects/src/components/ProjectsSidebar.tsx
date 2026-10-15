import { useEffect, useState, type ReactNode } from 'react';
import { Badge, Box, IconButton, List, ListItemButton, ListItemIcon, ListItemText, Tooltip, Typography } from '@mui/material';
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
        <Typography variant="caption" noWrap sx={{ flex: 1, fontWeight: 600, color: 'text.secondary', letterSpacing: 0.2 }}>
            {section.title}
        </Typography>
    );
    return (
        <Box component="section" aria-label={section.title} sx={{ mt: 1.5, '&:first-of-type': { mt: 0.5 } }}>
            <Box sx={{ display: 'flex', alignItems: 'center', pl: 2.25, pr: 1.25, minHeight: 28 }}>
                {section.to ? (
                    <Box component={NavLink} to={section.to} onClick={onNavigate} sx={{ flex: 1, minWidth: 0, display: 'flex', textDecoration: 'none', '&:hover .MuiTypography-root': { color: 'text.primary' } }}>
                        {title}
                    </Box>
                ) : title}
                {section.action && (
                    <Tooltip title={section.action.label}>
                        <IconButton size="small" aria-label={section.action.label} onClick={section.action.onClick} sx={{ p: 0.25, color: 'text.secondary', '&:hover': { color: 'primary.main' } }}>
                            <Add sx={{ fontSize: 16 }} />
                        </IconButton>
                    </Tooltip>
                )}
            </Box>
            <List dense disablePadding>
                {section.rows.length === 0 && section.empty && (
                    <Typography variant="body2" color="text.secondary" sx={{ px: 2.25, py: 0.75, fontSize: 13 }}>{section.empty}</Typography>
                )}
                {section.rows.map((row) => (
                    <ListItemButton
                        key={row.key}
                        component={NavLink}
                        to={row.to}
                        onClick={onNavigate}
                        title={row.title}
                        selected={row.selected}
                        aria-current={row.selected ? 'page' : undefined}
                        sx={{
                            mx: 1, px: 1.25, py: 0.5, minHeight: 36, borderRadius: 1.25, color: 'text.primary',
                            '&.Mui-selected': { bgcolor: 'primary.main', color: 'primary.contrastText', '&:hover': { bgcolor: 'primary.main' } },
                            '&.Mui-selected .row-end, &.Mui-selected .row-end *': { color: 'inherit' },
                        }}
                    >
                        <ListItemIcon sx={{ minWidth: 28, color: 'inherit' }}>
                            <Badge color="warning" variant="dot" invisible={!row.attention}>
                                {row.icon ?? <Folder sx={{ fontSize: 17 }} />}
                            </Badge>
                        </ListItemIcon>
                        <ListItemText primary={row.label} slotProps={{ primary: { noWrap: true, sx: { fontSize: 13.5, color: 'inherit', fontWeight: row.selected ? 500 : 400 } } }} />
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
                icon: p.starred ? <Star sx={{ fontSize: 17 }} /> : undefined,
                end: p.threads.waiting > 0 ? (
                    <Tooltip title={t('sidebar.waiting', { count: p.threads.waiting })}>
                        <Typography component="span" variant="caption" sx={{ fontWeight: 600, color: 'warning.main', fontVariantNumeric: 'tabular-nums' }}>
                            {p.threads.waiting}
                        </Typography>
                    </Tooltip>
                ) : undefined,
            };
        }),
        empty: projects ? t('sidebar.noProjects') : undefined,
    } : null;

    return (
        <Box component="nav" aria-label={t('nav.projects')} sx={{ py: 0.5 }}>
            {[...(projectSection ? [projectSection] : []), ...sections].map((s) => (
                <SectionView key={s.key} section={s} onNavigate={onNavigate} />
            ))}
        </Box>
    );
}

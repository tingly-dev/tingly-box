import type { ReactNode } from 'react';
import type { ActivityItem } from '@/layout/types';

export interface SubPage {
    path: string;
    label: string;
    subtitle?: string;
    icon?: ReactNode;
    active: boolean;
}

/**
 * The pages the desktop sidebar lists for an activity, as groups (its
 * dividers split them). Hidden agents are left out, as on desktop outside edit
 * mode; "add" actions (`#add-profile`) are not pages. An activity whose
 * sidebar lists things rather than pages (Workspace) has none: it has its own screens.
 */
export function subPageGroups(item: ActivityItem | undefined, pathname: string): SubPage[][] {
    if (!item || item.sidebar) return [];
    const groups: SubPage[][] = [[]];
    for (const child of item.children ?? []) {
        if (child.type === 'divider') {
            groups.push([]);
            continue;
        }
        if (child.hidden || child.path.startsWith('#')) continue;
        groups[groups.length - 1].push({
            path: child.path,
            label: child.label,
            subtitle: child.subtitle,
            icon: child.icon,
            active: child.match ? child.match(pathname) : child.path === pathname,
        });
    }
    return groups.filter((g) => g.length > 0);
}

/** Switching only makes sense between two or more pages. */
export const hasSubPages = (groups: SubPage[][]) => groups.reduce((n, g) => n + g.length, 0) > 1;

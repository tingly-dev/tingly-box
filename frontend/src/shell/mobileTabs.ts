import type { ReactNode } from 'react';
import type { ActivityItem } from '@/layout/types';
import { resolveActiveActivity } from '@/layout/activeActivity';

export type MobileTabKey = 'projects' | 'scenario' | 'more';

export interface MobileTab {
    key: MobileTabKey;
    label: string;
    icon: ReactNode;
    to: string;
    /** Something here waits on the user: a dot on the tab. */
    attention: boolean;
}

/** The phone's tabs, from the same activity list the desktop rail is built from: Workspace (when enabled), Agent, More. */
export function buildTabs(items: ActivityItem[], more: { label: string; icon: ReactNode }, projectsEnabled: boolean): MobileTab[] {
    const tab = (key: 'projects' | 'scenario'): MobileTab[] => {
        const item = items.find((i) => i.key === key);
        if (!item) return [];
        return [{ key, label: item.label, icon: item.icon, to: item.defaultPath ?? item.path ?? '/agent', attention: !!item.attention }];
    };
    return [
        ...(projectsEnabled ? tab('projects') : []),
        ...tab('scenario'),
        { key: 'more', label: more.label, icon: more.icon, to: '/more', attention: false },
    ];
}

/** The tab a path lights up: Workspace and Agent own their pages, everything else is under More. */
export function activeTab(tabs: MobileTab[], items: ActivityItem[], pathname: string, saved: string | null): MobileTabKey {
    if (pathname === '/more') return 'more';
    const activity = resolveActiveActivity(items, pathname, saved);
    return tabs.some((t) => t.key === activity) ? (activity as MobileTabKey) : 'more';
}

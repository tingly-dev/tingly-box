import type { ReactNode } from 'react';

export interface LayoutProps {
    children?: ReactNode;
}

export interface NavItemBase {
    type?: undefined;
    path: string;
    label: string;
    icon?: ReactNode;
    subtitle?: string;
    /** Optional descriptive copy shown as a delayed hover tooltip on the sidebar item. */
    tooltip?: string;
    /**
     * Optional override for "is this item active": receives the current
     * pathname, returns whether this item should highlight. Defaults to an
     * exact match on `path`. Use this for an item whose route has a dynamic
     * segment (e.g. `/remote-agent/:platform`, navigated via in-page tabs
     * rather than one sidebar row per value) so the row stays highlighted —
     * and the parent activity stays selected — across the whole sub-tree.
     */
    match?: (pathname: string) => boolean;
}

export interface NavDivider {
    type: 'divider';
}

export type NavItem = NavItemBase | NavDivider;

export interface ActivityItem {
    key: string;
    icon: ReactNode;
    label: string;
    path?: string;
    children?: NavItem[];
    // Default path to use when there's no saved path in memory
    defaultPath?: string;
}

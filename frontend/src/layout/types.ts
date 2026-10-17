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
    /**
     * Optional short status count rendered right-aligned on the label's own
     * line (e.g. "2/3"), with `tooltip`-style hover text spelling it out.
     * Use this instead of `subtitle` for a live number, so the row stays
     * one line like its siblings; `subtitle` is for identity text (a
     * profile's id) that belongs under the name.
     */
    count?: { value: string; title: string };
    /** Optional descriptive copy shown as a delayed hover tooltip on the sidebar item. */
    tooltip?: string;
    /**
     * Like `tooltip` but without the info marker: a description for rows
     * whose name already says what they are (an agent's one-line summary).
     */
    hint?: string;
    /**
     * Hidden-scenario id this row can be hidden by. The sidebar's edit mode
     * shows a visibility toggle for it; outside edit mode a `hidden` row
     * isn't rendered at all.
     */
    hideId?: string;
    hidden?: boolean;
    /** Set on rows that can be pinned to the top of the list (hover shows a pin). */
    pinId?: string;
    pinned?: boolean;
    /**
     * Optional override for "is this item active": receives the current
     * pathname, returns whether this item should highlight. Defaults to an
     * exact match on `path`. Use this for an item whose route has a dynamic
     * segment or sub-pages (e.g. `/projects/...`) so the row stays highlighted —
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
    /** Something here needs the user: a dot on the rail icon, with this as its hover text. */
    attention?: string;
    /**
     * Renders the sidebar body in place of the `children` rows, for an
     * activity whose sidebar lists things rather than pages (Projects lists
     * projects and working folders). `children` still decide which routes
     * light this activity up. `onNavigate` closes a drawer or flyout.
     */
    sidebar?: (onNavigate: () => void) => ReactNode;
}

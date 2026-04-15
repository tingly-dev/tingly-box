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
}

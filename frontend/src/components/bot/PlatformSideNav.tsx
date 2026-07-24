import { List, ListItemButton, ListItemIcon, ListItemText } from '@mui/material';
import type { ReactNode } from 'react';
import { NAV_ROW_SX, navRowTextSlotProps } from '@/layout/navRowStyles';

export interface PlatformSideNavItem {
    id: string;
    label: string;
    icon: ReactNode;
    subtitle?: string;
}

interface PlatformSideNavProps {
    items: PlatformSideNavItem[];
    value: string;
    onChange: (id: string) => void;
}

// PlatformSideNav is the platform picker shared by Overview and Remote — a
// vertical list of platform rows (icon, name, active/total subtitle), same
// visual language and row metrics as the global Sidebar. Meant to be used
// inside SecondaryNavLayout, which supplies the white background, border,
// and width — this component is just the list itself.
const PlatformSideNav: React.FC<PlatformSideNavProps> = ({ items, value, onChange }) => (
    <List sx={{ py: 0 }}>
        {items.map((item) => {
            const active = item.id === value;
            return (
                <ListItemButton
                    key={item.id}
                    aria-current={active ? 'true' : undefined}
                    onClick={() => onChange(item.id)}
                    sx={{
                        mb: 0.5,
                        // Same row metrics as the global Sidebar (see
                        // navRowStyles) — this list reads as a continuation
                        // of it, not a visually different component.
                        ...NAV_ROW_SX,
                        // Driven entirely by sx (not MUI's `selected` prop,
                        // whose own default .Mui-selected styling would win
                        // over a plain bgcolor here) — same approach as the
                        // global Sidebar's active-row styling.
                        ...(active && {
                            bgcolor: 'primary.main',
                            color: 'primary.contrastText',
                            '&:hover': { bgcolor: 'primary.main' },
                            '& .MuiListItemIcon-root': { color: 'inherit' },
                            '& .MuiListItemText-primary': { color: 'primary.contrastText' },
                        }),
                    }}
                >
                    <ListItemIcon sx={{ minWidth: 32, color: 'inherit' }}>{item.icon}</ListItemIcon>
                    <ListItemText
                        primary={item.label}
                        secondary={item.subtitle}
                        slotProps={navRowTextSlotProps(active)}
                        sx={{ minWidth: 0 }}
                    />
                </ListItemButton>
            );
        })}
    </List>
);

export default PlatformSideNav;

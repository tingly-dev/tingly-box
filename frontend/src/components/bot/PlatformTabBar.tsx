import { Box, Chip, Divider } from '@mui/material';
import type { ReactNode } from 'react';

export interface PlatformTabBarItem {
    id: string;
    label: string;
    icon: ReactNode;
    subtitle?: string;
}

interface PlatformTabBarProps {
    items: PlatformTabBarItem[];
    value: string;
    onChange: (id: string) => void;
    /** Draws a vertical divider after this many leading items (e.g. 1, to
     * separate a leading "All" chip from the per-platform ones). */
    dividerAfter?: number;
}

// PlatformTabBar is the platform picker for Overview and Remote — a row of
// chips at the top of the page content. Replaces an earlier vertical
// side-column version that tried to visually extend the app's Sidebar: that
// needed three nav columns side by side, which had no room below desktop
// widths and kept breaking (misaligned rows, wrong background/border,
// scrolling out of sync with the real Sidebar). A horizontal row lives
// entirely inside the normal page content — no breakout margins, no sticky
// positioning, no responsive collapse to get wrong.
const PlatformTabBar: React.FC<PlatformTabBarProps> = ({ items, value, onChange, dividerAfter }) => (
    <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 1, mb: 2 }}>
        {items.map((item, index) => {
            const active = item.id === value;
            return (
                <Box key={item.id} sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Chip
                        onClick={() => onChange(item.id)}
                        color={active ? 'primary' : 'default'}
                        variant={active ? 'filled' : 'outlined'}
                        label={
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
                                <Box sx={{ display: 'flex', alignItems: 'center', '& svg': { fontSize: 16 } }}>{item.icon}</Box>
                                {item.label}
                                {item.subtitle && (
                                    <Box
                                        component="span"
                                        sx={{
                                            fontSize: '0.6875rem',
                                            color: active ? 'rgba(255,255,255,0.75)' : 'text.secondary',
                                        }}
                                    >
                                        {item.subtitle}
                                    </Box>
                                )}
                            </Box>
                        }
                    />
                    {dividerAfter === index + 1 && <Divider orientation="vertical" flexItem sx={{ mx: 0.5, my: 0.5 }} />}
                </Box>
            );
        })}
    </Box>
);

export default PlatformTabBar;

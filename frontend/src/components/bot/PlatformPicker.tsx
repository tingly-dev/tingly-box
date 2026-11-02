import { Box, Tab, Tabs, Typography, useMediaQuery, useTheme } from '@mui/material';
import type { ReactNode } from 'react';

export interface PlatformPickerItem {
    id: string;
    label: string;
    /**
     * Rendered with the tile's selected state. The brand logos default to
     * grayscale (createBrandIcon(..., true)) and carry that filter on the
     * <img> itself — a parent filter can't undo it — so the icon has to be
     * (re)rendered with `grayscale={!active}` to come back to full color
     * when its tile is selected. Hence a render fn, not a static node.
     */
    icon: ReactNode;
    activeIcon?: ReactNode;
    subtitle?: string;
}

interface PlatformPickerProps {
    items: PlatformPickerItem[];
    value: string;
    onChange: (id: string) => void;
}

// Keep all platforms visible beside the connections on wide screens. On
// smaller screens a scrollable horizontal rail preserves the content width.
const PlatformPicker: React.FC<PlatformPickerProps> = ({ items, value, onChange }) => {
    const theme = useTheme();
    const vertical = useMediaQuery(theme.breakpoints.up('lg'));

    return (
        <Tabs
            aria-label="Platforms"
            orientation={vertical ? 'vertical' : 'horizontal'}
            variant="scrollable"
            scrollButtons="auto"
            allowScrollButtonsMobile
            value={value}
            onChange={(_, id: string) => onChange(id)}
            sx={{
                minWidth: 0,
                borderRight: {lg: 1},
                borderBottom: {xs: 1, lg: 0},
                borderColor: 'divider',
                '& .MuiTabs-indicator': {left: vertical ? 0 : undefined},
            }}
        >
            {items.map((item) => {
                const active = item.id === value;
                return (
                    <Tab
                        key={item.id}
                        id={`bot-platform-${item.id}`}
                        aria-controls="bot-platform-panel"
                        value={item.id}
                        icon={<Box sx={{width: 24, height: 24, flexShrink: 0, display: 'grid', placeItems: 'center'}}>
                            {active ? (item.activeIcon || item.icon) : item.icon}
                        </Box>}
                        iconPosition="start"
                        label={<Box sx={{minWidth: 0, textAlign: 'left'}}>
                            <Typography variant="body2" noWrap sx={{fontWeight: 600}}>{item.label}</Typography>
                            {item.subtitle && <Typography variant="caption" noWrap sx={{display: 'block', color: 'text.secondary'}}>
                                {item.subtitle}
                            </Typography>}
                        </Box>}
                        sx={{
                            minHeight: 56,
                            minWidth: {xs: 138, lg: 0},
                            maxWidth: 'none',
                            justifyContent: 'flex-start',
                            textTransform: 'none',
                            gap: 1,
                            px: 1.5,
                            borderRadius: 1,
                            '&.Mui-selected': {bgcolor: 'action.selected'},
                            '&:hover': {bgcolor: 'action.hover'},
                        }}
                    />
                );
            })}
        </Tabs>
    );
};

export default PlatformPicker;

import { Box, ButtonBase, Typography } from '@mui/material';
import type { ReactNode } from 'react';
import { radius } from '@tingly/ui/theme/radius';

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

// The supported platform set is small: show every option above the list,
// wrapping onto additional rows instead of hiding options behind scrolling.
const PlatformPicker: React.FC<PlatformPickerProps> = ({ items, value, onChange }) => (
    <Box
        role="group"
        aria-label="Platforms"
        sx={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fill, minmax(min(140px, 100%), 1fr))',
            gap: 1,
            mb: 2,
        }}
    >
        {items.map((item) => {
            const active = item.id === value;
            return (
                <ButtonBase
                    key={item.id}
                    id={`bot-platform-${item.id}`}
                    aria-controls="bot-platform-panel"
                    aria-pressed={active}
                    onClick={() => onChange(item.id)}
                    sx={{
                        minWidth: 0,
                        minHeight: 56,
                        justifyContent: 'flex-start',
                        gap: 1,
                        px: 1.5,
                        py: 1,
                        borderRadius: radius.card,
                        border: 1,
                        borderColor: active ? 'primary.main' : 'divider',
                        bgcolor: active ? 'action.selected' : 'background.paper',
                        color: active ? 'primary.main' : 'text.primary',
                        '&:hover': {bgcolor: 'action.hover'},
                    }}
                >
                    <Box sx={{width: 24, height: 24, flexShrink: 0, display: 'grid', placeItems: 'center'}}>
                        {active ? (item.activeIcon || item.icon) : item.icon}
                    </Box>
                    <Box sx={{minWidth: 0, textAlign: 'left'}}>
                        <Typography variant="body2" noWrap sx={{fontWeight: 600}}>{item.label}</Typography>
                        {item.subtitle && <Typography variant="caption" noWrap sx={{display: 'block', color: 'text.secondary'}}>
                            {item.subtitle}
                        </Typography>}
                    </Box>
                </ButtonBase>
            );
        })}
    </Box>
);

export default PlatformPicker;

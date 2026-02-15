import React from 'react';
import {
    Select,
    MenuItem,
    ListItemText,
    ListItemIcon,
    Box,
    Typography,
    ListSubheader,
    Chip,
} from '@mui/material';
import { BotPlatformConfig, CategoryLabels } from '../../types/bot';

interface BotPlatformSelectorProps {
    value: string;
    onChange: (platform: string) => void;
    platforms: BotPlatformConfig[];
    disabled?: boolean;
}

// Platform icons (simple emoji for now, could be replaced with SVG icons)
const platformIcons: Record<string, string> = {
    telegram: '✈️',
    slack: '💼',
    discord: '🎮',
    dingtalk: '🔔',
    feishu: '🚀',
    whatsapp: '📱',
};

// Group platforms by category
function groupPlatformsByCategory(platforms: BotPlatformConfig[]): Map<string, BotPlatformConfig[]> {
    const grouped = new Map<string, BotPlatformConfig[]>();
    for (const platform of platforms) {
        const existing = grouped.get(platform.category) || [];
        existing.push(platform);
        grouped.set(platform.category, existing);
    }
    return grouped;
}

export const BotPlatformSelector: React.FC<BotPlatformSelectorProps> = ({
    value,
    onChange,
    platforms,
    disabled = false,
}) => {
    const groupedPlatforms = groupPlatformsByCategory(platforms);

    return (
        <Select
            value={value}
            onChange={(e) => onChange(e.target.value)}
            fullWidth
            size="small"
            disabled={disabled}
        >
            {Array.from(groupedPlatforms.entries()).map(([category, categoryPlatforms]) => (
                <React.Fragment key={category}>
                    <ListSubheader
                        sx={{
                            bgcolor: 'background.paper',
                            fontWeight: 600,
                            fontSize: '0.75rem',
                            color: 'text.secondary',
                        }}
                    >
                        {CategoryLabels[category] || category}
                    </ListSubheader>
                    {categoryPlatforms.map((platform) => (
                        <MenuItem key={platform.platform} value={platform.platform}>
                            <ListItemIcon sx={{ minWidth: 36 }}>
                                <Typography variant="body1">{platformIcons[platform.platform] || '🤖'}</Typography>
                            </ListItemIcon>
                            <ListItemText
                                primary={platform.display_name}
                                secondary={platform.auth_type}
                                secondaryTypographyProps={{
                                    variant: 'caption',
                                    sx: { textTransform: 'capitalize' }
                                }}
                            />
                        </MenuItem>
                    ))}
                </React.Fragment>
            ))}
        </Select>
    );
};

export default BotPlatformSelector;

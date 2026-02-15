import React from 'react';
import {
    Select,
    MenuItem,
    Box,
    Typography,
    CircularProgress,
} from '@mui/material';
import { BotPlatformConfig } from '../../types/bot';

interface BotPlatformSelectorProps {
    value: string;
    onChange: (platform: string) => void;
    platforms: BotPlatformConfig[];
    disabled?: boolean;
    loading?: boolean;
}

export const BotPlatformSelector: React.FC<BotPlatformSelectorProps> = ({
    value,
    onChange,
    platforms,
    disabled = false,
    loading = false,
}) => {
    // Show loading state
    if (loading) {
        return (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, p: 1 }}>
                <CircularProgress size={16} />
                <Typography variant="body2" color="text.secondary">
                    Loading platforms...
                </Typography>
            </Box>
        );
    }

    // Show empty state
    if (platforms.length === 0) {
        return (
            <Box sx={{ p: 1 }}>
                <Typography variant="body2" color="text.secondary">
                    No platforms available. Make sure the remote-coder service is running.
                </Typography>
            </Box>
        );
    }

    return (
        <Select
            value={value}
            onChange={(e) => onChange(e.target.value as string)}
            fullWidth
            size="small"
            disabled={disabled}
        >
            {platforms.map((platform) => (
                <MenuItem key={platform.platform} value={platform.platform}>
                    <Box sx={{ display: 'flex', width: '100%', gap: 2 }}>
                        <Box sx={{ minWidth: 100 }}>{platform.display_name}</Box>
                        <Box sx={{ color: 'text.secondary' }}>{platform.auth_type}</Box>
                    </Box>
                </MenuItem>
            ))}
        </Select>
    );
};

export default BotPlatformSelector;

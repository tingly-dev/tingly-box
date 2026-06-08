import { Check as IconCheck, Circle as IconCircleFilled, KeyboardArrowDown as IconChevronDown } from '@/components/icons';
import {
    Box,
    Button,
    Menu,
    MenuItem,
    ListItemText,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import React, { useState } from 'react';

interface SessionAffinityControlProps {
    value: number;
    onChange: (value: number) => void;
    disabled?: boolean;
}

const AFFINITY_OPTIONS = [
    { value: 0, label: 'Off', description: 'Disable affinity (no cache optimization, may increase cost)' },
    { value: 1800, label: '30 minutes', description: '30 min pin → better cache hits for short sessions' },
    { value: 3600, label: '1 hour', description: '1 hour pin → optimal cache hits for coding sessions' },
    { value: 7200, label: '2 hours', description: '2 hour pin → extended cache optimization for long sessions' },
] as const;

export const SessionAffinityControl: React.FC<SessionAffinityControlProps> = ({
    value,
    onChange,
    disabled = false,
}) => {
    const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null);

    const isEnabled = value > 0;
    const displayValue = isEnabled ? `${value}s` : 'Off';

    const handleMenuOpen = (event: React.MouseEvent<HTMLElement>) => {
        if (!disabled) {
            setMenuAnchor(event.currentTarget);
        }
    };

    const handleMenuClose = () => {
        setMenuAnchor(null);
    };

    const handleChange = (newValue: number) => {
        onChange(newValue);
        handleMenuClose();
    };

    return (
        <>
            <Tooltip
                title={`Session Affinity: ${isEnabled ? `Pin to service → cache hits → faster + cheaper (TTL: ${value}s)` : 'Disabled (no cache optimization)'}`}
                placement="right"
                arrow
            >
                <Button
                    size="small"
                    variant="outlined"
                    onClick={handleMenuOpen}
                    disabled={disabled}
                    endIcon={<IconChevronDown sx={{ fontSize: 18 }} />}
                    sx={{
                        minWidth: 120,
                        textTransform: 'none',
                        whiteSpace: 'nowrap',
                        bgcolor: isEnabled ? 'primary.main' : 'transparent',
                        color: isEnabled ? 'primary.contrastText' : 'text.primary',
                        fontWeight: isEnabled ? 600 : 400,
                        border: isEnabled ? 'none' : '1px solid',
                        borderColor: 'divider',
                        opacity: disabled ? 0.6 : 1,
                        '&:hover': { bgcolor: isEnabled ? 'primary.dark' : 'action.selected' },
                    }}
                >
                    <IconCircleFilled sx={{ fontSize: 14, mr: '4px' }} />
                    Affinity: {displayValue}
                </Button>
            </Tooltip>

            <Menu
                anchorEl={menuAnchor}
                open={Boolean(menuAnchor)}
                onClose={handleMenuClose}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
            >
                {AFFINITY_OPTIONS.map((option) => (
                    <MenuItem
                        key={option.value}
                        selected={value === option.value}
                        onClick={() => handleChange(option.value)}
                        title={option.description}
                    >
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                            <ListItemText primary={option.label} primaryTypographyProps={{ variant: 'body2' }} />
                            {value === option.value && <IconCheck sx={{ fontSize: 16 }} />}
                        </Box>
                    </MenuItem>
                ))}
                <MenuItem sx={{ flexDirection: 'column', alignItems: 'flex-start', gap: 1 }}>
                    <TextField
                        size="small"
                        type="number"
                        label="Custom TTL (seconds)"
                        value={value || ''}
                        onChange={(e) => {
                            const val = parseInt(e.target.value) || 0;
                            if (val >= 0) {
                                handleChange(val);
                            }
                        }}
                        inputProps={{ min: 0, step: 60 }}
                        sx={{ width: 200 }}
                    />
                    <Typography variant="caption" color="text.secondary">
                        Custom TTL (seconds). Longer pin → more cache hits → lower cost
                    </Typography>
                </MenuItem>
            </Menu>
        </>
    );
};

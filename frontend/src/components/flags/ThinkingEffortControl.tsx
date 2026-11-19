import { Check as IconCheck, KeyboardArrowDown as IconChevronDown } from '@/components/icons';
import { Box, Button, ListItemText, Menu, MenuItem, Tooltip } from '@mui/material';
import React, { useState } from 'react';
import {pluginControlStateStyles} from './pluginControlStyles';

export const EFFORT_LEVELS = [
    { value: '', label: 'By Client', description: 'Use the client setting when no rule or scenario override applies' },
    { value: 'off', label: 'Off', description: 'Request that the model disable thinking' },
    { value: 'low', label: 'Low', description: 'Light reasoning, about 4K tokens on budget-based models' },
    { value: 'medium', label: 'Medium', description: 'Balanced reasoning, about 10K tokens on budget-based models' },
    { value: 'high', label: 'High', description: 'Deep reasoning, about 20K tokens on budget-based models' },
    { value: 'xhigh', label: 'XHigh', description: 'Request xhigh using the existing provider mapping. Some upstreams may reject this level.' },
    { value: 'max', label: 'Max', description: 'Maximum requested effort, about 32K tokens on budget-based models' },
] as const;

interface ThinkingEffortControlProps {
    value: string;
    disabled?: boolean;
    onChange: (level: string) => void;
}

const ThinkingEffortControl: React.FC<ThinkingEffortControlProps> = ({ value, disabled, onChange }) => {
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);

    const currentLevel = EFFORT_LEVELS.find(l => l.value === value);
    const isActive = value !== '';
    // Keep saved values such as "minimal" visible even when absent from the menu.
    const label = currentLevel?.label ?? (isActive ? value : 'By Client');
    const description = currentLevel?.description ?? (isActive ? `${value} (legacy level, no longer offered)` : 'By Client');

    return (
        <>
            <Tooltip title={`Thinking: ${description}`} placement="right" arrow>
                <Button
                    size="small"
                    variant="outlined"
                    onClick={(e) => !disabled && setAnchor(e.currentTarget)}
                    disabled={disabled}
                    endIcon={<IconChevronDown sx={{ fontSize: 18 }} />}
                    sx={(theme) => ({
                        minWidth: 110,
                        textTransform: 'none',
                        whiteSpace: 'nowrap',
                        ...pluginControlStateStyles(theme, isActive),
                        opacity: disabled ? 0.6 : 1,
                    })}
                >
                    Thinking: {label}
                </Button>
            </Tooltip>
            <Menu
                anchorEl={anchor}
                open={Boolean(anchor)}
                onClose={() => setAnchor(null)}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
            >
                {EFFORT_LEVELS.map((level) => (
                    <MenuItem
                        key={level.value}
                        selected={level.value === value}
                        onClick={() => { onChange(level.value); setAnchor(null); }}
                        title={level.description}
                    >
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                            <ListItemText primary={level.label} slotProps={{ primary: { variant: 'body2' } }} />
                            {level.value === value && <IconCheck sx={{ fontSize: 16 }} />}
                        </Box>
                    </MenuItem>
                ))}
            </Menu>
        </>
    );
};

export default ThinkingEffortControl;

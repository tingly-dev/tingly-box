import { Check as IconCheck, KeyboardArrowDown as IconChevronDown } from '@/components/icons';
import { Box, Button, ListItemText, Menu, MenuItem, Tooltip } from '@mui/material';
import React, { useState } from 'react';

// "minimal" and "xhigh" are intentionally left out: outside a handful of the
// newest OpenAI/Anthropic models they silently collapse onto "low"/"high"/
// "max" server-side, so offering them as distinct menu choices was mostly a
// false promise of precision (see .design/rule-flags.md). Rules already
// persisted with those values keep working — they're just not selectable here.
export const EFFORT_LEVELS = [
    { value: '', label: 'By Client', description: "Pass the client's thinking config through unchanged" },
    { value: 'off', label: 'Off', description: 'Force extended thinking disabled' },
    { value: 'low', label: 'Low', description: '~4K tokens — Fast' },
    { value: 'medium', label: 'Medium', description: '~10K tokens — Balanced' },
    { value: 'high', label: 'High', description: '~20K tokens — Deep' },
    { value: 'max', label: 'Max', description: '~32K tokens — Max quality' },
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
    // A rule can still carry a legacy "minimal"/"xhigh" value that's no longer
    // in the menu (see EFFORT_LEVELS comment) — show it verbatim rather than
    // mislabeling an active override as "By Client".
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
                    sx={{
                        minWidth: 110,
                        textTransform: 'none',
                        whiteSpace: 'nowrap',
                        bgcolor: isActive ? 'primary.main' : 'transparent',
                        color: isActive ? 'primary.contrastText' : 'text.primary',
                        fontWeight: isActive ? 600 : 400,
                        border: isActive ? 'none' : '1px solid',
                        borderColor: 'divider',
                        opacity: disabled ? 0.6 : 1,
                        '&:hover': { bgcolor: isActive ? 'primary.dark' : 'action.selected' },
                    }}
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

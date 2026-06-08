import { Check as IconCheck, KeyboardArrowDown as IconChevronDown } from '@/components/icons';
import { Box, Button, ListItemText, Menu, MenuItem, Tooltip } from '@mui/material';
import React, { useState } from 'react';

export const EFFORT_LEVELS = [
    { value: '', label: 'By Client', description: "Pass the client's thinking config through unchanged" },
    { value: 'off', label: 'Off', description: 'Force extended thinking disabled' },
    { value: 'low', label: 'Low', description: '~1K tokens — Fast' },
    { value: 'medium', label: 'Medium', description: '~5K tokens — Balanced' },
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

    return (
        <>
            <Tooltip title={`Thinking: ${currentLevel?.description || 'By Client'}`} placement="right" arrow>
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
                        border: isActive ? 'none' : '1px solid',
                        borderColor: 'divider',
                        opacity: disabled ? 0.6 : 1,
                        '&:hover': { bgcolor: isActive ? 'primary.dark' : 'action.selected' },
                    }}
                >
                    Thinking: {currentLevel?.label || 'By Client'}
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
                            <ListItemText primary={level.label} primaryTypographyProps={{ variant: 'body2' }} />
                            {level.value === value && <IconCheck sx={{ fontSize: 16 }} />}
                        </Box>
                    </MenuItem>
                ))}
            </Menu>
        </>
    );
};

export default ThinkingEffortControl;

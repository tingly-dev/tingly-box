import { Check as IconCheck, KeyboardArrowDown as IconChevronDown, Circle as IconCircleFilled } from '@/components/icons';
import { Box, Button, ListItemText, Menu, MenuItem, Tooltip } from '@mui/material';
import React, { useState } from 'react';

export const RECORD_V2_MODES = [
    { value: '', label: 'Off', description: 'Recording disabled' },
    { value: 'request', label: 'Request Only', description: 'Record the final outbound request only' },
    { value: 'request_response', label: 'Request + Response', description: 'Record the final outbound request and final response' },
    { value: 'staged_request_response', label: 'Request + Transform + Response', description: 'Record original request, transformed request, and final response' },
] as const;

interface RecordingV2ControlProps {
    value: string;
    disabled?: boolean;
    onChange: (mode: string) => void;
}

const RecordingV2Control: React.FC<RecordingV2ControlProps> = ({ value, disabled, onChange }) => {
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);

    const currentMode = RECORD_V2_MODES.find(m => m.value === value);
    const isActive = value !== '';

    return (
        <>
            <Tooltip
                title={`Recording V2: ${currentMode?.description || 'Disabled'}${isActive ? ' (enabled)' : ' (disabled)'}`}
                placement="right"
                arrow
            >
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
                    <IconCircleFilled sx={{ fontSize: 14, mr: '4px' }} />
                    Record: {currentMode?.label || 'Off'}
                </Button>
            </Tooltip>
            <Menu
                anchorEl={anchor}
                open={Boolean(anchor)}
                onClose={() => setAnchor(null)}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
            >
                {RECORD_V2_MODES.map((mode) => (
                    <MenuItem
                        key={mode.value}
                        selected={mode.value === value}
                        onClick={() => { onChange(mode.value); setAnchor(null); }}
                        title={mode.description}
                    >
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                            <ListItemText primary={mode.label} primaryTypographyProps={{ variant: 'body2' }} />
                            {mode.value === value && <IconCheck sx={{ fontSize: 16 }} />}
                        </Box>
                    </MenuItem>
                ))}
            </Menu>
        </>
    );
};

export default RecordingV2Control;

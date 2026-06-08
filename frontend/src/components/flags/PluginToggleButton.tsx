import { Check as IconCheck, KeyboardArrowDown as IconChevronDown } from '@/components/icons';
import { Button, ListItemText, Menu, MenuItem, Tooltip } from '@mui/material';
import React, { useState } from 'react';

interface PluginToggleButtonProps {
    label: string;
    description: string;
    value: boolean;
    disabled?: boolean;
    onChange: (value: boolean) => void;
}

const PluginToggleButton: React.FC<PluginToggleButtonProps> = ({ label, description, value, disabled, onChange }) => {
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);

    return (
        <>
            <Tooltip title={`${label}: ${description} (${value ? 'On' : 'Off'})`} placement="right" arrow>
                <Button
                    size="small"
                    variant="outlined"
                    onClick={(e) => !disabled && setAnchor(e.currentTarget)}
                    disabled={disabled}
                    endIcon={<IconChevronDown sx={{ fontSize: 18 }} />}
                    sx={{
                        minWidth: 100,
                        textTransform: 'none',
                        whiteSpace: 'nowrap',
                        bgcolor: value ? 'primary.main' : 'transparent',
                        color: value ? 'primary.contrastText' : 'text.primary',
                        fontWeight: value ? 600 : 400,
                        border: value ? 'none' : '1px solid',
                        borderColor: 'divider',
                        opacity: disabled ? 0.6 : 1,
                        '&:hover': { bgcolor: value ? 'primary.dark' : 'action.selected' },
                    }}
                >
                    {label}: {value ? 'On' : 'Off'}
                </Button>
            </Tooltip>
            <Menu
                anchorEl={anchor}
                open={Boolean(anchor)}
                onClose={() => setAnchor(null)}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
            >
                <MenuItem
                    selected={value}
                    onClick={() => { onChange(true); setAnchor(null); }}
                    title={description}
                >
                    <ListItemText primary="On" primaryTypographyProps={{ variant: 'body2' }} />
                    {value && <IconCheck sx={{ fontSize: 16 }} />}
                </MenuItem>
                <MenuItem
                    selected={!value}
                    onClick={() => { onChange(false); setAnchor(null); }}
                    title={description}
                >
                    <ListItemText primary="Off" primaryTypographyProps={{ variant: 'body2' }} />
                    {!value && <IconCheck sx={{ fontSize: 16 }} />}
                </MenuItem>
            </Menu>
        </>
    );
};

export default PluginToggleButton;

import { Block as InactiveIcon, CheckCircle as ActiveIcon, Delete as DeleteIcon, Download as ExportIcon, PlayArrow as ProbeIcon, Settings as SettingsIcon, SmartDisplay as SmartIcon } from '@mui/icons-material';
import { IconButton, Menu, MenuItem, Tooltip } from '@mui/material';
import React, { useCallback } from 'react';

export interface GraphSettingsMenuProps {
    // Common props
    smartEnabled: boolean;
    canProbe: boolean;
    isProbing: boolean;
    allowDeleteRule: boolean;
    active: boolean;
    allowToggleRule: boolean;
    saving: boolean;

    // Callbacks
    onToggleSmartRouting: () => void;
    onProbe: () => void;
    onExport: () => void;
    onDelete: () => void;
    onToggleActive: () => void;
}

export const GraphSettingsMenu: React.FC<GraphSettingsMenuProps> = ({
    smartEnabled,
    canProbe,
    isProbing,
    allowDeleteRule,
    active,
    allowToggleRule,
    saving,
    onToggleSmartRouting,
    onProbe,
    onExport,
    onDelete,
    onToggleActive,
}) => {
    const [menuAnchorEl, setMenuAnchorEl] = React.useState<null | HTMLElement>(null);
    const menuOpen = Boolean(menuAnchorEl);

    const handleMenuOpen = useCallback((event: React.MouseEvent<HTMLElement>) => {
        setMenuAnchorEl(event.currentTarget);
    }, []);

    const handleMenuClose = useCallback(() => {
        setMenuAnchorEl(null);
    }, []);

    const handleToggleSmartRouting = useCallback(() => {
        handleMenuClose();
        onToggleSmartRouting();
    }, [onToggleSmartRouting]);

    const handleProbe = useCallback(() => {
        handleMenuClose();
        onProbe();
    }, [onProbe]);

    const handleExport = useCallback(() => {
        handleMenuClose();
        onExport();
    }, [onExport]);

    const handleDelete = useCallback(() => {
        handleMenuClose();
        onDelete();
    }, [onDelete]);

    const handleToggleActive = useCallback(() => {
        handleMenuClose();
        onToggleActive();
    }, [onToggleActive]);

    return (
        <>
            <Tooltip title="Rule actions">
                <IconButton
                    size="small"
                    onClick={handleMenuOpen}
                    sx={{
                        color: 'text.secondary',
                        '&:hover': {
                            backgroundColor: 'action.hover',
                        },
                    }}
                >
                    <SettingsIcon fontSize="small" />
                </IconButton>
            </Tooltip>
            <Menu
                anchorEl={menuAnchorEl}
                open={menuOpen}
                onClose={handleMenuClose}
                anchorOrigin={{
                    vertical: 'bottom',
                    horizontal: 'right',
                }}
                transformOrigin={{
                    vertical: 'top',
                    horizontal: 'right',
                }}
            >
                {/* Test Connection */}
                <MenuItem
                    onClick={handleProbe}
                    disabled={!canProbe || isProbing}
                >
                    <ProbeIcon fontSize="small" sx={{ mr: 1 }} />
                    Test Connection
                </MenuItem>

                {/* Export with API Keys */}
                <MenuItem onClick={handleExport}>
                    <ExportIcon fontSize="small" sx={{ mr: 1 }} />
                    Export with API Keys
                </MenuItem>

                {/* Toggle Active/Inactive */}
                <MenuItem
                    onClick={handleToggleActive}
                    disabled={!allowToggleRule || saving}
                    sx={{
                        color: active ? 'warning.main' : 'success.main',
                    }}
                >
                    {active ? (
                        <>
                            <InactiveIcon fontSize="small" sx={{ mr: 1 }} />
                            Deactivate Rule
                        </>
                    ) : (
                        <>
                            <ActiveIcon fontSize="small" sx={{ mr: 1 }} />
                            Activate Rule
                        </>
                    )}
                </MenuItem>

                {/* Delete Rule */}
                {allowDeleteRule && (
                    <MenuItem
                        onClick={handleDelete}
                        sx={{ color: 'error.main' }}
                    >
                        <DeleteIcon fontSize="small" sx={{ mr: 1 }} />
                        Delete Rule
                    </MenuItem>
                )}

                {/* Toggle Smart Routing */}
                <MenuItem onClick={handleToggleSmartRouting}>
                    <SmartIcon fontSize="small" sx={{ mr: 1 }} />
                    {smartEnabled ? 'Convert To Direct Routing' : 'Convert To Smart Routing'}
                </MenuItem>
            </Menu>
        </>
    );
};

export default GraphSettingsMenu;

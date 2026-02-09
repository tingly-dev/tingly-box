import { Block as InactiveIcon, CheckCircle as ActiveIcon, ContentCopy as CopyIcon, Delete as DeleteIcon, Download as ExportIcon, PlayArrow as ProbeIcon, Settings as SettingsIcon, SmartDisplay as SmartIcon, UnfoldMore as ExportMenuIcon } from '@mui/icons-material';
import { IconButton, Menu, MenuItem, Tooltip } from '@mui/material';
import React, { useCallback, useState } from 'react';
import type { ExportFormat } from '@/components/rule-card/utils';

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
    onExport: (format: ExportFormat) => void;
    onExportAsBase64ToClipboard?: () => void;
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
    onExportAsBase64ToClipboard,
    onDelete,
    onToggleActive,
}) => {
    const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
    const [exportMenuAnchorEl, setExportMenuAnchorEl] = useState<null | HTMLElement>(null);
    const menuOpen = Boolean(menuAnchorEl);
    const exportMenuOpen = Boolean(exportMenuAnchorEl);

    const handleMenuOpen = useCallback((event: React.MouseEvent<HTMLElement>) => {
        setMenuAnchorEl(event.currentTarget);
    }, []);

    const handleMenuClose = useCallback(() => {
        setMenuAnchorEl(null);
    }, []);

    const handleExportMenuOpen = useCallback((event: React.MouseEvent<HTMLElement>) => {
        setExportMenuAnchorEl(event.currentTarget);
        handleMenuClose();
    }, [handleMenuClose]);

    const handleExportMenuClose = useCallback(() => {
        setExportMenuAnchorEl(null);
    }, []);

    const handleToggleSmartRouting = useCallback(() => {
        handleMenuClose();
        onToggleSmartRouting();
    }, [onToggleSmartRouting]);

    const handleProbe = useCallback(() => {
        handleMenuClose();
        onProbe();
    }, [onProbe]);

    const handleExportAsJsonl = useCallback(() => {
        handleMenuClose();
        onExport('jsonl');
    }, [onExport]);

    const handleExportAsBase64File = useCallback(() => {
        handleMenuClose();
        onExport('base64');
    }, [onExport]);

    const handleExportAsBase64ToClipboard = useCallback(() => {
        handleMenuClose();
        onExportAsBase64ToClipboard?.();
    }, [onExportAsBase64ToClipboard]);

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

                {/* Export Submenu */}
                <MenuItem onClick={handleExportMenuOpen}>
                    <ExportIcon fontSize="small" sx={{ mr: 1 }} />
                    Export
                    <ExportMenuIcon fontSize="small" sx={{ ml: 1, fontSize: '1rem' }} />
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

            {/* Export Submenu */}
            <Menu
                anchorEl={exportMenuAnchorEl}
                open={exportMenuOpen}
                onClose={handleExportMenuClose}
                anchorOrigin={{
                    vertical: 'bottom',
                    horizontal: 'right',
                }}
                transformOrigin={{
                    vertical: 'top',
                    horizontal: 'left',
                }}
            >
                <MenuItem onClick={handleExportAsJsonl}>
                    <DownloadIcon fontSize="small" sx={{ mr: 1 }} />
                    Download as JSONL
                </MenuItem>
                <MenuItem onClick={handleExportAsBase64File}>
                    <DownloadIcon fontSize="small" sx={{ mr: 1 }} />
                    Download as Base64
                </MenuItem>
                {onExportAsBase64ToClipboard && (
                    <MenuItem onClick={handleExportAsBase64ToClipboard}>
                        <CopyIcon fontSize="small" sx={{ mr: 1 }} />
                        Copy Base64 to Clipboard
                    </MenuItem>
                )}
            </Menu>
        </>
    );
};

export default GraphSettingsMenu;

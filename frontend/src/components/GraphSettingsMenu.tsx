import {
    Block as InactiveIcon,
    CheckCircle as ActiveIcon,
    ContentCopy as CopyIcon,
    Delete as DeleteIcon,
    Download as ExportIcon,
    Download as DownloadIcon,
    Edit as EditIcon,
    PlayArrow as ProbeIcon,
    Settings as SettingsIcon,
    UnfoldMore as ExportMenuIcon
} from '@mui/icons-material';
import { IconButton, Menu, MenuItem, Tooltip } from '@mui/material';
import { useState } from 'react';
import type { ExportFormat } from '@/components/rule-card/utils';

export interface GraphSettingsMenuProps {
    canProbe: boolean;
    isProbing: boolean;
    allowDeleteRule: boolean;
    active: boolean;
    allowToggleRule: boolean;
    saving: boolean;
    cursorCompatEnabled?: boolean;
    cursorCompatAutoEnabled?: boolean;
    onProbe: () => void;
    onExport: (format: ExportFormat) => void;
    onExportAsJsonlToClipboard?: () => void;
    onExportAsBase64ToClipboard?: () => void;
    onDelete: () => void;
    onToggleActive: () => void;
    onToggleCursorCompat?: () => void;
    onToggleCursorCompatAuto?: () => void;
    onEditFlags?: () => void;
}

export const GraphSettingsMenu = ({
    canProbe,
    isProbing,
    allowDeleteRule,
    active,
    allowToggleRule,
    saving,
    cursorCompatEnabled,
    cursorCompatAutoEnabled,
    onProbe,
    onExport,
    onExportAsJsonlToClipboard,
    onExportAsBase64ToClipboard,
    onDelete,
    onToggleActive,
    onToggleCursorCompat,
    onToggleCursorCompatAuto,
    onEditFlags,
}: GraphSettingsMenuProps) => {
    const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
    const [exportMenuAnchorEl, setExportMenuAnchorEl] = useState<null | HTMLElement>(null);

    const closeMenu = () => setMenuAnchorEl(null);
    const closeExportMenu = () => setExportMenuAnchorEl(null);
    const closeAllMenus = () => {
        setMenuAnchorEl(null);
        setExportMenuAnchorEl(null);
    };

    return (
        <>
            <Tooltip title="Rule actions">
                <IconButton
                    size="small"
                    onClick={(e) => setMenuAnchorEl(e.currentTarget)}
                    sx={{ color: 'text.secondary', '&:hover': { backgroundColor: 'action.hover' } }}
                >
                    <SettingsIcon fontSize="small" />
                </IconButton>
            </Tooltip>

            <Menu
                anchorEl={menuAnchorEl}
                open={Boolean(menuAnchorEl)}
                onClose={closeMenu}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                transformOrigin={{ vertical: 'top', horizontal: 'right' }}
            >
                <MenuItem onClick={() => { closeMenu(); onProbe(); }} disabled={!canProbe || isProbing}>
                    <ProbeIcon fontSize="small" sx={{ mr: 1 }} />Test Connection
                </MenuItem>

                <MenuItem onClick={(e) => { setExportMenuAnchorEl(e.currentTarget); closeMenu(); }}>
                    <ExportIcon fontSize="small" sx={{ mr: 1 }} />Export
                    <ExportMenuIcon fontSize="small" sx={{ ml: 1, fontSize: '1rem' }} />
                </MenuItem>

                <MenuItem
                    onClick={() => { closeMenu(); onToggleActive(); }}
                    disabled={!allowToggleRule || saving}
                    sx={{ color: active ? 'warning.main' : 'success.main' }}
                >
                    {active ? (
                        <>
                            <InactiveIcon fontSize="small" sx={{ mr: 1 }} />Deactivate Rule
                        </>
                    ) : (
                        <>
                            <ActiveIcon fontSize="small" sx={{ mr: 1 }} />Activate Rule
                        </>
                    )}
                </MenuItem>

                {onToggleCursorCompat && (
                    <MenuItem onClick={() => { closeMenu(); onToggleCursorCompat(); }}>
                        {cursorCompatEnabled ? (
                            <>
                                <ActiveIcon fontSize="small" sx={{ mr: 1 }} />Cursor Compatibility: On
                            </>
                        ) : (
                            <>
                                <InactiveIcon fontSize="small" sx={{ mr: 1 }} />Cursor Compatibility: Off
                            </>
                        )}
                    </MenuItem>
                )}

                {onToggleCursorCompatAuto && (
                    <MenuItem onClick={() => { closeMenu(); onToggleCursorCompatAuto(); }}>
                        {cursorCompatAutoEnabled ? (
                            <>
                                <ActiveIcon fontSize="small" sx={{ mr: 1 }} />Cursor Auto-Detect: On
                            </>
                        ) : (
                            <>
                                <InactiveIcon fontSize="small" sx={{ mr: 1 }} />Cursor Auto-Detect: Off
                            </>
                        )}
                    </MenuItem>
                )}

                {onEditFlags && (
                    <MenuItem onClick={() => { closeMenu(); onEditFlags(); }}>
                        <EditIcon fontSize="small" sx={{ mr: 1 }} />Edit flag
                    </MenuItem>
                )}

                {allowDeleteRule && (
                    <MenuItem onClick={() => { closeMenu(); onDelete(); }} sx={{ color: 'error.main' }}>
                        <DeleteIcon fontSize="small" sx={{ mr: 1 }} />Delete Rule
                    </MenuItem>
                )}
            </Menu>

            <Menu
                anchorEl={exportMenuAnchorEl}
                open={Boolean(exportMenuAnchorEl)}
                onClose={closeExportMenu}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
            >
                <MenuItem onClick={() => { closeAllMenus(); onExport('jsonl'); }}>
                    <DownloadIcon fontSize="small" sx={{ mr: 1 }} />Download as JSONL
                </MenuItem>
                <MenuItem onClick={() => { closeAllMenus(); onExport('base64'); }}>
                    <DownloadIcon fontSize="small" sx={{ mr: 1 }} />Download as Base64
                </MenuItem>
                {onExportAsJsonlToClipboard && (
                    <MenuItem onClick={() => { closeAllMenus(); onExportAsJsonlToClipboard(); }}>
                        <CopyIcon fontSize="small" sx={{ mr: 1 }} />Copy JSONL to Clipboard
                    </MenuItem>
                )}
                {onExportAsBase64ToClipboard && (
                    <MenuItem onClick={() => { closeAllMenus(); onExportAsBase64ToClipboard(); }}>
                        <CopyIcon fontSize="small" sx={{ mr: 1 }} />Copy Base64 to Clipboard
                    </MenuItem>
                )}
            </Menu>
        </>
    );
};

export default GraphSettingsMenu;

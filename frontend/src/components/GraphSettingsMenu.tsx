import {
    Block as InactiveIcon,
    CheckCircle as ActiveIcon,
    ContentCopy as CopyIcon,
    Delete as DeleteIcon,
    Download as ExportIcon,
    Download as DownloadIcon,
    PlayArrow as ProbeIcon,
    Settings as SettingsIcon,
    UnfoldMore as ExportMenuIcon,
    Speed as SpeedIcon,
    Stream as StreamIcon,
    Build as ToolIcon,
} from '@mui/icons-material';
import { IconButton, Menu, MenuItem, Tooltip, Divider } from '@mui/material';
import { useState } from 'react';
import type { ExportFormat } from '@/components/rule-card/utils';
import { ProbeMenu } from './probe';

export interface GraphSettingsMenuProps {
    allowDeleteRule: boolean;
    active: boolean;
    allowToggleRule: boolean;
    saving: boolean;
    onExport: (format: ExportFormat) => void;
    onExportAsJsonlToClipboard?: () => void;
    onExportAsBase64ToClipboard?: () => void;
    onDelete: () => void;
    onToggleActive: () => void;
    // Probe V2 props
    ruleUuid?: string;
    ruleName?: string;
    scenario?: string;
    model?: string;
}

export const GraphSettingsMenu = ({
    allowDeleteRule,
    active,
    allowToggleRule,
    saving,
    onExport,
    onExportAsJsonlToClipboard,
    onExportAsBase64ToClipboard,
    onDelete,
    onToggleActive,
    ruleUuid,
    ruleName,
    scenario,
    model,
}: GraphSettingsMenuProps) => {
    const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
    const [exportMenuAnchorEl, setExportMenuAnchorEl] = useState<null | HTMLElement>(null);
    const [probeAnchorEl, setProbeAnchorEl] = useState<null | HTMLElement>(null);

    const closeMenu = () => setMenuAnchorEl(null);
    const closeExportMenu = () => setExportMenuAnchorEl(null);
    const closeAllMenus = () => {
        setMenuAnchorEl(null);
        setExportMenuAnchorEl(null);
    };

    const handleProbeClose = () => {
        setProbeAnchorEl(null);
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
                <MenuItem onClick={(e) => { setProbeAnchorEl(e.currentTarget); closeMenu(); }}>
                    <ProbeIcon fontSize="small" sx={{ mr: 1 }} />Test Probe
                    <ExportMenuIcon fontSize="small" sx={{ ml: 1, fontSize: '1rem' }} />
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

            {/* Probe V3 Menu */}
            {ruleUuid && (
                <ProbeMenu
                    anchorEl={probeAnchorEl}
                    open={Boolean(probeAnchorEl)}
                    onClose={handleProbeClose}
                    targetType="rule"
                    targetId={ruleUuid}
                    targetName={ruleName || ruleUuid}
                    scenario={scenario}
                    model={model}
                />
            )}
        </>
    );
};

export default GraphSettingsMenu;

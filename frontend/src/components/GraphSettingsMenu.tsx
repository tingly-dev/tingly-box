import {
    Block as InactiveIcon,
    CheckCircle as ActiveIcon,
    ContentCopy as CopyIcon,
    Delete as DeleteIcon,
    Edit as EditIcon,
    PlayArrow as ProbeIcon,
    Settings as SettingsIcon,
} from '@/components/icons';
import { IconButton, Menu, MenuItem, Tooltip, Divider } from '@mui/material';
import { useState } from 'react';
import { ProbeMenu } from './probe';

export interface GraphSettingsMenuProps {
    allowDeleteRule: boolean;
    active: boolean;
    allowToggleRule: boolean;
    saving: boolean;
    onExportAsJsonlToClipboard?: () => void;
    onExportAsBase64ToClipboard?: () => void;
    onDelete: () => void;
    onToggleActive: () => void;
    onEditFlags?: () => void;
    // Probe props
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
    onExportAsJsonlToClipboard,
    onExportAsBase64ToClipboard,
    onDelete,
    onToggleActive,
    onEditFlags,
    ruleUuid,
    ruleName,
    scenario,
    model,
}: GraphSettingsMenuProps) => {
    const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
    const [probeOpen, setProbeOpen] = useState(false);

    const closeMenu = () => setMenuAnchorEl(null);

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
                {ruleUuid && (
                    <MenuItem onClick={() => { closeMenu(); setProbeOpen(true); }}>
                        <ProbeIcon fontSize="small" sx={{ mr: 1 }} />Test Probe
                    </MenuItem>
                )}

                {(onExportAsBase64ToClipboard || onExportAsJsonlToClipboard) && <Divider />}

                {onExportAsBase64ToClipboard && (
                    <MenuItem onClick={() => { closeMenu(); onExportAsBase64ToClipboard(); }}>
                        <CopyIcon fontSize="small" sx={{ mr: 1 }} />Copy Base64
                    </MenuItem>
                )}
                {onExportAsJsonlToClipboard && (
                    <MenuItem onClick={() => { closeMenu(); onExportAsJsonlToClipboard(); }}>
                        <CopyIcon fontSize="small" sx={{ mr: 1 }} />Copy JSONL
                    </MenuItem>
                )}

                <Divider />

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

            {/* Probe Dialog */}
            {ruleUuid && (
                <ProbeMenu
                    open={probeOpen}
                    onClose={() => setProbeOpen(false)}
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

import {
    Block as InactiveIcon,
    CheckCircle as ActiveIcon,
    ContentCopy as CopyIcon,
    Delete as DeleteIcon,
    Edit as EditIcon,
    Extension as PluginsIcon,
    MoreHoriz as MoreIcon,
    PlayArrow as ProbeIcon,
    Decisions as SmartIcon,
} from '@/components/icons';
import { Box, IconButton, Menu, MenuItem, Tooltip, Divider } from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProbeMenu } from './probe';

export interface GraphSettingsMenuProps {
    allowDeleteRule: boolean;
    active: boolean;
    allowToggleRule: boolean;
    saving: boolean;
    onExportAsJsonlToClipboard?: () => void;
    onExportAsBase64ToClipboard?: () => void;
    /** Receives the ⋯ button so the delete confirmation can open right there. */
    onDelete: (anchorEl: HTMLElement | null) => void;
    onToggleActive: () => void;
    onEditFlags?: () => void;
    /** Opens the plugin catalog (add / remove); `pluginsCount` is shown beside it. */
    onOpenPlugins?: () => void;
    pluginsCount?: number;
    /** Smart conditions on/off/add item; hidden when omitted. */
    smartCondition?: { state: 'none' | 'on' | 'off'; onToggle: () => void };
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
    onOpenPlugins,
    pluginsCount = 0,
    smartCondition,
    ruleUuid,
    ruleName,
    scenario,
    model,
}: GraphSettingsMenuProps) => {
    const { t } = useTranslation();
    const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
    const [probeOpen, setProbeOpen] = useState(false);

    const closeMenu = () => setMenuAnchorEl(null);

    return (
        <>
            <Tooltip title={t('ruleHeader.ruleActions')}>
                <IconButton
                    size="small"
                    onClick={(e) => setMenuAnchorEl(e.currentTarget)}
                    aria-label={t('ruleHeader.ruleActions')}
                    sx={{ color: 'text.secondary', '&:hover': { backgroundColor: 'action.hover' } }}
                >
                    <MoreIcon fontSize="small" />
                </IconButton>
            </Tooltip>

            <Menu
                anchorEl={menuAnchorEl}
                open={Boolean(menuAnchorEl)}
                onClose={closeMenu}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                transformOrigin={{ vertical: 'top', horizontal: 'right' }}
            >
                {onOpenPlugins && (
                    <MenuItem onClick={() => { closeMenu(); onOpenPlugins(); }}>
                        <PluginsIcon fontSize="small" sx={{ mr: 1 }} />{t('ruleHeader.plugins')}
                        {pluginsCount > 0 && (
                            <Box component="span" sx={{ ml: 'auto', pl: 2, color: 'text.secondary', fontSize: '0.8125rem' }}>{pluginsCount}</Box>
                        )}
                    </MenuItem>
                )}

                {smartCondition && (
                    <MenuItem
                        onClick={() => { closeMenu(); smartCondition.onToggle(); }}
                        disabled={saving}
                    >
                        <SmartIcon fontSize="small" sx={{ mr: 1 }} />
                        {t({ none: 'ruleHeader.smartConditionAdd', on: 'ruleHeader.smartConditionOn', off: 'ruleHeader.smartConditionOff' }[smartCondition.state])}
                    </MenuItem>
                )}

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
                    <MenuItem onClick={() => { const anchor = menuAnchorEl; closeMenu(); onDelete(anchor); }} sx={{ color: 'error.main' }}>
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

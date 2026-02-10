import React, { useState, useCallback } from 'react';
import { Add as AddIcon, Key as KeyIcon, ExpandMore as ExpandMoreIcon, UnfoldMore as UnfoldMoreIcon, Settings as SettingsIcon, Upload as ImportIcon } from '@mui/icons-material';
import { Button, IconButton, Menu, MenuItem, Stack, Tooltip } from '@mui/material';

export interface TemplatePageActionsProps {
    collapsible: boolean;
    allExpanded: boolean;
    onToggleExpandAll: () => void;
    showAddApiKeyButton: boolean;
    onAddApiKeyClick: () => void;
    showCreateRuleButton: boolean;
    onCreateRule: () => void;
    showExpandCollapseButton: boolean;
    onImportFromClipboard?: () => void;
}

export const TemplatePageActions: React.FC<TemplatePageActionsProps> = ({
    collapsible,
    allExpanded,
    onToggleExpandAll,
    showAddApiKeyButton,
    onAddApiKeyClick,
    showCreateRuleButton,
    onCreateRule,
    showExpandCollapseButton,
    onImportFromClipboard,
}) => {
    const [settingsMenuAnchorEl, setSettingsMenuAnchorEl] = useState<null | HTMLElement>(null);
    const settingsMenuOpen = Boolean(settingsMenuAnchorEl);

    const handleSettingsMenuOpen = useCallback((event: React.MouseEvent<HTMLElement>) => {
        setSettingsMenuAnchorEl(event.currentTarget);
    }, []);

    const handleSettingsMenuClose = useCallback(() => {
        setSettingsMenuAnchorEl(null);
    }, []);

    const handleAddApiKeyClick = useCallback(() => {
        handleSettingsMenuClose();
        onAddApiKeyClick();
    }, [onAddApiKeyClick, handleSettingsMenuClose]);

    const handleImportFromClipboard = useCallback(() => {
        handleSettingsMenuClose();
        onImportFromClipboard?.();
    }, [onImportFromClipboard, handleSettingsMenuClose]);

    return (
        <Stack direction="row" spacing={1}>
            {showExpandCollapseButton && collapsible && (
                <Tooltip title={allExpanded ? "Collapse all rules" : "Expand all rules"}>
                    <Button
                        variant="outlined"
                        startIcon={allExpanded ? <UnfoldMoreIcon /> : <ExpandMoreIcon />}
                        onClick={onToggleExpandAll}
                        size="small"
                    >
                        {allExpanded ? "Collapse" : "Expand"}
                    </Button>
                </Tooltip>
            )}
            {showCreateRuleButton && (
                <Tooltip title="Create new routing rule">
                    <Button
                        variant="contained"
                        startIcon={<AddIcon />}
                        onClick={onCreateRule}
                        size="small"
                    >
                        New Rule
                    </Button>
                </Tooltip>
            )}
            <Tooltip title="Settings">
                <IconButton
                    size="small"
                    onClick={handleSettingsMenuOpen}
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
                anchorEl={settingsMenuAnchorEl}
                open={settingsMenuOpen}
                onClose={handleSettingsMenuClose}
                anchorOrigin={{
                    vertical: 'bottom',
                    horizontal: 'right',
                }}
                transformOrigin={{
                    vertical: 'top',
                    horizontal: 'right',
                }}
            >
                {showAddApiKeyButton && (
                    <MenuItem onClick={handleAddApiKeyClick}>
                        <KeyIcon fontSize="small" sx={{ mr: 1 }} />
                        New Key
                    </MenuItem>
                )}
                {onImportFromClipboard && (
                    <MenuItem onClick={handleImportFromClipboard}>
                        <ImportIcon fontSize="small" sx={{ mr: 1 }} />
                        Import Rule & Key
                    </MenuItem>
                )}
            </Menu>
        </Stack>
    );
};

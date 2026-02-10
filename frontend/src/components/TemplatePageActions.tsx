import React from 'react';
import { Add as AddIcon, Key as KeyIcon, ExpandMore as ExpandMoreIcon, UnfoldMore as UnfoldMoreIcon, Upload as ImportIcon } from '@mui/icons-material';
import { Button, Stack, Tooltip } from '@mui/material';

export interface TemplatePageActionsProps {
    collapsible: boolean;
    allExpanded: boolean;
    onToggleExpandAll: () => void;
    showAddApiKeyButton: boolean;
    onAddApiKeyClick: () => void;
    showCreateRuleButton: boolean;
    onCreateRule: () => void;
    showExpandCollapseButton: boolean;
    showImportButton: boolean;
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
    showImportButton,
    onImportFromClipboard,
}) => {
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
            {showAddApiKeyButton && (
                <Tooltip title="Add new API Key">
                    <Button
                        variant="outlined"
                        startIcon={<KeyIcon />}
                        onClick={onAddApiKeyClick}
                        size="small"
                    >
                        New Key
                    </Button>
                </Tooltip>
            )}
            {showImportButton && onImportFromClipboard && (
                <Tooltip title="Import rule and keys from file or clipboard">
                    <Button
                        variant="outlined"
                        startIcon={<ImportIcon />}
                        onClick={onImportFromClipboard}
                        size="small"
                    >
                        Import
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
        </Stack>
    );
};

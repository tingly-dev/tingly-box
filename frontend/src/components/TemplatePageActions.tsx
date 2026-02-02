import React from 'react';
import { Add as AddIcon, Key as KeyIcon, ExpandMore as ExpandMoreIcon, UnfoldMore as UnfoldMoreIcon } from '@mui/icons-material';
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

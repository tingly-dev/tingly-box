import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
    Add as AddIcon,
    BugReport as TroubleshootIcon,
    Key as KeyIcon,
} from '@/components/icons';
import { Button, Stack, Tooltip } from '@mui/material';
import { ProbeMenu } from '@/components/probe';

export interface TemplatePageActionsProps {
    collapsible: boolean;
    allExpanded: boolean;
    onToggleExpandAll: () => void;
    showAddApiKeyButton?: boolean;
    onAddApiKeyClick: () => void;
    allowAddRule: boolean;
    onCreateRule: () => void;
    showExpandCollapseButton?: boolean;
    onViewLogs?: () => void;
    // Icon button actions - these will be rendered next to the title instead
    onShowGuide?: () => void;
    // Probe V2 props
    scenario?: string;
}

export const TemplatePageActions: React.FC<TemplatePageActionsProps> = ({
    collapsible,
    allExpanded,
    onToggleExpandAll,
    showAddApiKeyButton = true,
    onAddApiKeyClick,
    allowAddRule,
    onCreateRule,
    showExpandCollapseButton = true,
    onViewLogs,
    onShowGuide,
    scenario,
}) => {
    const { t } = useTranslation();
    const [probeAnchorEl, setProbeAnchorEl] = useState<null | HTMLElement>(null);
    const probeMenuOpen = Boolean(probeAnchorEl);

    const handleProbeClick = (event: React.MouseEvent<HTMLElement>) => {
        event.stopPropagation();
        setProbeAnchorEl(event.currentTarget);
    };

    const handleProbeClose = () => {
        setProbeAnchorEl(null);
    };

    return (
        <Stack direction="row" spacing={1.5} alignItems="center">
            {onViewLogs && (
                <Button
                    variant="outlined"
                    startIcon={<TroubleshootIcon />}
                    onClick={onViewLogs}
                    size="small"
                >
                    {t('templateActions.troubleshoot')}
                </Button>
            )}
            {showAddApiKeyButton && (
                <Tooltip title={t('templateActions.connectAI')}>
                    <Button
                        variant="outlined"
                        startIcon={<KeyIcon/>}
                        onClick={onAddApiKeyClick}
                        size="small"
                    >
                        {t('templateActions.connectAI')}
                    </Button>
                </Tooltip>
            )}
            {allowAddRule && (
                <Tooltip title={t('templateActions.createNewRule')}>
                    <Button
                        variant="contained"
                        startIcon={<AddIcon/>}
                        onClick={onCreateRule}
                        size="small"
                    >
                        {t('templateActions.newRule')}
                    </Button>
                </Tooltip>
            )}
        </Stack>
    );
};

import React from 'react';
import { useTranslation } from 'react-i18next';
import {
    Add as AddIcon,
    Bolt as BoltIcon,
    BugReport as TroubleshootIcon,
    Key as KeyIcon,
} from '@/components/icons';
import { Button, Stack, Tooltip } from '@mui/material';

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
    /** Run the quick streaming probe on every active rule in the list. */
    onProbeAll?: () => void;
    // Icon button actions - these will be rendered next to the title instead
    onShowGuide?: () => void;
    // Probe props
    scenario?: string;
}

export const TemplatePageActions: React.FC<TemplatePageActionsProps> = ({
    showAddApiKeyButton = true,
    onAddApiKeyClick,
    allowAddRule,
    onCreateRule,
    onViewLogs,
    onProbeAll,
}) => {
    const { t } = useTranslation();

    return (
        <Stack direction="row" spacing={1.5} alignItems="center">
            {/* Diagnostics pair: "is it working now" (test all) + "why not" (logs) */}
            {onProbeAll && (
                <Tooltip title={t('probe.testAllHint')}>
                    <Button
                        variant="outlined"
                        startIcon={<BoltIcon />}
                        onClick={onProbeAll}
                        size="small"
                    >
                        {t('probe.testAll')}
                    </Button>
                </Tooltip>
            )}
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

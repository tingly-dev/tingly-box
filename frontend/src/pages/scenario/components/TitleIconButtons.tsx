import React from 'react';
import { useTranslation } from 'react-i18next';
import {
    FoldUp as FoldUpIcon,
    FoldDown as FoldDownIcon,
    HelpOutline as HelpOutlineIcon,
} from '@/components/icons';
import { IconButton, Stack, Tooltip } from '@mui/material';

export interface TitleIconButtonsProps {
    collapsible: boolean;
    allExpanded: boolean;
    onToggleExpandAll: () => void;
    showExpandCollapseButton?: boolean;
    onShowGuide?: () => void;
}

export const TitleIconButtons: React.FC<TitleIconButtonsProps> = ({
    collapsible,
    allExpanded,
    onToggleExpandAll,
    showExpandCollapseButton = true,
    onShowGuide,
}) => {
    const { t } = useTranslation();

    // Don't render if no icon buttons to show
    if (!showExpandCollapseButton || !collapsible) {
        if (!onShowGuide) return null;
    }

    return (
        <Stack direction="row" spacing={0.5} alignItems="center">
            {showExpandCollapseButton && collapsible && (
                <Tooltip title={allExpanded ? t('templateActions.collapseAllRules') : t('templateActions.expandAllRules')}>
                    <IconButton size="small" onClick={onToggleExpandAll}>
                        {allExpanded ? <FoldUpIcon fontSize="small" /> : <FoldDownIcon fontSize="small" />}
                    </IconButton>
                </Tooltip>
            )}
            {onShowGuide && (
                <Tooltip title={t('templateActions.howRoutingWorks', { defaultValue: 'How routing works' })}>
                    <IconButton
                        size="small"
                        aria-label={t('templateActions.howRoutingWorks', { defaultValue: 'How routing works' })}
                        onClick={onShowGuide}
                        sx={{ color: 'text.secondary', '&:hover': { color: 'primary.main' } }}
                    >
                        <HelpOutlineIcon fontSize="small" />
                    </IconButton>
                </Tooltip>
            )}
        </Stack>
    );
};

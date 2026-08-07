import React from 'react';
import { useTranslation } from 'react-i18next';
import {
    FoldUp as FoldUpIcon,
    FoldDown as FoldDownIcon,
    HelpOutline as HelpOutlineIcon,
    Sort as SortIcon,
    SortByAlpha as SortByAlphaIcon,
} from '@/components/icons';
import { IconButton, Stack, Tooltip } from '@mui/material';
import type { RuleSortMode } from '@/pages/scenario/hooks/useRuleSort';

export interface TitleIconButtonsProps {
    collapsible: boolean;
    allExpanded: boolean;
    onToggleExpandAll: () => void;
    showExpandCollapseButton?: boolean;
    onShowGuide?: () => void;
    // Rule-list display order — purely a view concern, doesn't touch the
    // rules' actual stored/routing order. Omit both to hide the control.
    sortMode?: RuleSortMode;
    onToggleSort?: () => void;
}

export const TitleIconButtons: React.FC<TitleIconButtonsProps> = ({
    collapsible,
    allExpanded,
    onToggleExpandAll,
    showExpandCollapseButton = true,
    onShowGuide,
    sortMode,
    onToggleSort,
}) => {
    const { t } = useTranslation();
    const showSort = !!onToggleSort && !!sortMode;

    // Don't render if no icon buttons to show
    if ((!showExpandCollapseButton || !collapsible) && !showSort) {
        if (!onShowGuide) return null;
    }

    return (
        <Stack direction="row" spacing={0.5} sx={{
            alignItems: "center"
        }}>
            {showSort && (
                <Tooltip title={sortMode === 'original' ? t('templateActions.sortTooltipToName') : t('templateActions.sortTooltipToOriginal')}>
                    <IconButton size="small" onClick={onToggleSort} aria-label={sortMode === 'original' ? t('templateActions.sortByName') : t('templateActions.sortOriginal')}>
                        {sortMode === 'original' ? <SortIcon fontSize="small" /> : <SortByAlphaIcon fontSize="small" color="primary" />}
                    </IconButton>
                </Tooltip>
            )}
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

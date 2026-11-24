import { Box, IconButton, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Info as InfoIcon } from '@/components/icons';

/**
 * Deliberate total-width cap for the header card's rows (Start/Base URL/API
 * Key/Plugins ConfigRows): on wide screens the card stays full-width but its
 * content stops at this value so the row actions don't drift far from the
 * row content — keeps every use page's header visually consistent.
 */
export const SCENARIO_HEADER_CONTENT_MAX_WIDTH = 960;

/**
 * UnifiedCard header title block shared by the agent pages and the pages
 * that keep their own structure (Team, Profile, Image API): the card title
 * plus an optional i18n-keyed info tooltip and an addon (a config status or
 * a small action such as "Duplicate as profile").
 */
export const ScenarioCardHeader: React.FC<{ title: string; tooltipKey?: string; addon?: React.ReactNode }> = ({ title, tooltipKey, addon }) => {
    const { t } = useTranslation();
    return (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <span>{title}</span>
            {tooltipKey && (
                <Tooltip title={t(tooltipKey)}>
                    <IconButton size="small" sx={{ ml: 0.5 }}>
                        <InfoIcon fontSize="small" sx={{ color: 'text.secondary' }} />
                    </IconButton>
                </Tooltip>
            )}
            {addon && <Box sx={{ ml: 0.5 }}>{addon}</Box>}
        </Box>
    );
};

/**
 * The `PageHeader` layout every Agent-shaped page shares (Agent pages, a Claude
 * Code profile, a Team, Image API): title, status and actions on one row where
 * they fit; below `breakpoint` the actions drop under the title instead of
 * wrapping it. The buttons never wrap among themselves (the primary one never
 * lands on a line of its own); the status text wraps under the title instead.
 * Pages with more buttons than an Agent page pass `lg`.
 */
export const agentPageHeaderSx = (breakpoint: 'md' | 'lg' = 'md') => ({
    pb: 0,
    borderBottom: 0,
    flexDirection: { xs: 'column', [breakpoint]: 'row' },
    alignItems: { xs: 'flex-start', [breakpoint]: 'center' },
    '& h1': { whiteSpace: 'nowrap' },
    '& > :last-child': { flexWrap: { [breakpoint]: 'nowrap' }, flexShrink: { [breakpoint]: 0 } },
}) as const;

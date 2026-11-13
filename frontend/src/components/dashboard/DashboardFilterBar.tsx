import { useTranslation } from 'react-i18next';
import {
    Box,
    Divider,
    FormControl,
    FormControlLabel,
    IconButton,
    InputLabel,
    ListSubheader,
    MenuItem,
    Select,
    Switch,
    Tooltip,
    Paper,
    Typography,
    CircularProgress,
} from '@mui/material';
import { Refresh as RefreshIcon, FilterOff, AccessTime as TimeIcon } from '@/components/icons';
import { switchControlLabelStyle } from '@/styles/toggleStyles';
import type { TimeRange } from '@/hooks/useDashboardData';
import { TIME_RANGE_CONFIG } from '@/hooks/useDashboardData';
import type { ProviderOptionGroup, ScenarioOption, UsageIdentity } from '@/hooks/useDashboardData';
import { scenarioLabel } from '@/pages/scenario/scenarioRegistry';
import { shortenUserId } from '@/hooks/useDashboardData';
import { useTeamContext } from '@/contexts/TeamContext';
import { groupByTeam } from '@/utils/team';
import { useMemo } from 'react';
import { fontMono, fontSizes } from '@/theme/fonts';
import { radius } from '@tingly/ui/theme/radius';

// Owner label is rendered through t() so a live language switch updates it;
// sharing-key labels carry their own display name instead.
const identityLabel = (t: (key: string, options?: Record<string, unknown>) => string, identity: UsageIdentity): string =>
    identity.type === 'owner'
        ? t('dashboard.overview.mainAccount', { defaultValue: 'Main account' })
        : identity.label;

// Shared by the provider and sharing-key pickers so both group the same way.
const GROUP_SUBHEADER_SX = {
    fontWeight: 600,
    fontSize: fontSizes.xs,
    textTransform: 'uppercase',
    letterSpacing: '0.05em',
    lineHeight: '28px',
    pt: 1,
    pl: 1.5,
    borderLeft: '3px solid',
    borderLeftColor: 'primary.main',
    backgroundColor: 'action.hover',
} as const;

// Container width below which the toolbar stacks into two rows, and below which
// the filters go 2x2. Measured: all controls plus the clear button fit in ~830px of content width.
const TOOLBAR_STACK_PX = 830;
const TOOLBAR_GRID2_PX = 519;

const TIME_RANGES = Object.keys(TIME_RANGE_CONFIG) as TimeRange[];

// Every select in the bar shares this size so the row stays compact.
const SELECT_SX = {
    borderRadius: radius.card,
    '& .MuiOutlinedInput-input': { py: 1 },
} as const;

/**
 * Toolbar of the usage dashboard: agent / provider / model / identity filter
 * selects and the clear-filters affordance on the left; time range, auto-refresh
 * and manual refresh on the right. Pure presentation — all state and callbacks
 * come from `useDashboardData` (and the route, for the time range) via the page.
 */
export default function DashboardFilterBar({
    providerGroups,
    modelOptions,
    usageIdentities,
    selectedProvider,
    onProviderChange,
    selectedModel,
    onModelChange,
    selectedUser,
    onUserChange,
    selectedIdentityLabel,
    scenarioOptions,
    selectedScenario,
    onScenarioChange,
    hasActiveFilters,
    onClearFilters,
    autoRefresh,
    onAutoRefreshChange,
    refreshing,
    onRefresh,
    timeRange,
    onTimeRangeChange,
}: {
    providerGroups: ProviderOptionGroup[];
    modelOptions: string[];
    usageIdentities: UsageIdentity[];
    selectedProvider: string;
    onProviderChange: (value: string) => void;
    selectedModel: string;
    onModelChange: (value: string) => void;
    selectedUser: string;
    onUserChange: (value: string) => void;
    selectedIdentityLabel: string;
    scenarioOptions: ScenarioOption[];
    selectedScenario: string;
    onScenarioChange: (value: string) => void;
    hasActiveFilters: boolean;
    onClearFilters: () => void;
    autoRefresh: boolean;
    onAutoRefreshChange: (value: boolean) => void;
    refreshing: boolean;
    onRefresh: () => void;
    timeRange: TimeRange;
    onTimeRangeChange: (value: TimeRange) => void;
}) {
    const { t } = useTranslation();
    const { teams } = useTeamContext();
    // Sharing keys grouped by Team; Teams without keys are skipped, and keys
    // whose Team isn't known (yet) stay selectable in a trailing group.
    const sharingKeyGroups = useMemo(() => {
        const { groups, unmatched } = groupByTeam(
            usageIdentities.filter((identity) => identity.type === 'sharing_key'),
            teams,
            (identity) => identity.teamId,
        );
        return [
            ...groups.filter((group) => group.items.length > 0),
            ...(unmatched.length > 0 ? [{ team: null, items: unmatched }] : []),
        ];
    }, [usageIdentities, teams]);

    // The selected agent stays selectable even when this range has no usage
    // for it (a link from an agent page can land on a quiet one).
    const agentOptions = useMemo(
        () => selectedScenario !== 'all' && !scenarioOptions.some((o) => o.scenario === selectedScenario)
            ? [...scenarioOptions, { scenario: selectedScenario, requests: 0 }]
            : scenarioOptions,
        [scenarioOptions, selectedScenario],
    );

    return (
        // The Paper is the size container, so the layout reacts to the toolbar's own
        // width (sidebar collapsed or not), not the viewport's.
        <Paper variant="outlined" sx={{ containerType: 'inline-size', px: 1.5, py: 1, borderRadius: radius.card }}>
            <Box
                sx={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    columnGap: 1.5,
                    // Too narrow for one row: range / auto / refresh first, filters below.
                    [`@container (max-width: ${TOOLBAR_STACK_PX}px)`]: {
                        flexDirection: 'column',
                        alignItems: 'stretch',
                        rowGap: 1.25,
                    },
                }}
            >
                {/* Filters: one row beside the range controls; when stacked, an even
                    4-up grid (2x2 when narrower) with the clear button in its own column. */}
                <Box
                    sx={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 1,
                        minWidth: 0,
                        // Plenty of room: let the selects show their full labels.
                        '@container (min-width: 1000px)': {
                            '& > .MuiFormControl-root': { width: 150 },
                            '& > .MuiFormControl-root:nth-of-type(4)': { width: 170 },
                        },
                        [`@container (max-width: ${TOOLBAR_STACK_PX}px)`]: {
                            display: 'grid',
                            gridTemplateColumns: 'repeat(4, minmax(0, 1fr)) auto',
                            '& > .MuiFormControl-root': { width: '100%' },
                            '& > .MuiDivider-root': { display: 'none' },
                        },
                        [`@container (max-width: ${TOOLBAR_GRID2_PX}px)`]: {
                            // The extra column exists only while the clear button does; otherwise
                            // the third filter would flow into it and the grid would read 3 + 1.
                            gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
                            '&:has(> .MuiIconButton-root)': { gridTemplateColumns: 'repeat(2, minmax(0, 1fr)) auto' },
                            '& > .MuiIconButton-root': { gridColumn: 3, gridRow: '1 / span 2' },
                        },
                    }}
                >
                    <FormControl size="small" sx={{ width: { xs: 140, md: 114 } }}>
                        <InputLabel sx={{ fontWeight: 500, fontSize: '0.875rem' }}>{t('dashboard.overview.agent', { defaultValue: 'Agent' })}</InputLabel>
                        <Select
                            value={selectedScenario}
                            label={t('dashboard.overview.agent', { defaultValue: 'Agent' })}
                            onChange={(e) => onScenarioChange(e.target.value)}
                            sx={SELECT_SX}
                        >
                            <MenuItem value="all">{t('dashboard.overview.allAgents', { defaultValue: 'All agents' })}</MenuItem>
                            {agentOptions.map((o) => (
                                <MenuItem key={o.scenario} value={o.scenario}>
                                    {scenarioLabel(t, o.scenario)}
                                </MenuItem>
                            ))}
                        </Select>
                    </FormControl>

                    <FormControl size="small" sx={{ width: { xs: 140, md: 124 } }}>
                        <InputLabel sx={{ fontWeight: 500, fontSize: '0.875rem' }}>{t('dashboard.overview.provider', { defaultValue: 'Provider' })}</InputLabel>
                        <Select
                            value={selectedProvider}
                            label={t('dashboard.overview.provider', { defaultValue: 'Provider' })}
                            onChange={(e) => onProviderChange(e.target.value)}
                            sx={SELECT_SX}
                        >
                            <MenuItem value="all">{t('dashboard.overview.allProviders', { defaultValue: 'All providers' })}</MenuItem>
                            {providerGroups.map((group) => [
                                <ListSubheader
                                    key={`header-${group.authType}`}
                                    sx={GROUP_SUBHEADER_SX}
                                >
                                    {group.label}
                                </ListSubheader>,
                                ...group.providers.map((p) => (
                                    <MenuItem key={p.uuid} value={p.uuid}>
                                        {p.name}
                                    </MenuItem>
                                )),
                            ])}
                        </Select>
                    </FormControl>

                    <FormControl size="small" sx={{ width: { xs: 140, md: 114 } }}>
                        <InputLabel sx={{ fontWeight: 500, fontSize: '0.875rem' }}>{t('dashboard.overview.model', { defaultValue: 'Model' })}</InputLabel>
                        <Select
                            value={selectedModel}
                            label={t('dashboard.overview.model', { defaultValue: 'Model' })}
                            onChange={(e) => onModelChange(e.target.value)}
                            sx={SELECT_SX}
                        >
                            <MenuItem value="all">{t('dashboard.overview.allModels', { defaultValue: 'All models' })}</MenuItem>
                            {modelOptions.map((model) => (
                                <MenuItem key={model} value={model}>
                                    {model}
                                </MenuItem>
                            ))}
                        </Select>
                    </FormControl>

                    <FormControl size="small" sx={{ width: { xs: 160, md: 130 } }}>
                        <InputLabel sx={{ fontWeight: 500, fontSize: '0.875rem' }}>{t('dashboard.overview.identity', { defaultValue: 'Identity' })}</InputLabel>
                        <Select
                            value={selectedUser}
                            label={t('dashboard.overview.identity', { defaultValue: 'Identity' })}
                            onChange={(e) => onUserChange(e.target.value)}
                            renderValue={() => selectedIdentityLabel}
                            sx={SELECT_SX}
                        >
                            <MenuItem value="all">{t('dashboard.overview.allIdentities', { defaultValue: 'All identities' })}</MenuItem>
                            {usageIdentities.filter((identity) => identity.type === 'owner').map((identity) => (
                                <MenuItem key={identity.userId} value={identity.userId}>
                                    {identity.label}
                                </MenuItem>
                            ))}
                            {sharingKeyGroups.map((group) => [
                                <ListSubheader key={`keys-${group.team?.id ?? 'other'}`} sx={GROUP_SUBHEADER_SX}>
                                    {group.team
                                        ? t('dashboard.overview.sharingKeysForTeam', { team: group.team.name })
                                        : t('dashboard.overview.sharingKeys', { defaultValue: 'Team Keys' })}
                                </ListSubheader>,
                                ...group.items.map((identity) => (
                                    <MenuItem key={identity.userId} value={identity.userId}>
                                        <Box sx={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 2, width: '100%' }}>
                                            <Typography variant="body2" noWrap>
                                                {identityLabel(t, identity)}{!identity.enabled ? t('dashboard.overview.disabledSuffix', { defaultValue: ' (disabled)' }) : ''}
                                            </Typography>
                                            <Tooltip title={identity.userId} placement="right">
                                                <Typography
                                                    variant="caption"
                                                    sx={{
                                                        color: "text.secondary",
                                                        fontFamily: fontMono,
                                                        flexShrink: 0
                                                    }}>
                                                    {shortenUserId(identity.userId)}
                                                </Typography>
                                            </Tooltip>
                                        </Box>
                                    </MenuItem>
                                )),
                            ])}
                        </Select>
                    </FormControl>

                    {hasActiveFilters && (
                        <>
                            <Divider orientation="vertical" flexItem sx={{ mx: 0.5 }} />
                            <Tooltip title={t('dashboard.overview.clearFilters', { defaultValue: 'Clear all filters' })}>
                                <IconButton
                                    size="small"
                                    onClick={onClearFilters}
                                    sx={{
                                        backgroundColor: 'action.hover',
                                        '&:hover': { backgroundColor: 'action.selected' },
                                    }}
                                >
                                    <FilterOff />
                                </IconButton>
                            </Tooltip>
                        </>
                    )}
                </Box>

                {/* Range + refresh: right-aligned; first row when stacked, with the range stretching. */}
                <Box
                    sx={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 1,
                        ml: 'auto',
                        [`@container (max-width: ${TOOLBAR_STACK_PX}px)`]: { ml: 0, order: -1 },
                    }}
                >
                    <FormControl
                        size="small"
                        sx={{
                            minWidth: 120,
                            [`@container (max-width: ${TOOLBAR_STACK_PX}px)`]: { flex: 1 },
                        }}
                    >
                        <Select
                            value={timeRange}
                            onChange={(e) => onTimeRangeChange(e.target.value as TimeRange)}
                            inputProps={{ 'aria-label': t('dashboard.overview.timeRange') }}
                            startAdornment={<TimeIcon fontSize="small" sx={{ mr: 0.75, color: 'text.secondary' }} />}
                            sx={SELECT_SX}
                        >
                            {TIME_RANGES.map((range) => (
                                <MenuItem key={range} value={range}>{t(TIME_RANGE_CONFIG[range].labelKey)}</MenuItem>
                            ))}
                        </Select>
                    </FormControl>
                    <Divider orientation="vertical" flexItem sx={{ mx: 0.5, my: 0.5 }} />
                    <FormControlLabel
                        control={
                            <Switch
                                size="small"
                                checked={autoRefresh}
                                onChange={(e) => onAutoRefreshChange(e.target.checked)}
                                color="primary"
                            />
                        }
                        label={<Typography variant="body2">{t('dashboard.overview.auto', { defaultValue: 'Auto' })}</Typography>}
                        sx={switchControlLabelStyle}
                    />
                    <Tooltip title={t('dashboard.overview.refreshData', { defaultValue: 'Refresh data' })}>
                        <IconButton
                            size="small"
                            onClick={onRefresh}
                            disabled={refreshing}
                            sx={{
                                backgroundColor: 'action.hover',
                                '&:hover': { backgroundColor: 'action.selected' },
                                '&:disabled': { backgroundColor: 'transparent' },
                            }}
                        >
                            {refreshing ? <CircularProgress size={18} /> : <RefreshIcon />}
                        </IconButton>
                    </Tooltip>
                </Box>
            </Box>
        </Paper>
    );
}

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
    Typography,
    CircularProgress,
} from '@mui/material';
import { Refresh as RefreshIcon, FilterOff } from '@/components/icons';
import { switchControlLabelStyle } from '@/styles/toggleStyles';
import type { ProviderOptionGroup, UsageIdentity } from '@/hooks/useDashboardData';
import { shortenUserId } from '@/hooks/useDashboardData';
import { useTeamContext } from '@/contexts/TeamContext';
import { groupSharingKeysByTeam } from './groupSharingKeysByTeam';

// Owner label is rendered through t() so a live language switch updates it;
// sharing-key labels carry their own display name instead.
const identityLabel = (t: (key: string, options?: Record<string, unknown>) => string, identity: UsageIdentity): string =>
    identity.type === 'owner'
        ? t('dashboard.overview.mainAccount', { defaultValue: 'Main account' })
        : identity.label;

// Shared by the provider and sharing-key pickers so both group the same way.
const GROUP_SUBHEADER_SX = {
    fontWeight: 600,
    fontSize: '0.7rem',
    textTransform: 'uppercase',
    letterSpacing: '0.05em',
    lineHeight: '28px',
    pt: 1,
    pl: 1.5,
    borderLeft: '3px solid',
    borderLeftColor: 'primary.main',
    backgroundColor: 'action.hover',
} as const;

/**
 * Header action bar of the usage dashboard: provider / model / identity
 * filter selects, the clear-filters affordance, and the auto-refresh +
 * manual-refresh controls. Pure presentation — all state and callbacks come
 * from `useDashboardData` via the page.
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
    hasActiveFilters,
    onClearFilters,
    autoRefresh,
    onAutoRefreshChange,
    refreshing,
    onRefresh,
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
    hasActiveFilters: boolean;
    onClearFilters: () => void;
    autoRefresh: boolean;
    onAutoRefreshChange: (value: boolean) => void;
    refreshing: boolean;
    onRefresh: () => void;
}) {
    const { t } = useTranslation();
    const { teams } = useTeamContext();
    const sharingKeyGroups = groupSharingKeysByTeam(
        usageIdentities.filter((identity) => identity.type === 'sharing_key'),
        teams,
    );

    return (
        <>
            <FormControl size="small" sx={{ minWidth: { xs: 140, sm: 160 } }}>
                <InputLabel sx={{ fontWeight: 500, fontSize: '0.875rem' }}>{t('dashboard.overview.provider', { defaultValue: 'Provider' })}</InputLabel>
                <Select
                    value={selectedProvider}
                    label={t('dashboard.overview.provider', { defaultValue: 'Provider' })}
                    onChange={(e) => onProviderChange(e.target.value)}
                    sx={{
                        borderRadius: 2,
                        '& .MuiOutlinedInput-input': { py: 1 },
                    }}
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

            <FormControl size="small" sx={{ minWidth: { xs: 140, sm: 160 } }}>
                <InputLabel sx={{ fontWeight: 500, fontSize: '0.875rem' }}>{t('dashboard.overview.model', { defaultValue: 'Model' })}</InputLabel>
                <Select
                    value={selectedModel}
                    label={t('dashboard.overview.model', { defaultValue: 'Model' })}
                    onChange={(e) => onModelChange(e.target.value)}
                    sx={{
                        borderRadius: 2,
                        '& .MuiOutlinedInput-input': { py: 1 },
                    }}
                >
                    <MenuItem value="all">{t('dashboard.overview.allModels', { defaultValue: 'All models' })}</MenuItem>
                    {modelOptions.map((model) => (
                        <MenuItem key={model} value={model}>
                            {model}
                        </MenuItem>
                    ))}
                </Select>
            </FormControl>

            <FormControl size="small" sx={{ minWidth: { xs: 160, sm: 200 } }}>
                <InputLabel sx={{ fontWeight: 500, fontSize: '0.875rem' }}>{t('dashboard.overview.identity', { defaultValue: 'Identity' })}</InputLabel>
                <Select
                    value={selectedUser}
                    label={t('dashboard.overview.identity', { defaultValue: 'Identity' })}
                    onChange={(e) => onUserChange(e.target.value)}
                    renderValue={() => selectedIdentityLabel}
                    sx={{
                        borderRadius: 2,
                        '& .MuiOutlinedInput-input': { py: 1 },
                    }}
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
                                : t('dashboard.overview.sharingKeys', { defaultValue: 'Sharing Keys' })}
                        </ListSubheader>,
                        ...group.identities.map((identity) => (
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
                                                fontFamily: 'monospace',
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
                    <Divider orientation="vertical" flexItem sx={{ mx: 0.5, display: { xs: 'none', sm: 'block' } }} />
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

            <Divider orientation="vertical" flexItem sx={{ mx: 0.5, display: { xs: 'none', sm: 'block' } }} />

            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
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
        </>
    );
}

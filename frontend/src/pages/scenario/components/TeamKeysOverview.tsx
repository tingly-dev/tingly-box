import { Box, ButtonBase, Card, Chip, CircularProgress, Collapse, IconButton, Stack, Switch, Tooltip, Typography } from '@mui/material';
import {
    CompareArrows as IconMove,
    ContentCopy as IconCopy,
    DeleteOutline as IconDeleteOutline,
    ExpandMore as ExpandMoreIcon,
    Visibility as IconEye,
    VisibilityOff as IconEyeOff,
} from '@/components/icons';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import ConfigRow from '@/components/ConfigRow';
import CreateSharingKeyButton from '@/components/CreateSharingKeyButton';
import type { SharingKey } from '@/components/SharingKeysTable';
import type { SharingKeyTableActions } from '@/hooks/useSharingKeyActions';
import type { Team } from '@/types/team';
import { fontMono, fontSizes } from '@/theme/fonts';
import { maskToken } from '@/utils/sharingKey';
import { SCENARIO_HEADER_CONTENT_MAX_WIDTH } from './ScenarioCardHeader';

const PREF_KEY = 'team-keys-overview-expanded';

// Same right-edge room for the fold chevron as the connection card below
// (AgentAccessSection), so the two cards' rows and chevrons line up.
const CHEVRON_GUTTER = 6;

/**
 * Whether the Team page's key card is open. Folded by default; once the
 * user opens or closes it, that choice is remembered on this machine for
 * every Team (it is a way of using the Team page, not a per-Team setting).
 */
export function useTeamKeysOverviewExpanded() {
    const [expanded, setExpanded] = useState<boolean>(() => {
        try {
            return localStorage.getItem(PREF_KEY) === 'true';
        } catch {
            return false; // storage unavailable: just follow the default
        }
    });
    const toggle = useCallback(() => {
        setExpanded((prev) => {
            const next = !prev;
            try {
                localStorage.setItem(PREF_KEY, String(next));
            } catch { /* best-effort */ }
            return next;
        });
    }, []);
    return { expanded, toggle };
}

export interface TeamKeysOverviewProps {
    team: Team;
    keys: SharingKey[];
    loading: boolean;
    expanded: boolean;
    onToggle: () => void;
    /** From `useSharingKeyActions` — reveal / copy / enable / move / delete. */
    actions: SharingKeyTableActions;
    onCreate: () => void;
}

const formatDate = (dateStr?: string) => (dateStr ? new Date(dateStr).toLocaleString() : '-');

/**
 * The Team's keys, under the page header — where they are handed out and
 * managed. Each key is one person's credential (its own user id), so a row
 * is a person: name, key (masked, reveal / copy), last use, and its
 * enable / move / delete.
 *
 * Built like the connection card below it: folded, one ConfigRow line
 * ("Team Keys   5 keys") that is itself the fold button; open, that line
 * gains Create and the keys follow on the same label / value / trailing grid.
 */
export const TeamKeysOverview: React.FC<TeamKeysOverviewProps> = ({
    team, keys, loading, expanded, onToggle, actions, onCreate,
}) => {
    const { t } = useTranslation();
    const label = t(expanded ? 'sharingKeys.overview.collapse' : 'sharingKeys.overview.expand');
    const iconButtonSx = { p: 0.25, color: 'text.disabled', '&:hover': { color: 'text.primary' } };

    // Folded and open use two different toggle buttons (open, the line holds
    // Create, so only the chevron folds it): hand keyboard focus across.
    const toggleEl = useRef<HTMLElement | null>(null);
    const toggleRef = useCallback((el: HTMLElement | null) => { toggleEl.current = el; }, []);
    const refocus = useRef(false);
    useEffect(() => {
        if (refocus.current) {
            refocus.current = false;
            toggleEl.current?.focus();
        }
    }, [expanded]);
    const handleToggle = () => {
        refocus.current = true;
        onToggle();
    };

    const chevron = (
        <ExpandMoreIcon
            sx={{
                transition: 'transform 0.2s',
                transform: expanded ? 'rotate(180deg)' : 'rotate(0deg)',
                fontSize: '1.1rem',
                color: 'text.secondary',
                flexShrink: 0,
            }}
        />
    );

    const summaryRow = (
        <Box sx={{ pt: 1, pb: 1, pr: CHEVRON_GUTTER }}>
            <Box sx={{ maxWidth: SCENARIO_HEADER_CONTENT_MAX_WIDTH }}>
                <Box sx={{ px: 2, py: 0.5 }}>
                    <ConfigRow
                        tabs={[{
                            key: 'keys',
                            label: t('sharingKeys.title'),
                            content: loading ? (
                                <Box sx={{ py: 1, display: 'flex' }}><CircularProgress size={16} /></Box>
                            ) : (
                                <Typography data-testid="team-keys-count" variant="subtitle2" sx={{ py: 1 }}>
                                    {t('sharingKeys.overview.count', { count: keys.length })}
                                </Typography>
                            ),
                            actions: expanded
                                ? <CreateSharingKeyButton team={team} variant="outlined" size="small" onClick={onCreate} />
                                : undefined,
                        }]}
                        activeTab="keys"
                        onTabChange={() => {}}
                    />
                </Box>
            </Box>
        </Box>
    );

    return (
        <Card
            sx={{
                position: 'relative',
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
                boxShadow: 'none',
                backgroundColor: 'background.paper',
                '& .team-keys-toggle:hover svg': { color: 'text.primary' },
            }}
        >
            {expanded ? (
                <>
                    {summaryRow}
                    <Tooltip title={label}>
                        <ButtonBase
                            ref={toggleRef}
                            className="team-keys-toggle"
                            onClick={handleToggle}
                            aria-expanded
                            aria-label={label}
                            sx={{ position: 'absolute', top: 12, right: 22, zIndex: 1, p: 0.25, borderRadius: 1 }}
                        >
                            {chevron}
                        </ButtonBase>
                    </Tooltip>
                </>
            ) : (
                // Folded, the whole line is the button; its name is what it
                // shows, the hover title says what clicking does.
                <ButtonBase
                    ref={toggleRef}
                    component="div"
                    className="team-keys-toggle"
                    onClick={handleToggle}
                    aria-expanded={false}
                    title={label}
                    sx={{ display: 'block', width: '100%', textAlign: 'left', '&:hover': { backgroundColor: 'action.hover' } }}
                >
                    {summaryRow}
                    <Box sx={{ position: 'absolute', top: 12, right: 22, p: 0.25, display: 'flex' }}>{chevron}</Box>
                </ButtonBase>
            )}
            <Collapse in={expanded} timeout={0} unmountOnExit>
                <Box sx={{ pb: 1, pr: CHEVRON_GUTTER }}>
                    <Box sx={{ maxWidth: SCENARIO_HEADER_CONTENT_MAX_WIDTH, px: 2 }}>
                        {!loading && keys.length === 0 ? (
                            <Typography variant="body2" sx={{ color: 'text.secondary', px: 1.5, pb: 0.5 }}>
                                {t('sharingKeys.overview.empty')}
                            </Typography>
                        ) : (
                            // Capped so a Team with many keys scrolls inside the card
                            // instead of pushing the rules below off the page.
                            <Box
                                component="ul"
                                aria-label={t('sharingKeys.title')}
                                sx={{ listStyle: 'none', m: 0, p: 0, maxHeight: 360, overflowY: 'auto' }}
                            >
                                {keys.map((key) => {
                                    const visible = !!actions.visibleTokens[key.token_id];
                                    return (
                                        <Box
                                            component="li"
                                            key={key.token_id}
                                            sx={{
                                                // ConfigRow's grid, so names sit under "Team Keys"
                                                // and keys under the count, as in the card below.
                                                display: 'grid',
                                                alignItems: 'center',
                                                columnGap: { xs: 1, sm: 3 },
                                                gridTemplateColumns: { xs: 'minmax(0, 1fr) auto', sm: '168px minmax(0, 1fr) auto' },
                                                py: 0.25,
                                                borderRadius: 1,
                                                '&:hover': { backgroundColor: 'action.hover' },
                                            }}
                                        >
                                            <Stack direction="row" spacing={1} sx={{ alignItems: 'center', minWidth: 0, px: 1.5, gridColumn: { xs: '1 / -1', sm: '1' }, opacity: key.enabled ? 1 : 0.55 }}>
                                                <Typography noWrap title={key.display_name} sx={{ fontSize: fontSizes.md, fontWeight: 500, minWidth: 0 }}>
                                                    {key.display_name}
                                                </Typography>
                                                {!key.enabled && (
                                                    <Chip label={t('sharingKeys.table.inactive')} size="small" variant="outlined" sx={{ height: 18, fontSize: fontSizes.xs }} />
                                                )}
                                            </Stack>
                                            <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center', minWidth: 0, opacity: key.enabled ? 1 : 0.55 }}>
                                                <Typography
                                                    variant="subtitle2"
                                                    sx={{ fontFamily: fontMono, color: 'text.secondary', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
                                                >
                                                    {visible ? key.token_id : maskToken(key.token_id)}
                                                </Typography>
                                                <Tooltip title={visible ? t('sharingKeys.table.hide') : t('sharingKeys.table.show')}>
                                                    <IconButton size="small" sx={iconButtonSx} onClick={() => actions.onToggleVisibility(key.token_id)}>
                                                        {visible ? <IconEyeOff sx={{ fontSize: 14 }} /> : <IconEye sx={{ fontSize: 14 }} />}
                                                    </IconButton>
                                                </Tooltip>
                                                <Tooltip title={t('sharingKeys.table.copy')}>
                                                    <IconButton size="small" sx={iconButtonSx} onClick={() => actions.onCopy(key.token_id)}>
                                                        <IconCopy sx={{ fontSize: 14 }} />
                                                    </IconButton>
                                                </Tooltip>
                                            </Stack>
                                            <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center', justifySelf: 'end' }}>
                                                <Typography
                                                    variant="caption"
                                                    noWrap
                                                    title={`${t('sharingKeys.table.created')}: ${formatDate(key.created_at)}`}
                                                    sx={{ color: 'text.secondary', mr: 0.5, display: { xs: 'none', md: 'block' } }}
                                                >
                                                    {t('sharingKeys.table.lastUsed')}: {formatDate(key.last_used_at)}
                                                </Typography>
                                                <Tooltip title={key.enabled ? t('sharingKeys.table.active') : t('sharingKeys.table.inactive')}>
                                                    <Switch
                                                        size="small"
                                                        color="success"
                                                        checked={key.enabled}
                                                        onChange={() => actions.onToggleEnabled(key)}
                                                        slotProps={{ input: { 'aria-label': `${key.display_name} ${t('sharingKeys.table.status')}` } }}
                                                    />
                                                </Tooltip>
                                                <Tooltip title={t('sharingKeys.table.moveTooltip')}>
                                                    <IconButton size="small" aria-label={t('sharingKeys.table.moveTooltip')} onClick={() => actions.onMove(key)}>
                                                        <IconMove sx={{ fontSize: 16 }} />
                                                    </IconButton>
                                                </Tooltip>
                                                <Tooltip title={t('sharingKeys.table.deleteTooltip')}>
                                                    <IconButton
                                                        size="small"
                                                        color="error"
                                                        aria-label={t('sharingKeys.table.deleteTooltip')}
                                                        onClick={() => actions.onDelete(key)}
                                                        sx={{ opacity: 0.75, '&:hover': { opacity: 1 } }}
                                                    >
                                                        <IconDeleteOutline sx={{ fontSize: 16 }} />
                                                    </IconButton>
                                                </Tooltip>
                                            </Stack>
                                        </Box>
                                    );
                                })}
                            </Box>
                        )}
                    </Box>
                </Box>
            </Collapse>
        </Card>
    );
};

export default TeamKeysOverview;

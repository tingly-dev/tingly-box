import { Alert, Box, Button, Chip, Divider, Stack, Typography } from '@mui/material';
import { Fragment, useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import CreateSharingKeyButton from '@/components/CreateSharingKeyButton';
import PageHeader from '@/components/PageHeader';
import { PageLayout } from '@/components/PageLayout';
import SharingKeysTable, { type SharingKey } from '@/components/SharingKeysTable';
import Surface from '@/components/Surface';
import { useTeamContext } from '@/contexts/TeamContext';
import { useNotify } from '@/hooks/useNotify';
import { useSharingKeyActions } from '@/hooks/useSharingKeyActions';
import { api } from '@/services/api';
import type { Team } from '@/types/team';

const teamPath = (team: Team) => (team.is_default ? '/agent/team' : `/agent/team/${team.slug}`);

/**
 * Every Sharing Key on this instance, grouped by the Team it belongs to.
 * Keys are Team-scoped credentials (.design/team.md), so this is an overview
 * of the per-Team lists rather than a Team-less global list: each section
 * creates keys into its own Team, and moving a key is the only way across.
 */
const TeamKeysPage = () => {
    const { t } = useTranslation();
    const notify = useNotify();
    const { teams, loading: teamsLoading } = useTeamContext();
    const [keys, setKeys] = useState<SharingKey[]>([]);
    const [keysLoading, setKeysLoading] = useState(true);

    const loadKeys = useCallback(async () => {
        const result = await api.listAPITokens({ limit: 500 });
        if (result.success && result.data) {
            setKeys(result.data.tokens || []);
        } else {
            notify.error(result.error?.message || t('sharingKeys.loadFailed'));
        }
        setKeysLoading(false);
    }, [notify, t]);

    useEffect(() => { void loadKeys(); }, [loadKeys]);

    const { tableProps, openCreate, dialogs } = useSharingKeyActions({ teams, onChanged: loadKeys });

    // Same order as the Team sidebar: default Team first, then the rest.
    const groups = useMemo(() => {
        const ordered = [...teams.filter((team) => team.is_default), ...teams.filter((team) => !team.is_default)];
        const defaultTeamId = ordered.find((team) => team.is_default)?.id;
        return ordered.map((team) => ({
            team,
            keys: keys.filter((key) => (key.team_id || defaultTeamId) === team.id),
        }));
    }, [teams, keys]);

    return (
        <PageLayout loading={teamsLoading || keysLoading}>
            <Stack spacing={2.5}>
                <PageHeader
                    title={t('sharingKeys.allTitle')}
                    subtitle={t('sharingKeys.allSubtitle', { keys: keys.length, teams: teams.length })}
                />

                <Alert
                    severity="info"
                    variant="outlined"
                    sx={{ py: 0.25, alignItems: 'center', '& .MuiAlert-message': { py: 0.5 } }}
                >
                    <Typography variant="body2">{t('teams.allKeysScopeSummary')}</Typography>
                </Alert>

                <Surface padding={{ xs: 2, sm: 2.5 }}>
                    <Stack spacing={3}>
                        {groups.map(({ team, keys: teamKeys }, index) => (
                            <Fragment key={team.id}>
                                {index > 0 && <Divider />}
                                <Box>
                                    <Stack
                                        direction="row"
                                        spacing={1}
                                        useFlexGap
                                        sx={{ alignItems: 'center', flexWrap: 'wrap', mb: 1.5 }}
                                    >
                                        <Typography variant="subtitle1" sx={{ fontWeight: 500 }}>
                                            {team.name}
                                        </Typography>
                                        <Typography variant="caption" sx={{ fontFamily: 'monospace', color: 'text.secondary' }}>
                                            {team.slug}
                                        </Typography>
                                        <Chip label={teamKeys.length} size="small" color="primary" variant="outlined" sx={{ height: 20, minWidth: 20, fontSize: '0.7rem' }} />
                                        {!team.enabled && (
                                            <Chip label={t('teams.inactive')} size="small" color="warning" variant="outlined" sx={{ height: 20, fontSize: '0.7rem' }} />
                                        )}
                                        <Box sx={{ flex: 1 }} />
                                        <Button component={Link} to={teamPath(team)} size="small">
                                            {t('sharingKeys.openTeam')}
                                        </Button>
                                        <CreateSharingKeyButton team={team} variant="outlined" size="small" onClick={() => openCreate(team)} />
                                    </Stack>
                                    {!team.enabled && teamKeys.length > 0 && (
                                        <Typography variant="body2" sx={{ color: 'warning.main', mb: 1 }}>
                                            {t('teams.disabledHint')}
                                        </Typography>
                                    )}
                                    {teamKeys.length > 0 ? (
                                        <SharingKeysTable tokens={teamKeys} {...tableProps} />
                                    ) : (
                                        // One quiet line instead of a full empty-state card per
                                        // Team: the Create button is already on the header row.
                                        <Typography variant="body2" sx={{ color: 'text.secondary', py: 1 }}>
                                            {t('sharingKeys.emptyTeam')}
                                        </Typography>
                                    )}
                                </Box>
                            </Fragment>
                        ))}
                    </Stack>
                </Surface>
            </Stack>
            {dialogs}
        </PageLayout>
    );
};

export default TeamKeysPage;

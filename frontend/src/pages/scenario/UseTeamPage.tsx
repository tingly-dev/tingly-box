import CardGrid from '@/components/CardGrid.tsx';
import ProviderConfigCard from '@/components/ProviderConfigCard.tsx';
import ConfirmDialog from '@/components/ConfirmDialog.tsx';
import {
    Alert, Box, Button, CircularProgress, Dialog, DialogActions,
    DialogContent, DialogTitle, FormControlLabel, IconButton, Stack, Switch,
    TextField, Tooltip,
} from '@mui/material';
import {Delete, Duplicate, Edit, HelpOutline, Info as IconInfo} from '@/components/icons';
import DuplicateDialog from '@/components/DuplicateDialog';
import {suggestTeamCopyName} from '@/utils/copyName';
import {useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {useNavigate, useParams} from 'react-router-dom';
import PageLayout from '@/components/PageLayout';
import PageHeader from '@/components/PageHeader';
import ScenarioLogDialog from '@/components/RuleLogDialog';
import AgentAccessSection, {useAccessExpanded} from './components/AgentAccessSection';
import AgentStatusRow, {AgentLogButton} from './components/AgentStatusRow';
import {useAgentObservation} from '@/pages/scenario/hooks/useAgentObservation';
import ScenarioPageSkeleton from './components/ScenarioPageSkeleton';
import { SCENARIO_HEADER_CONTENT_MAX_WIDTH, agentPageHeaderSx } from './components/ScenarioCardHeader';
import { usePluginsActive } from '@/hooks/usePluginsActive';
import PluginFeatures from '@/components/PluginFeatures';
import TemplatePage from './components/TemplatePage.tsx';
import TeamKeysOverview, {useTeamKeysOverviewExpanded} from './components/TeamKeysOverview';
import {useSharingKeyActions} from '@/hooks/useSharingKeyActions';
import {useTeamSharingKeys} from '@/hooks/useTeamSharingKeys';
import TeamGuideDialog from '@/components/TeamGuideDialog';
import {useScenarioPageInternal} from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import {ScenarioPageModalProvider} from '@/pages/scenario/context/ScenarioPageContext';
import {api} from '@/services/api';
import {useNotify} from '@/hooks/useNotify';
import {useTeamContext} from '@/contexts/TeamContext';
import { fontSizes } from '@/theme/fonts';

const UseTeamPageContent: React.FC = () => {
    const {t} = useTranslation();
    const notify = useNotify();
    const navigate = useNavigate();
    const {teamSlug} = useParams<{teamSlug: string}>();
    const {teams, loading: teamsLoading, refresh} = useTeamContext();
    const currentTeam = teamSlug
        ? teams.find(team => team.slug === teamSlug)
        : teams.find(team => team.is_default);
    const scenario = currentTeam && !currentTeam.is_default ? `team:${currentTeam.id}` : 'team';
    const {isLoading, notification, copyToClipboard, baseUrl} = useScenarioPageInternal(scenario);

    const observation = useAgentObservation(scenario);
    const access = useAccessExpanded(scenario, !observation.data?.lastRequestAt);
    // Whether any plugin is on, independent of the connection card's fold
    // state, so the folded card can surface the row only when it matters.
    const pluginsActive = usePluginsActive(scenario);
    const pluginsRow = <PluginFeatures scenario={scenario} />;
    const [logOpen, setLogOpen] = useState(false);
    // The Team's keys and the actions on them (create / enable / move / delete),
    // all worked from the key card under the header.
    const teamKeys = useTeamSharingKeys(currentTeam?.id);
    const keyActions = useSharingKeyActions({teams, onChanged: teamKeys.reload});
    const keysOverview = useTeamKeysOverviewExpanded();
    const [teamGuideOpen, setTeamGuideOpen] = useState(false);
    const [editorOpen, setEditorOpen] = useState(false);
    const [deleteOpen, setDeleteOpen] = useState(false);
    const [teamName, setTeamName] = useState('');
    const [saving, setSaving] = useState(false);
    const [toggling, setToggling] = useState(false);

    useEffect(() => {
        if (!teamsLoading && teamSlug && !currentTeam) navigate('/agent/team', {replace: true});
    }, [teamsLoading, teamSlug, currentTeam, navigate]);

    const openEditor = () => {
        if (!currentTeam) return;
        setTeamName(currentTeam.name);
        setEditorOpen(true);
    };

    // saveTeam/toggleTeam/deleteTeam share the same shape: flip a busy flag,
    // call the API, surface an error and bail, or fall through on success.
    const runTeamAction = async (
        setBusy: (busy: boolean) => void,
        apiCall: () => Promise<{success: boolean; error?: {message?: string}}>,
        failMessage: string,
    ): Promise<boolean> => {
        setBusy(true);
        const result = await apiCall();
        setBusy(false);
        if (!result.success) {
            notify.error(result.error?.message || failMessage);
            return false;
        }
        return true;
    };

    const saveTeam = async () => {
        if (!currentTeam || !teamName.trim()) return;
        const ok = await runTeamAction(
            setSaving,
            () => api.updateTeam(currentTeam.id, {name: teamName.trim()}),
            t('teams.saveFailed'),
        );
        if (!ok) return;
        setEditorOpen(false);
        await refresh();
        notify.success(t('teams.updateSuccess'));
    };

    const toggleTeam = async () => {
        if (!currentTeam) return;
        const wasEnabled = currentTeam.enabled;
        const ok = await runTeamAction(
            setToggling,
            () => api.setTeamEnabled(currentTeam.id, !wasEnabled),
            t('teams.saveFailed'),
        );
        if (!ok) return;
        await refresh();
        notify.success(wasEnabled ? t('teams.disabled') : t('teams.enabled'));
    };

    const [duplicateOpen, setDuplicateOpen] = useState(false);
    // Copies the Team's routing into a new Team (its keys stay behind) and lands on it;
    // the dialog stays open on failure so the name can be fixed.
    const duplicateTeam = async (name: string): Promise<boolean> => {
        if (!currentTeam) return false;
        const result = await api.duplicateTeam(currentTeam.id, name);
        if (!result.success) {
            notify.error(result.error?.message || t('teams.duplicateFailed'));
            return false;
        }
        await refresh();
        navigate(`/agent/team/${result.data.slug}`);
        notify.success(t('teams.duplicated'));
        return true;
    };

    const deleteTeam = async () => {
        if (!currentTeam) return;
        const ok = await runTeamAction(
            setSaving,
            () => api.deleteTeam(currentTeam.id),
            t('teams.deleteFailed'),
        );
        if (!ok) return;
        setDeleteOpen(false);
        setEditorOpen(false);
        await refresh();
        navigate('/agent/team', {replace: true});
        notify.success(t('teams.deleteSuccess'));
    };

    return (
        <PageLayout loading={teamsLoading || isLoading || !currentTeam}
                    loadingContent={<ScenarioPageSkeleton />} notification={notification}>
            {currentTeam && (
                <Stack spacing={2}>
                    <PageHeader
                        // Same shape as the Agent pages: the name carries its edit / delete,
                        // the switch and looks-in sit on the right; status text wraps under
                        // the title. The Team's keys are managed in the card right below.
                        title={(
                            <Box sx={{display: 'flex', alignItems: 'center', gap: 1, minWidth: 0}}>
                                <Box component="span" sx={{minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis'}}>
                                    {t('layout.nav.useTeam', {defaultValue: 'Team'})} - {currentTeam.name}
                                </Box>
                                <Tooltip
                                    title={t('teams.keyScopeSummary', {
                                        team: currentTeam.name,
                                        slug: currentTeam.slug,
                                    })}
                                    arrow
                                    placement="bottom"
                                    slotProps={{tooltip: {sx: {maxWidth: 380}}}}
                                >
                                    <IconButton
                                        size="small"
                                        aria-label={t('teams.keyScopeInfoLabel')}
                                        sx={{color: 'text.secondary'}}
                                    >
                                        <IconInfo fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                                <Tooltip title={t('teams.guide.trigger')}>
                                    <IconButton
                                        size="small"
                                        aria-label={t('teams.guide.trigger')}
                                        onClick={() => setTeamGuideOpen(true)}
                                        sx={{color: 'text.secondary', '&:hover': {color: 'primary.main'}}}
                                    >
                                        <HelpOutline fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                                <Tooltip title={t('teams.editTeam')}>
                                    <IconButton size="small" aria-label={t('teams.editTeam')} onClick={openEditor}>
                                        <Edit fontSize="small" sx={{color: 'text.secondary'}} />
                                    </IconButton>
                                </Tooltip>
                                <Tooltip title={t('teams.duplicateTeam')}>
                                    <IconButton size="small" aria-label={t('teams.duplicateTeam')}
                                                onClick={() => setDuplicateOpen(true)}>
                                        <Duplicate fontSize="small" sx={{color: 'text.secondary'}} />
                                    </IconButton>
                                </Tooltip>
                                {!currentTeam.is_default && (
                                    <Tooltip title={t('teams.deleteTeam')}>
                                        <IconButton size="small" color="error" aria-label={t('teams.deleteTeam')}
                                                    onClick={() => setDeleteOpen(true)}>
                                            <Delete fontSize="small" />
                                        </IconButton>
                                    </Tooltip>
                                )}
                            </Box>
                        )}
                        meta={(
                            <AgentStatusRow
                                observation={observation.data}
                                connected={!!observation.data?.lastRequestAt}
                                configStatus={null}
                                unconnectedText={t('agentStatus.noRequestsYet')}
                                leading={currentTeam.slug}
                            />
                        )}
                        actions={(
                            <>
                                <FormControlLabel
                                    labelPlacement="start"
                                    label={t(currentTeam.enabled ? 'common.enabled' : 'common.disabled')}
                                    control={(
                                        <Switch
                                            size="small"
                                            checked={currentTeam.enabled}
                                            disabled={toggling}
                                            onChange={() => void toggleTeam()}
                                            slotProps={{
                                                input: {
                                                    'aria-label': t(currentTeam.enabled
                                                        ? 'teams.disableTeam'
                                                        : 'teams.enableTeam'),
                                                },
                                            }}
                                        />
                                    )}
                                    sx={{
                                        m: 0,
                                        gap: 0.5,
                                        '& .MuiFormControlLabel-label': {
                                            color: 'text.secondary',
                                            fontSize: fontSizes.md,
                                        },
                                    }}
                                />
                                <AgentLogButton onOpenLog={() => setLogOpen(true)} />
                            </>
                        )}
                        // More buttons than an Agent page, so they share the title's row from lg up.
                        sx={agentPageHeaderSx('lg')}
                    />
                    {/* Part of the header: the Team's keys stay in view for handing out, and are managed here. */}
                    <TeamKeysOverview
                        team={currentTeam}
                        keys={teamKeys.keys}
                        loading={teamKeys.loading}
                        expanded={keysOverview.expanded}
                        onToggle={keysOverview.toggle}
                        actions={keyActions.tableProps}
                        onCreate={() => keyActions.openCreate(currentTeam)}
                    />
                    {!currentTeam.enabled && <Alert severity="warning">{t('teams.disabledHint')}</Alert>}
                    {/* Folds to one Base URL line, like the Agent pages; open until the first request. */}
                    <AgentAccessSection expanded={access.expanded} onToggle={access.toggle} url={`${baseUrl}/tingly/team`}
                        foldedExtras={pluginsActive && pluginsRow}
                    >
                        <Box sx={{maxWidth: SCENARIO_HEADER_CONTENT_MAX_WIDTH}}>
                            {/* No API Key row: that is this instance's own key; Team members
                                connect with their Team Key (listed above), never this one. */}
                            <ProviderConfigCard title={t('teams.accessTitle')} baseUrlPath="/tingly/team" baseUrl={baseUrl}
                                                onCopy={copyToClipboard} compact scenario={scenario} showPlugins={false}
                                                showApiKeyRow={false} />
                            <Box sx={{ px: 2, pt: 1 }}>{pluginsRow}</Box>
                        </Box>
                    </AgentAccessSection>

                    <CardGrid>
                    <TemplatePage key={scenario} scenario={scenario} collapsible allowDeleteRule hideTroubleshoot />
                    </CardGrid>
                </Stack>
            )}

            {keyActions.dialogs}

            <ScenarioLogDialog open={logOpen} onClose={() => setLogOpen(false)} scenario={scenario} title={t('agentStatus.log')} />
            <TeamGuideDialog open={teamGuideOpen} onClose={() => setTeamGuideOpen(false)} />

            <Dialog open={editorOpen} onClose={() => setEditorOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>{t('teams.editTeam')}</DialogTitle>
                <DialogContent>
                    <Stack spacing={2.5} sx={{mt: 1}}>
                        <TextField label={t('teams.name')} value={teamName} autoFocus fullWidth
                                   onChange={(event) => setTeamName(event.target.value)} />
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setEditorOpen(false)} disabled={saving}>{t('common.cancel')}</Button>
                    <Button variant="contained" onClick={saveTeam}
                            disabled={saving || !teamName.trim()}>
                        {saving && <CircularProgress size={16} sx={{mr: 1}} />}
                        {t('common.save')}
                    </Button>
                </DialogActions>
            </Dialog>

            <DuplicateDialog
                open={duplicateOpen}
                onClose={() => setDuplicateOpen(false)}
                title={t('teams.duplicateTitle')}
                description={t('teams.duplicateDescription', {team: currentTeam?.name, interpolation: {escapeValue: false}})}
                label={t('teams.name')}
                initialName={suggestTeamCopyName(currentTeam?.name ?? '', teams.map(team => team.name))}
                onSubmit={duplicateTeam}
            />

            <ConfirmDialog
                open={deleteOpen}
                onClose={() => setDeleteOpen(false)}
                title={t('teams.deleteTeam')}
                description={t('teams.deleteConfirm', {team: currentTeam?.name})}
                confirmLabel={t('teams.deleteTeam')}
                confirmColor="error"
                loading={saving}
                onConfirm={deleteTeam}
            />
        </PageLayout>
    );
};

const UseTeamPage: React.FC = () => (
    <ScenarioPageModalProvider><UseTeamPageContent /></ScenarioPageModalProvider>
);

export default UseTeamPage;

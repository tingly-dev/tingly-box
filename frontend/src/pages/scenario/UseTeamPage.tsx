import CardGrid from '@/components/CardGrid.tsx';
import UnifiedCard from '@/components/UnifiedCard.tsx';
import ProviderConfigCard from '@/components/ProviderConfigCard.tsx';
import {
    Alert, Box, Button, Chip, CircularProgress, Dialog, DialogActions,
    DialogContent, DialogTitle, MenuItem, Stack, TextField, Typography,
} from '@mui/material';
import { Add, Delete, Key as IconKey, Settings } from '@/components/icons';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import PageLayout from '@/components/PageLayout';
import ScenarioPageSkeleton from './components/ScenarioPageSkeleton';
import TemplatePage from './components/TemplatePage.tsx';
import SharingKeysDialog from './components/SharingKeysDialog.tsx';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';
import { api } from '@/services/api';
import { useNotify } from '@/hooks/useNotify';
import type { Team } from '@/types/team';

const selectedTeamStorageKey = 'selected_team_id';

const slugify = (value: string) => value.trim().toLowerCase()
    .replace(/[^a-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 64);

const UseTeamPageContent: React.FC = () => {
    const { t } = useTranslation();
    const notify = useNotify();
    const [teams, setTeams] = useState<Team[]>([]);
    const [teamsLoading, setTeamsLoading] = useState(true);
    const [selectedTeamID, setSelectedTeamID] = useState('');
    const [sharingKeysOpen, setSharingKeysOpen] = useState(false);
    const [editorOpen, setEditorOpen] = useState(false);
    const [editing, setEditing] = useState(false);
    const [teamName, setTeamName] = useState('');
    const [teamSlug, setTeamSlug] = useState('');
    const [savingTeam, setSavingTeam] = useState(false);
    const [deleteOpen, setDeleteOpen] = useState(false);

    const currentTeam = useMemo(
        () => teams.find((team) => team.id === selectedTeamID) || teams[0],
        [teams, selectedTeamID],
    );
    const scenario = currentTeam && !currentTeam.is_default ? `team:${currentTeam.id}` : 'team';
    const {isLoading, notification, copyToClipboard, baseUrl} = useScenarioPageInternal(scenario);

    const loadTeams = async (preferredTeamID?: string) => {
        setTeamsLoading(true);
        const result = await api.listTeams();
        setTeamsLoading(false);
        if (!result.success) {
            notify.error(result.error?.message || t('teams.loadFailed'));
            return;
        }
        const nextTeams: Team[] = result.data?.teams || [];
        setTeams(nextTeams);
        const saved = preferredTeamID || localStorage.getItem(selectedTeamStorageKey) || '';
        const selected = nextTeams.find((team) => team.id === saved)
            || nextTeams.find((team) => team.is_default) || nextTeams[0];
        if (selected) setSelectedTeamID(selected.id);
    };

    useEffect(() => { loadTeams(); }, []);

    const selectTeam = (teamID: string) => {
        setSelectedTeamID(teamID);
        localStorage.setItem(selectedTeamStorageKey, teamID);
    };

    const openCreate = () => {
        setEditing(false); setTeamName(''); setTeamSlug(''); setEditorOpen(true);
    };

    const openEdit = () => {
        if (!currentTeam) return;
        setEditing(true); setTeamName(currentTeam.name); setTeamSlug(currentTeam.slug); setEditorOpen(true);
    };

    const saveTeam = async () => {
        const name = teamName.trim();
        const slug = teamSlug.trim();
        if (!name || !slug) return;
        setSavingTeam(true);
        const result = editing && currentTeam
            ? await api.updateTeam(currentTeam.id, {name, slug})
            : await api.createTeam({name, slug});
        setSavingTeam(false);
        if (!result.success) {
            notify.error(result.error?.message || t('teams.saveFailed'));
            return;
        }
        setEditorOpen(false);
        notify.success(editing ? t('teams.updateSuccess') : t('teams.createSuccess'));
        await loadTeams(result.data?.id || currentTeam?.id);
    };

    const toggleCurrentTeam = async () => {
        if (!currentTeam) return;
        const result = await api.setTeamEnabled(currentTeam.id, !currentTeam.enabled);
        if (!result.success) {
            notify.error(result.error?.message || t('teams.saveFailed'));
            return;
        }
        notify.success(currentTeam.enabled ? t('teams.disabled') : t('teams.enabled'));
        await loadTeams(currentTeam.id);
    };

    const deleteCurrentTeam = async () => {
        if (!currentTeam) return;
        const result = await api.deleteTeam(currentTeam.id);
        if (!result.success) {
            notify.error(result.error?.message || t('teams.deleteFailed'));
            return;
        }
        setDeleteOpen(false);
        setEditorOpen(false);
        notify.success(t('teams.deleteSuccess'));
        await loadTeams();
    };

    return (
        <PageLayout loading={isLoading || teamsLoading} loadingContent={<ScenarioPageSkeleton />} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    titleHeadingLevel={1}
                    title="Team"
                    size="full"
                    rightAction={currentTeam && (
                        <Button variant="contained" size="small" startIcon={<IconKey sx={{fontSize: 18}} />}
                                onClick={() => setSharingKeysOpen(true)}>
                            {t('scenarioPage.sharingKeys')}
                        </Button>
                    )}
                >
                    {currentTeam && (
                        <Stack spacing={2.5}>
                            <Stack direction={{xs: 'column', sm: 'row'}} spacing={1.5} sx={{alignItems: {sm: 'center'}}}>
                                <TextField select size="small" label={t('teams.currentTeam')} value={currentTeam.id}
                                           onChange={(event) => selectTeam(event.target.value)} sx={{minWidth: 260}}>
                                    {teams.map((team) => (
                                        <MenuItem key={team.id} value={team.id}>
                                            <Stack direction="row" spacing={1} sx={{alignItems: 'center'}}>
                                                <span>{team.name}</span>
                                                {!team.enabled && <Chip size="small" label={t('teams.inactive')} />}
                                            </Stack>
                                        </MenuItem>
                                    ))}
                                </TextField>
                                <Button variant="outlined" size="small" startIcon={<Add />} onClick={openCreate}>
                                    {t('teams.newTeam')}
                                </Button>
                                <Button variant="text" size="small" startIcon={<Settings />} onClick={openEdit}>
                                    {t('teams.manageTeam')}
                                </Button>
                            </Stack>
                            {!currentTeam.enabled && <Alert severity="warning">{t('teams.disabledHint')}</Alert>}
                            <ProviderConfigCard title={currentTeam.name} baseUrlPath="/tingly/team" baseUrl={baseUrl}
                                                onCopy={copyToClipboard} compact scenario={scenario} />
                        </Stack>
                    )}
                </UnifiedCard>
                {currentTeam && <TemplatePage key={scenario} scenario={scenario} collapsible allowDeleteRule />}
            </CardGrid>

            {currentTeam && (
                <SharingKeysDialog open={sharingKeysOpen} onClose={() => setSharingKeysOpen(false)}
                                   team={currentTeam} teams={teams} />
            )}

            <Dialog open={editorOpen} onClose={() => setEditorOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>{editing ? t('teams.editTeam') : t('teams.createTeam')}</DialogTitle>
                <DialogContent>
                    <Stack spacing={2.5} sx={{mt: 1}}>
                        <TextField label={t('teams.name')} value={teamName} autoFocus fullWidth
                                   onChange={(event) => {
                                       setTeamName(event.target.value);
                                       if (!editing) setTeamSlug(slugify(event.target.value));
                                   }} />
                        <TextField label={t('teams.slug')} value={teamSlug} fullWidth helperText={t('teams.slugHelper')}
                                   onChange={(event) => setTeamSlug(slugify(event.target.value))} />
                        {editing && currentTeam && (
                            <Box>
                                <Button onClick={toggleCurrentTeam} color={currentTeam.enabled ? 'warning' : 'success'}>
                                    {currentTeam.enabled ? t('teams.disableTeam') : t('teams.enableTeam')}
                                </Button>
                                {!currentTeam.is_default && (
                                    <Button color="error" startIcon={<Delete />} onClick={() => setDeleteOpen(true)}>
                                        {t('teams.deleteTeam')}
                                    </Button>
                                )}
                            </Box>
                        )}
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setEditorOpen(false)}>{t('common.cancel')}</Button>
                    <Button variant="contained" onClick={saveTeam}
                            disabled={savingTeam || !teamName.trim() || !teamSlug.trim()}>
                        {savingTeam && <CircularProgress size={16} sx={{mr: 1}} />}
                        {t('common.save')}
                    </Button>
                </DialogActions>
            </Dialog>

            <Dialog open={deleteOpen} onClose={() => setDeleteOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>{t('teams.deleteTeam')}</DialogTitle>
                <DialogContent><Typography>{t('teams.deleteConfirm', {team: currentTeam?.name})}</Typography></DialogContent>
                <DialogActions>
                    <Button onClick={() => setDeleteOpen(false)}>{t('common.cancel')}</Button>
                    <Button variant="contained" color="error" onClick={deleteCurrentTeam}>{t('teams.deleteTeam')}</Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
};

const UseTeamPage: React.FC = () => (
    <ScenarioPageModalProvider><UseTeamPageContent /></ScenarioPageModalProvider>
);

export default UseTeamPage;

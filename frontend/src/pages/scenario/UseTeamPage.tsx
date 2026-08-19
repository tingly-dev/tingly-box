import CardGrid from '@/components/CardGrid.tsx';
import UnifiedCard from '@/components/UnifiedCard.tsx';
import ProviderConfigCard from '@/components/ProviderConfigCard.tsx';
import {
    Alert, Box, Button, Chip, CircularProgress, Dialog, DialogActions,
    DialogContent, DialogTitle, IconButton, Stack, TextField, Tooltip, Typography,
} from '@mui/material';
import {Delete, Edit, Key as IconKey} from '@/components/icons';
import {useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {useNavigate, useParams} from 'react-router-dom';
import PageLayout from '@/components/PageLayout';
import ScenarioPageSkeleton from './components/ScenarioPageSkeleton';
import TemplatePage from './components/TemplatePage.tsx';
import SharingKeysDialog from './components/SharingKeysDialog.tsx';
import {useScenarioPageInternal} from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import {ScenarioPageModalProvider} from '@/pages/scenario/context/ScenarioPageContext';
import {api} from '@/services/api';
import {useNotify} from '@/hooks/useNotify';
import {useTeamContext} from '@/contexts/TeamContext';

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

    const [sharingKeysOpen, setSharingKeysOpen] = useState(false);
    const [editorOpen, setEditorOpen] = useState(false);
    const [deleteOpen, setDeleteOpen] = useState(false);
    const [teamName, setTeamName] = useState('');
    const [saving, setSaving] = useState(false);

    useEffect(() => {
        if (!teamsLoading && teamSlug && !currentTeam) navigate('/agent/team', {replace: true});
    }, [teamsLoading, teamSlug, currentTeam, navigate]);

    const openEditor = () => {
        if (!currentTeam) return;
        setTeamName(currentTeam.name);
        setEditorOpen(true);
    };

    const saveTeam = async () => {
        if (!currentTeam || !teamName.trim()) return;
        setSaving(true);
        const result = await api.updateTeam(currentTeam.id, {name: teamName.trim()});
        setSaving(false);
        if (!result.success) {
            notify.error(result.error?.message || t('teams.saveFailed'));
            return;
        }
        setEditorOpen(false);
        await refresh();
        notify.success(t('teams.updateSuccess'));
    };

    const toggleTeam = async () => {
        if (!currentTeam) return;
        setSaving(true);
        const result = await api.setTeamEnabled(currentTeam.id, !currentTeam.enabled);
        setSaving(false);
        if (!result.success) {
            notify.error(result.error?.message || t('teams.saveFailed'));
            return;
        }
        await refresh();
        notify.success(currentTeam.enabled ? t('teams.disabled') : t('teams.enabled'));
    };

    const deleteTeam = async () => {
        if (!currentTeam) return;
        setSaving(true);
        const result = await api.deleteTeam(currentTeam.id);
        setSaving(false);
        if (!result.success) {
            notify.error(result.error?.message || t('teams.deleteFailed'));
            return;
        }
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
                <CardGrid>
                    <UnifiedCard
                        titleHeadingLevel={1}
                        title={(
                            <Box sx={{display: 'flex', alignItems: 'center', gap: 1}}>
                                <span>{currentTeam.name}</span>
                                <Chip size="small" variant="outlined" label={currentTeam.slug} />
                                <Tooltip title={t('teams.editTeam')}>
                                    <IconButton size="small" onClick={openEditor}><Edit fontSize="small" /></IconButton>
                                </Tooltip>
                                {!currentTeam.is_default && (
                                    <Tooltip title={t('teams.deleteTeam')}>
                                        <IconButton size="small" color="error" onClick={() => setDeleteOpen(true)}>
                                            <Delete fontSize="small" />
                                        </IconButton>
                                    </Tooltip>
                                )}
                            </Box>
                        )}
                        size="full"
                        rightAction={(
                            <Button variant="contained" size="small" startIcon={<IconKey sx={{fontSize: 18}} />}
                                    onClick={() => setSharingKeysOpen(true)}>
                                {t('scenarioPage.sharingKeys')}
                            </Button>
                        )}
                    >
                        <Stack spacing={2}>
                            {!currentTeam.enabled && <Alert severity="warning">{t('teams.disabledHint')}</Alert>}
                            <ProviderConfigCard title={t('teams.accessTitle')} baseUrlPath="/tingly/team" baseUrl={baseUrl}
                                                onCopy={copyToClipboard} compact scenario={scenario} />
                        </Stack>
                    </UnifiedCard>
                    <TemplatePage key={scenario} scenario={scenario} collapsible allowDeleteRule />
                </CardGrid>
            )}

            {currentTeam && (
                <SharingKeysDialog open={sharingKeysOpen} onClose={() => setSharingKeysOpen(false)}
                                   team={currentTeam} teams={teams} />
            )}

            <Dialog open={editorOpen} onClose={() => setEditorOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>{t('teams.editTeam')}</DialogTitle>
                <DialogContent>
                    <Stack spacing={2.5} sx={{mt: 1}}>
                        <TextField label={t('teams.name')} value={teamName} autoFocus fullWidth
                                   onChange={(event) => setTeamName(event.target.value)} />
                        <Box>
                            <Button onClick={toggleTeam} disabled={saving}
                                    color={currentTeam?.enabled ? 'warning' : 'success'}>
                                {currentTeam?.enabled ? t('teams.disableTeam') : t('teams.enableTeam')}
                            </Button>
                        </Box>
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

            <Dialog open={deleteOpen} onClose={() => setDeleteOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>{t('teams.deleteTeam')}</DialogTitle>
                <DialogContent>
                    <Typography>{t('teams.deleteConfirm', {team: currentTeam?.name})}</Typography>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setDeleteOpen(false)} disabled={saving}>{t('common.cancel')}</Button>
                    <Button variant="contained" color="error" onClick={deleteTeam} disabled={saving}>
                        {t('teams.deleteTeam')}
                    </Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
};

const UseTeamPage: React.FC = () => (
    <ScenarioPageModalProvider><UseTeamPageContent /></ScenarioPageModalProvider>
);

export default UseTeamPage;

import CardGrid from "@/components/CardGrid.tsx";
import PageLayout from '@/components/PageLayout';
import PageHeader from '@/components/PageHeader';
import ScenarioLogDialog from '@/components/RuleLogDialog';
import AgentActivityDialog from './components/AgentActivityDialog';
import AgentStatusRow, { AgentLooksIn } from './components/AgentStatusRow';
import AgentAccessSection, { useAccessExpanded } from './components/AgentAccessSection';
import ProfileSetupDialog from './components/ProfileSetupDialog';
import { useAgentObservation } from '@/pages/scenario/hooks/useAgentObservation';
import { isCredentialProvider } from '@/utils/providers';
import ScenarioPageSkeleton from './components/ScenarioPageSkeleton';
import { SCENARIO_HEADER_CONTENT_MAX_WIDTH, agentPageHeaderSx } from './components/ScenarioCardHeader';
import AgentPluginsCard from './components/AgentPluginsCard';
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import { useSlotRouting } from "@/pages/scenario/hooks/useSlotRouting";
import ConfigRow from "@/components/ConfigRow.tsx";
import { ChoiceToggle } from "@/components/ChoiceToggle";
import { useProfileContext } from '@/contexts/ProfileContext';
import { useVersion } from '@/contexts/VersionContext';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { api } from '@/services/api';
import { copyableTextStyle } from '@/styles/textStyles';
import { ContentCopy as ContentCopyIcon } from '@/components/icons';
import { Delete as DeleteIcon } from '@/components/icons';
import { Edit as EditIcon } from '@/components/icons';
import { Info as InfoIcon } from '@/components/icons';
import { Terminal as TerminalIcon } from '@/components/icons';
import Switch from '@mui/material/Switch';
import {
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Divider,
    IconButton,
    Stack,
    TextField,
    Tooltip,
    Typography
} from '@mui/material';
import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import TemplatePage from './components/TemplatePage.tsx';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';
import ClaudeCodeProfileOverrides, { type ClaudeCodeProfileSettingsArtifact } from './components/ClaudeCodeProfileOverrides';
import ConfirmDialog from '@/components/ConfirmDialog';
import { fontMono, fontSizes } from '@/theme/fonts';

const BASE_SCENARIO = 'claude_code';

// Maps a backend launch_source to the matching quick-start command mode, so
// the panel opens on the form that already works for this install instead of
// always assuming npx. null means "no match" (not loaded yet) — callers keep
// whatever mode is already selected.
const commandModeForSource = (source: string | null): 'npx' | 'global' | null => {
    switch (source) {
        case 'npx':
        case 'npx-bundle':
            return 'npx';
        case 'npm':
        case 'npm-bundle':
        case 'binary':
            return 'global';
        default:
            return null;
    }
};

const ClaudeCodeProfilePageContent: React.FC = () => {
    const { profileId } = useParams<{ profileId: string }>();
    const navigate = useNavigate();
    const { t } = useTranslation();
    const scenario = `${BASE_SCENARIO}:${profileId}`;

    const {
        showTokenModal,
        setShowTokenModal,
        token,
        showNotification,
        providers,
        notification,
        copyToClipboard,
        baseUrl,
        isLoading,
    } = useScenarioPageInternal(scenario, { skipRules: true });
    const { launchSource } = useVersion();

    // Profile state
    const { getProfiles, refresh: refreshProfiles } = useProfileContext();
    const currentProfile = getProfiles(BASE_SCENARIO).find(p => p.id === profileId);
    const [requestsOpen, setRequestsOpen] = useState(false);
    const [setupOpen, setSetupOpen] = useState(false);
    const [logOpen, setLogOpen] = useState(false);
    const [renameProfileOpen, setRenameProfileOpen] = useState(false);
    const [deleteProfileOpen, setDeleteProfileOpen] = useState(false);
    const [renameName, setRenameName] = useState('');
    const [isProfileMutating, setIsProfileMutating] = useState(false);
    const [appVersion, setAppVersion] = useState('');
    const [unifiedMode, setUnifiedMode] = useState(currentProfile?.unified || false);
    // The profile's rules (and, in unified mode, its Slots row). Its mode is
    // fixed at creation, so there is no switch.
    const slots = useSlotRouting(scenario, `builtin:${scenario}:cc`, !!currentProfile, unifiedMode ? 'unified' : 'separate');
    // npx matches the primary installation path and does not assume that the
    // package has also installed a permanent executable on PATH. Keep the
    // global command available as an explicit alternative for those who have.
    const [commandMode, setCommandMode] = useState<'npx' | 'global'>('npx');
    // Once the launch source arrives, adopt its matching mode — but only
    // until the user picks one themselves, so a later poll never yanks the
    // toggle out from under them.
    const [commandModeTouched, setCommandModeTouched] = useState(false);
    useEffect(() => {
        if (commandModeTouched) return;
        const preferred = commandModeForSource(launchSource);
        if (preferred) setCommandMode(preferred);
    }, [launchSource, commandModeTouched]);
    const handleCommandModeChange = (mode: 'npx' | 'global') => {
        setCommandModeTouched(true);
        setCommandMode(mode);
    };
    const [settingsArtifact, setSettingsArtifact] = useState<ClaudeCodeProfileSettingsArtifact | null>(null);
    // Quick Start / Settings File share one ConfigRow (like Base URL | API Key
    // elsewhere) instead of two separate always-expanded rows — same info,
    // less vertical space, one at a time.
    const [profileConfigTab, setProfileConfigTab] = useState<'quickstart' | 'settings-file'>('quickstart');

    // Update unified mode when profile changes
    useEffect(() => {
        setUnifiedMode(currentProfile?.unified || false);
    }, [currentProfile]);

    useEffect(() => {
        setSettingsArtifact(null);
    }, [profileId, currentProfile?.name]);

    const handleArtifactChange = React.useCallback((artifact: ClaudeCodeProfileSettingsArtifact) => {
        setSettingsArtifact(artifact);
    }, []);

    // Load app version for npm command
    useEffect(() => {
        api.getVersion().then(setAppVersion);
    }, []);

    // Refresh profiles on mount to ensure we have the latest data
    useEffect(() => {
        refreshProfiles();
    }, [refreshProfiles]);

    // Rename profile handler
    const handleRenameProfile = async () => {
        if (!renameName.trim() || !profileId) return;
        try {
            setIsProfileMutating(true);
            const result = await api.updateProfile(BASE_SCENARIO, profileId, renameName.trim());
            if (result.success) {
                showNotification(t('claudeCode.profile.profileRenamed'), 'success');
                setRenameProfileOpen(false);
                refreshProfiles();
            } else {
                showNotification(`${t('claudeCode.profile.renameFailed')}: ${result.error || 'Unknown error'}`, 'error');
            }
        } catch {
            showNotification(t('claudeCode.profile.renameFailed'), 'error');
        } finally {
            setIsProfileMutating(false);
        }
    };

    // Delete profile handler
    const handleDeleteProfile = async () => {
        if (!profileId) return;
        try {
            setIsProfileMutating(true);
            const result = await api.deleteProfile(BASE_SCENARIO, profileId);
            if (result.success) {
                showNotification(t('claudeCode.profile.profileDeleted'), 'success');
                setDeleteProfileOpen(false);
                refreshProfiles();
                navigate('/agent/claude_code');
            } else {
                showNotification(`${t('claudeCode.profile.deleteFailed')}: ${result.error || 'Unknown error'}`, 'error');
            }
        } catch {
            showNotification(t('claudeCode.profile.deleteFailed'), 'error');
        } finally {
            setIsProfileMutating(false);
        }
    };

    // Profile quick start command. Keep the old `cc --profile` form compatible
    // in the CLI, but surface the new direct `profile <name>` form in the UI.
    const ccCommand = React.useMemo(() => {
        if (commandMode === 'npx') {
            const packageSpec = appVersion ? `tingly-box@${appVersion}` : 'tingly-box';
            return `npx -y ${packageSpec} profile ${profileId}`;
        }
        return `tingly-box profile ${profileId}`;
    }, [commandMode, appVersion, profileId]);

    // Facts, not clicks: a provider exists, a request has come through this profile.
    const observation = useAgentObservation(scenario);
    const launched = observation.data ? !!observation.data.lastRequestAt : null;
    const providerConnected = isLoading ? null : providers.some((p: any) => p.enabled && isCredentialProvider(p));
    const access = useAccessExpanded(scenario, launched !== true);
    const accessUrl = `${baseUrl}/tingly/${scenario}`;

    return (
        <PageLayout loading={isLoading || slots.loading} loadingContent={<ScenarioPageSkeleton />} notification={notification}>
            <Stack spacing={2}>
            <PageHeader
                // The profile's alias is its name: "Claude Code - ds". Rename / delete act on
                // that name, so they sit right next to it rather than among the page actions.
                title={
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0 }}>
                        <Box component="span" sx={{ minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                            {`Claude Code - ${currentProfile?.name || profileId}`}
                        </Box>
                        <Tooltip title={`Base URL: ${baseUrl}/tingly/${scenario}`}>
                            <IconButton size="small" aria-label="Base URL">
                                <InfoIcon fontSize="small" sx={{ color: 'text.secondary' }} />
                            </IconButton>
                        </Tooltip>
                        <Tooltip title={t('claudeCode.profile.renameProfile')}>
                            <IconButton size="small" aria-label={t('claudeCode.profile.renameProfile')} onClick={() => { setRenameName(currentProfile?.name || ''); setRenameProfileOpen(true); }}>
                                <EditIcon fontSize="small" sx={{ color: 'text.secondary' }} />
                            </IconButton>
                        </Tooltip>
                        <Tooltip title={t('claudeCode.profile.deleteProfile')}>
                            <IconButton size="small" color="error" aria-label={t('claudeCode.profile.deleteProfile')} onClick={() => setDeleteProfileOpen(true)}>
                                <DeleteIcon fontSize="small" />
                            </IconButton>
                        </Tooltip>
                    </Box>
                }
                meta={(
                    <AgentStatusRow
                        observation={observation.data}
                        connected={launched === true}
                        configStatus={null}
                        unconnectedText={t('agentStatus.noRequestsYet')}
                        // The alias is in the title; the id stays here since the launch command uses it.
                        leading={`${currentProfile ? `${profileId} · ` : ''}${unifiedMode ? t('claudeCode.profile.unified') : t('claudeCode.profile.separate')}`}
                    />
                )}
                // Same shape as the Agent pages: looks-in first, the key action
                // (Setup Guide) filled and rightmost.
                actions={(
                    <>
                        <AgentLooksIn
                            onOpenRequests={() => setRequestsOpen(true)}
                            onOpenLog={() => setLogOpen(true)}
                            onOpenUsage={() => navigate(`/dashboard/today?scenario=${encodeURIComponent(scenario)}`)}
                        />
                        <Button onClick={() => setSetupOpen(true)} variant="contained" size="small">
                            {t('scenarioPage.setupGuide')}
                        </Button>
                    </>
                )}
                sx={agentPageHeaderSx()}
            />
                <AgentAccessSection expanded={access.expanded} onToggle={access.toggle} url={accessUrl}>
                  <Box sx={{ maxWidth: SCENARIO_HEADER_CONTENT_MAX_WIDTH }}>
                    <Box sx={{ px: 2, pb: 0.5 }}>
                        <ConfigRow
                            tabs={[
                                {
                                    key: 'quickstart',
                                    label: t('claudeCode.profile.quickStart'),
                                    content: (
                                        <Typography
                                            variant="subtitle2"
                                            onClick={() => copyToClipboard(ccCommand, 'command')}
                                            sx={copyableTextStyle}
                                            title={t('claudeCode.profile.clickToCopy')}
                                        >
                                            {ccCommand}
                                        </Typography>
                                    ),
                                    actions: (
                                        <>
                                            <Tooltip title={t('claudeCode.profile.copyCommand')} arrow>
                                                <IconButton
                                                    size="small"
                                                    aria-label={t('claudeCode.profile.copyCommand')}
                                                    onClick={() => copyToClipboard(ccCommand, 'command')}
                                                >
                                                    <ContentCopyIcon fontSize="small" />
                                                </IconButton>
                                            </Tooltip>
                                            <ChoiceToggle
                                                value={commandMode}
                                                onChange={handleCommandModeChange}
                                                options={[
                                                    {
                                                        value: 'npx', label: 'NPX', tooltip: 'Use npx command',
                                                        // The same green "n" disc the old icon-only switch used for npx.
                                                        icon: (
                                                            <Box component="span" sx={{ width: 16, height: 16, borderRadius: '50%', bgcolor: 'success.main', color: 'background.paper', display: 'inline-flex', alignItems: 'center', justifyContent: 'center', fontSize: 11, fontWeight: 700, lineHeight: 1 }}>
                                                                n
                                                            </Box>
                                                        ),
                                                    },
                                                    { value: 'global', label: t('claudeCode.profile.commandGlobal'), tooltip: 'Use global CLI command', icon: <TerminalIcon /> },
                                                ]}
                                            />
                                        </>
                                    ),
                                },
                                {
                                    key: 'settings-file',
                                    label: t('claudeCode.profile.settingsFile'),
                                    content: settingsArtifact?.settingsPath ? (
                                        <Tooltip title={t('claudeCode.profile.settingsFileWarning')} arrow>
                                            <Typography
                                                variant="subtitle2"
                                                onClick={() => copyToClipboard(settingsArtifact.settingsPath, 'settings-file')}
                                                sx={{ ...copyableTextStyle, fontFamily: fontMono, fontSize: fontSizes.md }}
                                            >
                                                {settingsArtifact.settingsPath}
                                            </Typography>
                                        </Tooltip>
                                    ) : (
                                        <Typography variant="body2" color="text.secondary">
                                            {t('claudeCode.profile.resolvingSettingsFile')}
                                        </Typography>
                                    ),
                                    actions: (
                                        <>
                                            <Tooltip title={t('claudeCode.profile.copySettingsFile')} arrow>
                                                <span>
                                                    <IconButton
                                                        size="small"
                                                        aria-label={t('claudeCode.profile.copySettingsFile')}
                                                        disabled={!settingsArtifact?.settingsPath}
                                                        onClick={() => settingsArtifact?.settingsPath && copyToClipboard(settingsArtifact.settingsPath, 'settings-file')}
                                                    >
                                                        <ContentCopyIcon fontSize="small" />
                                                    </IconButton>
                                                </span>
                                            </Tooltip>
                                            <Box aria-hidden sx={{ width: 30, height: 30, flexShrink: 0 }} />
                                            <Tooltip
                                                title={settingsArtifact
                                                    ? settingsArtifact.settingsExists
                                                        ? t('claudeCode.profile.settingsGenerated')
                                                        : t('claudeCode.profile.settingsNotGenerated')
                                                    : ''}
                                                arrow
                                            >
                                                <Box
                                                    role={settingsArtifact ? 'status' : undefined}
                                                    aria-label={settingsArtifact
                                                        ? settingsArtifact.settingsExists
                                                            ? t('claudeCode.profile.settingsGenerated')
                                                            : t('claudeCode.profile.settingsNotGenerated')
                                                        : undefined}
                                                    sx={{
                                                        width: 30,
                                                        height: 30,
                                                        flexShrink: 0,
                                                        display: 'flex',
                                                        alignItems: 'center',
                                                        justifyContent: 'center',
                                                        color: settingsArtifact?.settingsExists ? 'success.main' : 'text.disabled',
                                                        visibility: settingsArtifact ? 'visible' : 'hidden',
                                                    }}
                                                >
                                                    <InfoIcon fontSize="small" />
                                                </Box>
                                            </Tooltip>
                                        </>
                                    ),
                                },
                            ]}
                            activeTab={profileConfigTab}
                            onTabChange={(key) => setProfileConfigTab(key as 'quickstart' | 'settings-file')}
                        />
                    </Box>
                    <ProviderConfigCard
                        title={`Claude Code [${profileId}]`}
                        baseUrlPath={`/tingly/${scenario}`}
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                        scenario={scenario}
                        showApiKeyRow={true}
                        showBaseUrlRow={true}
                        compact={true}
                        showPlugins={false}
                        extraContent={slots.slotsRow}
                    />
                  </Box>
                </AgentAccessSection>
                {/* Page-level plugins: their own card, as on the Agent pages, shown whether
                    the connection above is folded or not. */}
                <AgentPluginsCard scenario={scenario} />
            <CardGrid>

                <ClaudeCodeProfileOverrides
                    profileId={profileId || ''}
                    profileName={currentProfile?.name}
                    onArtifactChange={handleArtifactChange}
                />

                <TemplatePage
                    scenario={scenario}
                    rules={slots.rules}
                    onRulesChange={slots.setRules}
                    collapsible={true}
                    allowToggleRule={false}
                    allowAddRule={false}
                    hideTroubleshoot
                />

                <ScenarioLogDialog open={logOpen} onClose={() => setLogOpen(false)} scenario={scenario} title={t('agentStatus.log')} />
                <AgentActivityDialog scenario={scenario} open={requestsOpen} onClose={() => setRequestsOpen(false)} />

                <ProfileSetupDialog
                    open={setupOpen}
                    onClose={() => setSetupOpen(false)}
                    command={ccCommand}
                    providerConnected={providerConnected}
                    launched={launched}
                    onConnectProvider={() => navigate('/help')}
                />

                {/* Rename profile dialog */}
                <Dialog
                    open={renameProfileOpen}
                    onClose={() => setRenameProfileOpen(false)}
                    maxWidth="xs"
                    fullWidth
                    slotProps={{
                        paper: {
                            sx: { overflow: 'visible' }
                        }
                    }}
                >
                    <DialogTitle>{t('claudeCode.profile.renameTitle')}</DialogTitle>
                    <DialogContent sx={{ pt: 1 }}>
                        <TextField
                            autoFocus
                            fullWidth
                            label={t('claudeCode.profile.profileName')}
                            value={renameName}
                            onChange={(e) => setRenameName(e.target.value)}
                            onKeyDown={(e) => e.key === 'Enter' && handleRenameProfile()}
                            size="small"
                            margin="dense"
                        />
                    </DialogContent>
                    <DialogActions sx={{ px: 3, pb: 2, gap: 1, justifyContent: 'flex-end' }}>
                        <Button onClick={() => setRenameProfileOpen(false)} color="inherit" disabled={isProfileMutating}>
                            Cancel
                        </Button>
                        <Button onClick={handleRenameProfile} variant="contained" disabled={!renameName.trim() || isProfileMutating}>
                            {t('claudeCode.profile.save')}
                        </Button>
                    </DialogActions>
                </Dialog>

                <ConfirmDialog
                    open={deleteProfileOpen}
                    onClose={() => setDeleteProfileOpen(false)}
                    onConfirm={handleDeleteProfile}
                    loading={isProfileMutating}
                    confirmColor="error"
                    confirmLabel="Delete"
                    title={t('claudeCode.profile.deleteTitle')}
                    description={
                        <>
                            <Typography variant="body1">
                                {t('claudeCode.profile.deleteConfirm', { name: currentProfile?.name || profileId || '', interpolation: { escapeValue: false } })}
                            </Typography>
                            <Typography variant="body2" sx={{ color: "text.secondary", mt: 1 }}>
                                {t('claudeCode.profile.deleteWarning')}
                            </Typography>
                        </>
                    }
                />
            </CardGrid>
            </Stack>
        </PageLayout>
    );
};

const ClaudeCodeProfilePage: React.FC = () => {
    return (
        <ScenarioPageModalProvider>
            <ClaudeCodeProfilePageContent />
        </ScenarioPageModalProvider>
    );
};

export default ClaudeCodeProfilePage;

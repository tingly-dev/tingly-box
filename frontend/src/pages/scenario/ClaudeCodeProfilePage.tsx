import CardGrid from "@/components/CardGrid.tsx";
import PageLayout from '@/components/PageLayout';
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ConfigRow from "@/components/ConfigRow.tsx";
import { ActiveBadge } from "@/components/ActiveBadge";
import { useProfileContext } from '@/contexts/ProfileContext';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { api } from '@/services/api';
import { copyableTextStyle } from '@/styles/textStyles';
import { ContentCopy as ContentCopyIcon } from '@/components/icons';
import { Delete as DeleteIcon } from '@/components/icons';
import { Edit as EditIcon } from '@/components/icons';
import { Info as InfoIcon } from '@/components/icons';
import { Terminal as TerminalIcon } from '@/components/icons';
import Chip from '@mui/material/Chip';
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
import React, { useCallback, useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import TemplatePage from './components/TemplatePage.tsx';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

const BASE_SCENARIO = 'claude_code';

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
    } = useScenarioPageInternal(scenario);

    // Rules state
    const [rules, setRules] = useState<any[]>([]);
    const [loadingRule, setLoadingRule] = useState(true);

    // Profile state
    const { getProfiles, refresh: refreshProfiles } = useProfileContext();
    const currentProfile = getProfiles(BASE_SCENARIO).find(p => p.id === profileId);
    const [renameProfileOpen, setRenameProfileOpen] = useState(false);
    const [deleteProfileOpen, setDeleteProfileOpen] = useState(false);
    const [renameName, setRenameName] = useState('');
    const [isProfileMutating, setIsProfileMutating] = useState(false);
    const [appVersion, setAppVersion] = useState('');
    const [unifiedMode, setUnifiedMode] = useState(currentProfile?.unified || false);
    const [isUpdatingMode, setIsUpdatingMode] = useState(false);
    const [commandMode, setCommandMode] = useState<'npx' | 'global'>('npx');

    // Update unified mode when profile changes
    useEffect(() => {
        setUnifiedMode(currentProfile?.unified || false);
    }, [currentProfile]);

    // Load rules for this profile
    const loadRules = useCallback(async () => {
        setLoadingRule(true);
        // Profile rules have their own scenario (e.g., claude_code:p1)
        // Just load all rules for this profile scenario
        const result = await api.getRules(scenario);
        setRules(result.success ? result.data : []);
        setLoadingRule(false);
    }, [scenario]);

    useEffect(() => {
        loadRules();
    }, [loadRules]);

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

    // Handle mode toggle
    const handleModeToggle = async () => {
        if (!profileId) return;
        const newMode = !unifiedMode;
        try {
            setIsUpdatingMode(true);
            // Only pass unified mode, let backend use existing name
            const result = await api.updateProfile(BASE_SCENARIO, profileId, '', newMode);
            if (result.success) {
                setUnifiedMode(newMode);
                showNotification(t('claudeCode.profile.modeUpdated', { mode: newMode ? t('claudeCode.profile.unified') : t('claudeCode.profile.separate') }), 'success');
                refreshProfiles();
                // Reload rules after mode change
                await loadRules();
            } else {
                showNotification(`${t('claudeCode.profile.modeUpdateFailed')}: ${result.error || 'Unknown error'}`, 'error');
            }
        } catch {
            showNotification(t('claudeCode.profile.modeUpdateFailed'), 'error');
        } finally {
            setIsUpdatingMode(false);
        }
    };

    const ccCommand = React.useMemo(() => {
        if (commandMode === 'npx' && appVersion) {
            return `npx -y tingly-box@${appVersion} cc --profile ${profileId}`;
        }
        return `tingly-box cc --profile ${profileId}`;
    }, [commandMode, appVersion, profileId]);

    return (
        <PageLayout loading={loadingRule} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    size="full"
                    title={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1 }}>
                            <span>Claude Code</span>
                            <Tooltip title={`Base URL: ${baseUrl}/tingly/${scenario}`}>
                                <IconButton size="small">
                                    <InfoIcon fontSize="small" sx={{ color: 'text.secondary' }} />
                                </IconButton>
                            </Tooltip>
                            <Tooltip title={t('claudeCode.profile.renameProfile')}>
                                <IconButton size="small" onClick={() => { setRenameName(currentProfile?.name || ''); setRenameProfileOpen(true); }}>
                                    <EditIcon fontSize="small" />
                                </IconButton>
                            </Tooltip>
                            <Tooltip title={t('claudeCode.profile.deleteProfile')}>
                                <IconButton size="small" color="error" onClick={() => setDeleteProfileOpen(true)}>
                                    <DeleteIcon fontSize="small" />
                                </IconButton>
                            </Tooltip>
                        </Box>
                    }
                    rightAction={
                        <Stack direction="row" spacing={1} alignItems="center">
                            <Chip
                                label={unifiedMode ? t('claudeCode.profile.unified') : t('claudeCode.profile.separate')}
                                size="small"
                                variant="outlined"
                                color={unifiedMode ? "primary" : "default"}
                            />
                            <Chip label={currentProfile ? `${profileId} - ${currentProfile.name}` : profileId} size="small" variant="outlined" />
                        </Stack>
                    }
                >
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
                                            <Tooltip title="Use npx command">
                                                <IconButton
                                                    size="small"
                                                    onClick={() => setCommandMode('npx')}
                                                    sx={{
                                                        position: 'relative',
                                                        opacity: commandMode === 'npx' ? 1 : 0.5,
                                                        transition: 'opacity 0.2s',
                                                        '&:hover': {
                                                            opacity: 1,
                                                            backgroundColor: 'action.hover',
                                                        },
                                                    }}
                                                >
                                                    <Box
                                                        sx={{
                                                            width: 20,
                                                            height: 20,
                                                            borderRadius: '50%',
                                                            backgroundColor: 'success.main',
                                                            display: 'flex',
                                                            alignItems: 'center',
                                                            justifyContent: 'center',
                                                        }}
                                                    >
                                                        <Typography sx={{ fontSize: '12px', lineHeight: 1, color: 'background.paper', fontWeight: 'bold' }}>
                                                            n
                                                        </Typography>
                                                    </Box>
                                                    {commandMode === 'npx' && <ActiveBadge />}
                                                </IconButton>
                                            </Tooltip>
                                            <Tooltip title="Use global CLI command">
                                                <IconButton
                                                    size="small"
                                                    onClick={() => setCommandMode('global')}
                                                    sx={{
                                                        opacity: commandMode === 'global' ? 1 : 0.5,
                                                        transition: 'opacity 0.2s',
                                                        '&:hover': {
                                                            opacity: 1,
                                                            backgroundColor: 'action.hover',
                                                        },
                                                    }}
                                                >
                                                    <TerminalIcon fontSize="small" sx={{ color: 'text.primary' }} />
                                                    {commandMode === 'global' && <ActiveBadge />}
                                                </IconButton>
                                            </Tooltip>
                                        </>
                                    ),
                                },
                            ]}
                            activeTab="quickstart"
                            onTabChange={() => {}}
                        />
                    </Box>
                    <Box sx={{ px: 2, py: 0.5 }}>
                        <ConfigRow
                            tabs={[
                                {
                                    key: 'mode',
                                    label: t('claudeCode.profile.mode'),
                                    content: (
                                        <Typography
                                            variant="subtitle2"
                                            color="text.secondary"
                                            sx={{
                                                fontFamily: 'monospace',
                                                fontSize: '0.75rem',
                                                cursor: 'pointer',
                                                '&:hover': {
                                                    textDecoration: 'underline',
                                                    backgroundColor: 'action.hover'
                                                },
                                                padding: 1,
                                                borderRadius: 1,
                                                transition: 'all 0.2s ease-in-out'
                                            }}
                                        >
                                            {unifiedMode
                                                ? t('claudeCode.profile.unifiedDescription')
                                                : t('claudeCode.profile.separateDescription')}
                                        </Typography>
                                    ),
                                },
                            ]}
                            activeTab="mode"
                            onTabChange={() => {}}
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
                    />
                </UnifiedCard>

                <TemplatePage
                    title="Models and Forwarding Rules"
                    scenario={scenario}
                    rules={rules}
                    onRulesChange={setRules}
                    collapsible={true}
                    allowToggleRule={false}
                    allowAddRule={false}
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

                {/* Delete profile confirmation dialog */}
                <Dialog
                    open={deleteProfileOpen}
                    onClose={() => setDeleteProfileOpen(false)}
                    maxWidth="xs"
                    fullWidth
                >
                    <DialogTitle>{t('claudeCode.profile.deleteTitle')}</DialogTitle>
                    <DialogContent sx={{ pt: 3 }}>
                        <Typography variant="body1">
                            {t('claudeCode.profile.deleteConfirm', { name: currentProfile?.name || profileId || '', interpolation: { escapeValue: false } })}
                        </Typography>
                        <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
                            {t('claudeCode.profile.deleteWarning')}
                        </Typography>
                    </DialogContent>
                    <DialogActions sx={{ px: 3, pb: 2, gap: 1, justifyContent: 'flex-end' }}>
                        <Button onClick={() => setDeleteProfileOpen(false)} color="inherit" disabled={isProfileMutating}>
                            Cancel
                        </Button>
                        <Button onClick={handleDeleteProfile} variant="contained" color="error" disabled={isProfileMutating}>
                            Delete
                        </Button>
                    </DialogActions>
                </Dialog>
            </CardGrid>
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

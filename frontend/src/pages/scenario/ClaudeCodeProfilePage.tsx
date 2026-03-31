import CardGrid from "@/components/CardGrid.tsx";
import PageLayout from '@/components/PageLayout';
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import { useProfileContext } from '@/contexts/ProfileContext';
import { useScenarioPageInternal } from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import { api } from '@/services/api';
import { ContentCopy as ContentCopyIcon } from '@mui/icons-material';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import InfoIcon from '@mui/icons-material/Info';
import CodeIcon from '@mui/icons-material/Code';
import TerminalIcon from '@mui/icons-material/Terminal';
import Chip from '@mui/material/Chip';
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

const BASE_SCENARIO = 'claude_code';

const ClaudeCodeProfilePage: React.FC = () => {
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
    const [npmMode, setNpmMode] = useState(true);
    const [appVersion, setAppVersion] = useState('');

    // Load rules for this profile
    useEffect(() => {
        let isMounted = true;
        const loadData = async () => {
            setLoadingRule(true);
            const result = await api.getRules(scenario);
            if (isMounted) {
                setRules(result.success ? result.data : []);
                setLoadingRule(false);
            }
        };
        loadData();
        return () => { isMounted = false; };
    }, [scenario]);

    // Load app version for npm command
    useEffect(() => {
        api.getVersion().then(setAppVersion);
    }, []);

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
                navigate('/use-claude-code');
            } else {
                showNotification(`${t('claudeCode.profile.deleteFailed')}: ${result.error || 'Unknown error'}`, 'error');
            }
        } catch {
            showNotification(t('claudeCode.profile.deleteFailed'), 'error');
        } finally {
            setIsProfileMutating(false);
        }
    };

    const ccCommand = npmMode && appVersion
        ? `npx -y tingly-box@${appVersion} cc --profile ${profileId}`
        : `tingly-box cc --profile ${profileId}`;

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
                        <Chip label={currentProfile ? `${profileId} - ${currentProfile.name}` : profileId} size="small" variant="outlined" />
                    }
                >
                    <Box sx={{ px: 2, pb: 1.5 }}>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, maxWidth: 700 }}>
                            <Typography
                                variant="subtitle2"
                                color="text.secondary"
                                sx={{ minWidth: 190, flexShrink: 0, fontWeight: 500 }}
                            >
                                {t('claudeCode.profile.quickStart')}
                            </Typography>
                            <Typography
                                variant="subtitle2"
                                onClick={() => copyToClipboard(ccCommand, 'command')}
                                sx={{
                                    fontFamily: 'monospace',
                                    fontSize: '0.75rem',
                                    color: 'primary.main',
                                    flex: 1,
                                    minWidth: 0,
                                    cursor: 'pointer',
                                    '&:hover': {
                                        textDecoration: 'underline',
                                        backgroundColor: 'action.hover'
                                    },
                                    padding: 1,
                                    borderRadius: 1,
                                    transition: 'all 0.2s ease-in-out'
                                }}
                                title={t('claudeCode.profile.clickToCopy')}
                            >
                                {ccCommand}
                            </Typography>
                            <Stack direction="row" spacing={0.5} sx={{ flexShrink: 0, ml: 'auto' }}>
                                <Tooltip title={npmMode ? t('claudeCode.profile.switchToGlobal') : t('claudeCode.profile.switchToNpm')}>
                                    <IconButton
                                        onClick={() => setNpmMode(!npmMode)}
                                        size="small"
                                        color={npmMode ? "primary" : "default"}
                                        sx={{ position: 'relative' }}
                                    >
                                        {npmMode ? (
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
                                        ) : (
                                            <CodeIcon fontSize="small" />
                                        )}
                                    </IconButton>
                                </Tooltip>
                                <Tooltip title={t('claudeCode.profile.copyCommand')}>
                                    <IconButton size="small" onClick={() => copyToClipboard(ccCommand, 'command')}>
                                        <ContentCopyIcon fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                            </Stack>
                        </Box>
                    </Box>
                    <Divider sx={{ mx: 2 }} />
                    <ProviderConfigCard
                        title={`Claude Code [${profileId}]`}
                        baseUrlPath={`/tingly/${scenario}`}
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                        token={token}
                        onShowTokenModal={() => setShowTokenModal(true)}
                        scenario={scenario}
                        showApiKeyRow={true}
                        showBaseUrlRow={true}
                    />
                    <Divider sx={{ mx: 2, mt: 1.5 }} />
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

export default ClaudeCodeProfilePage;

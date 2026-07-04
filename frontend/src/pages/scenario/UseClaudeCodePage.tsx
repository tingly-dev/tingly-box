import CardGrid from "@/components/CardGrid.tsx";
import AgentSetupCard, { type AgentApplyResult, hasModelOnAnyRule, scrollToModelsCard } from './components/AgentSetupCard';
import ClaudeCodeConfigModal from './components/ClaudeCodeConfigModal';
import ConnectProviderFlow from '@/components/ConnectProviderFlow';
import { derivePrefsFromRules } from './components/ClaudeCodeQuickConfig';
import type { ClaudeCodeDefaultMode } from './components/ClaudeCodeQuickConfig';
import PageLayout from '@/components/PageLayout';
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import TemplatePage from './components/TemplatePage.tsx';
import UnifiedCard from "@/components/UnifiedCard.tsx";
import {useScenarioPageInternal} from '@/pages/scenario/hooks/useScenarioPageInternal.ts';
import {api} from '@/services/api';
import {toggleButtonGroupStyle, toggleButtonStyle} from "@/styles/toggleStyles";
import { Info as InfoIcon, Refresh as RestartIcon } from '@/components/icons';
import {
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    ToggleButton,
    ToggleButtonGroup,
    Tooltip,
    Typography,
    Alert
} from '@mui/material';
import React, {useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import { ScenarioPageModalProvider } from '@/pages/scenario/context/ScenarioPageContext';

type ConfigMode = 'unified' | 'separate' | 'smart';

const CONFIG_MODES: { value: ConfigMode; label: string; description: string; enabled: boolean }[] = [
    { value: 'unified', label: 'Unified Model', description: 'Config unified model for all claude code requests', enabled: true },
    { value: 'separate', label: 'Separate Model', description: 'Config different models for claude code scenario, like subagent, summary, default, ...', enabled: true },
    { value: 'smart', label: 'Smart', description: '(WIP) Smart routing according to request field / content / model feature / user intent / ...', enabled: false },
];

const SCENARIO = 'claude_code';

const UseClaudeCodePageContent: React.FC = () => {
    const { t } = useTranslation();

    const {
        showNotification,
        notification,
        copyToClipboard,
        baseUrl,
    } = useScenarioPageInternal(SCENARIO);

    // Custom state for this page
    const [rules, setRules] = useState<any[]>([]);
    const [loadingRule, setLoadingRule] = useState(true);
    const [configMode, setConfigMode] = useState<ConfigMode>('unified');
    const [pendingMode, setPendingMode] = useState<ConfigMode | null>(null);
    const [confirmDialogOpen, setConfirmDialogOpen] = useState(false);
    const [isApplyLoading, setIsApplyLoading] = useState(false);
    const [configModalOpen, setConfigModalOpen] = useState(false);
    const [connectProviderOpen, setConnectProviderOpen] = useState(false);
    const [pendingContext1MChange, setPendingContext1MChange] = useState<{ enabled: boolean; ruleUuid?: string } | null>(null);

    const handleContext1MToggle = (newState: boolean, ruleUuid?: string) => {
        // Store the pending change (scoped to the toggled rule) and open the
        // config panel so the user can apply the matching env update.
        setPendingContext1MChange({ enabled: newState, ruleUuid });
        setConfigModalOpen(true);
    };

    // Load scenario config to get config mode
    const loadScenarioConfig = async () => {
        try {
            const result = await api.getScenarioConfig(SCENARIO);
            if (result.success && result.data && result.data.flags) {
                const { separate } = result.data.flags;
                if (separate) {
                    setConfigMode('separate');
                } else {
                    setConfigMode('unified');
                }
            }
        } catch (error) {
            console.error('Failed to load scenario config:', error);
        }
    };

    // Handle config mode change - show confirmation dialog first
    const handleConfigModeChange = (newMode: ConfigMode) => {
        if (newMode === configMode) return;
        setPendingMode(newMode);
        setConfirmDialogOpen(true);
    };

    // Confirm mode change
    const confirmModeChange = async () => {
        if (!pendingMode) return;

        setConfirmDialogOpen(false);
        try {
            // GET-merge: SetScenarioConfig replaces the record wholesale,
            // so a partial payload silently wipes extensions (e.g. vision_proxy_service).
            const current = (await api.getScenarioConfig(SCENARIO))?.data || {};
            const config = {
                ...current,
                scenario: SCENARIO,
                flags: {
                    ...(current.flags || {}),
                    unified: pendingMode === 'unified',
                    separate: pendingMode === 'separate',
                    smart: false,
                },
            };
            const result = await api.setScenarioConfig(SCENARIO, config);

            if (result.success) {
                setConfigMode(pendingMode);

                showNotification(
                    `Configuration mode changed to ${pendingMode}. Please reapply the configuration to Claude Code.`,
                    'success'
                );
            } else {
                showNotification('Failed to save configuration mode', 'error');
            }
        } catch (error) {
            console.error('Failed to save scenario config:', error);
            showNotification('Failed to save configuration mode', 'error');
        } finally {
            setPendingMode(null);
        }
    };

    // Cancel mode change
    const cancelModeChange = () => {
        setConfirmDialogOpen(false);
        setPendingMode(null);
    };

    useEffect(() => {
        let isMounted = true;

        const loadDataAsync = async () => {
            setLoadingRule(true);
            if (configMode === 'unified') {
                const result = await api.getRule("builtin:claude_code:cc");
                if (isMounted) {
                    setRules(result.success ? [result.data] : []);
                    setLoadingRule(false);
                }
            } else {
                const result = await api.getRules(SCENARIO);
                if (isMounted) {
                    // Filter out the unified rule in separate mode
                    const filtered = (result.success ? result.data : []).filter((r: any) => r.uuid !== 'builtin:claude_code:cc');
                    setRules(filtered);
                    setLoadingRule(false);
                }
            }
        };

        loadDataAsync();

        return () => {
            isMounted = false;
        };
    }, [configMode]);

    useEffect(() => {
        loadScenarioConfig();
    }, []);

    // Normalize the backend's ApplyConfigResponse into the AgentApplyResult
    // shape consumed by both the AgentSetupCard and the modal. The richer
    // fields (created/updated/backup) let the modal show a detailed alert
    // while AgentSetupCard keeps using the flat `files` list.
    const applyPrefs = async (
        prefs: Record<string, string>,
        installStatusLine: boolean,
        defaultMode: ClaudeCodeDefaultMode = 'acceptEdits',
    ): Promise<AgentApplyResult> => {
        try {
            setIsApplyLoading(true);
            const result = await api.applyClaudeConfig(prefs, installStatusLine, defaultMode);
            if (result?.success) {
                const created = result.createdFiles || [];
                const updated = result.updatedFiles || [];
                return {
                    success: true,
                    files: [...created, ...updated],
                    createdFiles: created,
                    updatedFiles: updated,
                    backupPaths: result.backupPaths || [],
                };
            }
            return { success: false, error: result?.message || 'Apply failed' };
        } catch (e: any) {
            return { success: false, error: e?.message || 'Failed to apply configurations' };
        } finally {
            setIsApplyLoading(false);
        }
    };

    // AgentSetupCard's one-click apply uses prefs derived from the current
    // rules + configMode — same defaults the modal would seed with.
    const handleApply = () =>
        applyPrefs(derivePrefsFromRules({ rules, mode: configMode }) as Record<string, string>, false);
    const handleApplyWithStatusLine = () =>
        applyPrefs(derivePrefsFromRules({ rules, mode: configMode }) as Record<string, string>, true);

    return (
        <PageLayout loading={loadingRule} notification={notification}>
            <CardGrid>
                <UnifiedCard
                    title={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flex: 1 }}>
                            <span>Claude Code</span>
                            <Tooltip title="AI-powered CLI development agent for implementation, testing, and git operations">
                                <IconButton size="small" sx={{ ml: 0.5 }}>
                                    <InfoIcon fontSize="small" sx={{ color: 'text.secondary' }} />
                                </IconButton>
                            </Tooltip>
                        </Box>
                    }
                    size="full"
                    rightAction={
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                            <ToggleButtonGroup
                                value={configMode}
                                exclusive
                                size="small"
                                onChange={(_, value) => value && handleConfigModeChange(value)}
                                sx={toggleButtonGroupStyle}
                            >
                                {CONFIG_MODES.filter(m => m.enabled).map((mode) => (
                                    <Tooltip key={mode.value} title={mode.description} arrow>
                                        <ToggleButton
                                            value={mode.value}
                                            sx={toggleButtonStyle}
                                        >
                                            {mode.label}
                                        </ToggleButton>
                                    </Tooltip>
                                ))}
                            </ToggleButtonGroup>
                            <Button
                                onClick={() => setConfigModalOpen(true)}
                                variant="contained"
                                color="primary"
                                size="small"
                            >
                                {t('claudeCode.configButton')}
                            </Button>
                        </Box>
                    }
                >
                    <ProviderConfigCard
                        title="Claude Code"
                        baseUrlPath={`/tingly/${SCENARIO}`}
                        baseUrl={baseUrl}
                        onCopy={copyToClipboard}
                        scenario={SCENARIO}
                        showApiKeyRow={true}
                        showBaseUrlRow={true}
                        compact={true}
                    />
                </UnifiedCard>

                <AgentSetupCard
                    agentKey={SCENARIO}
                    agentName="Claude Code"
                    installCommand="npm install -g @anthropic-ai/claude-code"
                    installMirrorCommand="npm install -g @anthropic-ai/claude-code --registry=https://registry.npmmirror.com"
                    onApply={handleApply}
                    onApplyWithStatusLine={handleApplyWithStatusLine}
                    isApplyLoading={isApplyLoading}
                    onViewConfig={() => setConfigModalOpen(true)}
                    hasModelSelected={hasModelOnAnyRule(rules)}
                    onSelectModel={scrollToModelsCard}
                    onConnectProvider={() => setConnectProviderOpen(true)}
                />

                <TemplatePage
                    scenario={SCENARIO}
                    rules={rules}
                    onRulesChange={setRules}
                    collapsible={true}
                    allowToggleRule={false}
                    allowAddRule={false}
                    onContext1MToggle={handleContext1MToggle}
                />

                <Dialog
                    open={confirmDialogOpen}
                    onClose={cancelModeChange}
                    maxWidth="sm"
                    fullWidth
                >
                    <DialogTitle>Change Configuration Mode?</DialogTitle>
                    <DialogContent>
                        <Typography variant="body1" sx={{ mb: 1 }}>
                            You are about to switch from <strong>{configMode}</strong> to <strong>{pendingMode}</strong> mode.
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            After changing the mode, you will need to reapply the configuration to Claude Code for the changes to take effect.
                        </Typography>
                    </DialogContent>
                    <DialogActions sx={{ px: 3, pb: 2, gap: 1, justifyContent: 'flex-end' }}>
                        <Button onClick={cancelModeChange} color="inherit">
                            Cancel
                        </Button>
                        <Button onClick={confirmModeChange} variant="contained">
                            Confirm
                        </Button>
                    </DialogActions>
                </Dialog>

                <ClaudeCodeConfigModal
                    open={configModalOpen}
                    onClose={() => {
                        setConfigModalOpen(false);
                        setPendingContext1MChange(null);
                    }}
                    configMode={configMode}
                    baseUrl={baseUrl}
                    rules={rules}
                    copyToClipboard={copyToClipboard}
                    onApplyWithPrefs={(prefs, installStatusLine, defaultMode) =>
                        applyPrefs(prefs as Record<string, string>, installStatusLine, defaultMode)
                    }
                    isApplyLoading={isApplyLoading}
                    pendingContext1MChange={pendingContext1MChange}
                />

                <ConnectProviderFlow
                    open={connectProviderOpen}
                    onClose={() => setConnectProviderOpen(false)}
                    showNotification={showNotification}
                    onProviderAdded={() => window.location.reload()}
                />

            </CardGrid>
        </PageLayout>
    );
};

const UseClaudeCodePage: React.FC = () => {
    return (
        <ScenarioPageModalProvider>
            <UseClaudeCodePageContent />
        </ScenarioPageModalProvider>
    );
};

export default UseClaudeCodePage;

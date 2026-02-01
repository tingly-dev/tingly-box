import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ExperimentalFeatures from "@/components/ExperimentalFeatures.tsx";
import InfoIcon from '@mui/icons-material/Info';
import { Box, Button, Divider, Tooltip, IconButton, Typography } from '@mui/material';
import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import EmptyStateGuide from '@/components/EmptyStateGuide';
import PageLayout from '@/components/PageLayout';
import TemplatePage from '@/components/TemplatePage.tsx';
import OpenCodeConfigModal from '@/components/OpenCodeConfigModal';
import { useFunctionPanelData } from '../hooks/useFunctionPanelData';
import { api, getBaseUrl } from '../services/api';

const scenario = "opencode";

const UseOpenCodePage: React.FC = () => {
    const {
        showTokenModal,
        setShowTokenModal,
        token,
        showNotification,
        providers,
        loading: providersLoading,
    } = useFunctionPanelData();
    const [baseUrl, setBaseUrl] = useState<string>('');
    const [rules, setRules] = useState<any[]>([]);
    const [loadingRule, setLoadingRule] = useState(true);
    const [newlyCreatedRuleUuids, setNewlyCreatedRuleUuids] = useState<Set<string>>(new Set());
    const [configModalOpen, setConfigModalOpen] = useState(false);
    const [isApplyLoading, setIsApplyLoading] = useState(false);
    // Config preview state
    const [configJson, setConfigJson] = useState('');
    const [scriptWindows, setScriptWindows] = useState('');
    const [scriptUnix, setScriptUnix] = useState('');
    const [isConfigLoading, setIsConfigLoading] = useState(false);
    const navigate = useNavigate();

    const handleAddOAuthClick = () => {
        navigate('/oauth?dialog=add');
    };

    const copyToClipboard = async (text: string, label: string) => {
        try {
            await navigator.clipboard.writeText(text);
            showNotification(`${label} copied to clipboard!`, 'success');
        } catch (err) {
            showNotification('Failed to copy to clipboard', 'error');
        }
    };

    const handleRuleDelete = useCallback((deletedRuleUuid: string) => {
        setRules((prevRules) => prevRules.filter(r => r.uuid !== deletedRuleUuid));
    }, []);

    const handleRulesChange = useCallback((updatedRules: any[]) => {
        setRules(updatedRules);
        // If a new rule was added (length increased), add it to newlyCreatedRuleUuids
        if (updatedRules.length > rules.length) {
            const newRule = updatedRules[updatedRules.length - 1];
            setNewlyCreatedRuleUuids(prev => new Set(prev).add(newRule.uuid));
        }
    }, [rules.length]);

    // Fetch OpenCode config preview from backend
    const fetchConfigPreview = async () => {
        setIsConfigLoading(true);
        try {
            const result = await api.getOpenCodeConfigPreview();
            if (result.success) {
                setConfigJson(result.configJson);
                setScriptWindows(result.scriptWindows);
                setScriptUnix(result.scriptUnix);
            } else {
                setConfigJson('// Error: ' + (result.message || 'Failed to load config'));
                setScriptWindows('// Error loading config');
                setScriptUnix('// Error loading config');
                showNotification('Failed to load config preview: ' + (result.message || 'Unknown error'), 'error');
            }
        } catch (err) {
            console.error('Failed to fetch config preview:', err);
            setConfigJson('// Error: Failed to connect to server');
            setScriptWindows('// Error: Failed to connect to server');
            setScriptUnix('// Error: Failed to connect to server');
            showNotification('Failed to load config preview', 'error');
        } finally {
            setIsConfigLoading(false);
        }
    };

    // Handle opening config modal - fetch preview first
    const handleOpenConfigModal = async () => {
        // Reset config state
        setConfigJson('// Loading...');
        setScriptWindows('// Loading...');
        setScriptUnix('// Loading...');
        await fetchConfigPreview();
        setConfigModalOpen(true);
    };

    useEffect(() => {
        let isMounted = true;

        const loadDataAsync = async () => {
            const url = await getBaseUrl();
            if (isMounted) setBaseUrl(url);

            const result = await api.getRules(scenario);
            if (isMounted) {
                if (result.success) {
                    const ruleData = result.data;
                    setRules(ruleData);
                }
                setLoadingRule(false);
            }
        };

        loadDataAsync();

        return () => {
            isMounted = false;
        };
    }, []);

    // Apply handler for OpenCode config - calls backend to generate and write config
    const handleApply = async () => {
        try {
            setIsApplyLoading(true);
            const result = await api.applyOpenCodeConfig();

            if (result.success) {
                const configPath = '~/.config/opencode/opencode.json';
                let successMsg = `Configuration file written: ${configPath}`;
                if (result.created) {
                    successMsg += ' (created)';
                } else if (result.updated) {
                    successMsg += ' (updated)';
                }
                if (result.backupPath) {
                    successMsg += `\nBackup created: ${result.backupPath}`;
                }
                showNotification(successMsg, 'success');
            } else {
                showNotification(`Failed to apply opencode.json: ${result.message || 'Unknown error'}`, 'error');
            }
        } catch (err) {
            showNotification('Failed to apply OpenCode config', 'error');
        } finally {
            setIsApplyLoading(false);
        }
    };

    const header = null;

    const isLoading = providersLoading || loadingRule;

    return (
        <PageLayout loading={isLoading}>
            {!providers.length ? (
                <CardGrid>
                    <UnifiedCard
                        title={
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                <span>OpenCode SDK Configuration</span>
                            </Box>
                        }
                        size="full"
                    >
                        <EmptyStateGuide
                            title="No Providers Configured"
                            description="Add an API key or OAuth provider to get started"
                            onAddApiKeyClick={() => navigate('/api-keys?dialog=add')}
                            onAddOAuthClick={handleAddOAuthClick}
                        />
                    </UnifiedCard>
                </CardGrid>
            ) : (
                <CardGrid>
                    <UnifiedCard
                        title={
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                <span>OpenCode SDK Configuration</span>
                                <Tooltip title={`Base URL: ${baseUrl}/tingly/opencode`}>
                                    <IconButton size="small" sx={{ ml: 0.5 }}>
                                        <InfoIcon fontSize="small" sx={{ color: 'text.secondary' }} />
                                    </IconButton>
                                </Tooltip>
                            </Box>
                        }
                        size="full"
                        rightAction={
                            <Button
                                onClick={handleOpenConfigModal}
                                variant="contained"
                                size="small"
                            >
                                Config OpenCode
                            </Button>
                        }
                    >
                        {header}

                        <Divider></Divider>
                        
                        {/* Experimental Features - collapsible section */}
                        <ExperimentalFeatures scenario="opencode" />
                    </UnifiedCard>

                    <TemplatePage
                        title="Models and Forwarding Rules"
                        scenario={scenario}
                        rules={rules}
                        collapsible={true}
                        showTokenModal={showTokenModal}
                        setShowTokenModal={setShowTokenModal}
                        token={token}
                        showNotification={showNotification}
                        providers={providers}
                        onRulesChange={handleRulesChange}
                        newlyCreatedRuleUuids={newlyCreatedRuleUuids}
                        allowDeleteRule={true}
                        onRuleDelete={handleRuleDelete}
                        showAddApiKeyButton={false}
                    />

                    {/* OpenCode Config Modal */}
                    <OpenCodeConfigModal
                        open={configModalOpen}
                        onClose={() => setConfigModalOpen(false)}
                        generateConfigJson={() => configJson}
                        generateScriptWindows={() => scriptWindows}
                        generateScriptUnix={() => scriptUnix}
                        copyToClipboard={copyToClipboard}
                        onApply={handleApply}
                        isApplyLoading={isApplyLoading}
                        isLoading={isConfigLoading}
                    />
                </CardGrid>
            )}
        </PageLayout>
    );
};

export default UseOpenCodePage;

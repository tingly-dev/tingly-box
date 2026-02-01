import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ExperimentalFeatures from "@/components/ExperimentalFeatures.tsx";
import { ContentCopy as CopyIcon } from '@mui/icons-material';
import VisibilityIcon from '@mui/icons-material/Visibility';
import { Box, Button, IconButton, Tooltip } from '@mui/material';
import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { v4 as uuidv4 } from 'uuid';
import { ApiConfigRow } from '@/components/ApiConfigRow';
import { BaseUrlRow } from '@/components/BaseUrlRow';
import EmptyStateGuide from '@/components/EmptyStateGuide';
import PageLayout from '@/components/PageLayout';
import TemplatePage from '@/components/TemplatePage.tsx';
import OpenCodeConfigModal from '@/components/OpenCodeConfigModal';
import { useFunctionPanelData } from '../hooks/useFunctionPanelData';
import { api, getBaseUrl } from '../services/api';

const ruleId = "built-in-opencode";
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
    const [baseUrl, setBaseUrl] = React.useState<string>('');
    const [rules, setRules] = React.useState<any[]>([]);
    const [loadingRule, setLoadingRule] = React.useState(true);
    const [newlyCreatedRuleUuids, setNewlyCreatedRuleUuids] = React.useState<Set<string>>(new Set());
    const [configModalOpen, setConfigModalOpen] = React.useState(false);
    const [isApplyLoading, setIsApplyLoading] = React.useState(false);
    // Config preview state
    const [configJson, setConfigJson] = React.useState('');
    const [scriptWindows, setScriptWindows] = React.useState('');
    const [scriptUnix, setScriptUnix] = React.useState('');
    const [isConfigLoading, setIsConfigLoading] = React.useState(false);
    const navigate = useNavigate();

    const handleAddApiKeyClick = () => {
        navigate('/api-keys?dialog=add');
    };

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

    const handleCreateRule = async () => {
        try {
            const newRuleData = {
                scenario: scenario,
                request_model: `model-${uuidv4().slice(0, 8)}`,
                response_model: '',
                active: true,
                services: []
            };
            const result = await api.createRule('', newRuleData);
            if (result.success && result.data?.uuid) {
                // Add the new rule UUID to the set so it auto-expands and scrolls into view
                setNewlyCreatedRuleUuids(prev => new Set(prev).add(result.data.uuid));
                showNotification('Routing rule created successfully!', 'success');
                // Reload rules
                const rulesResult = await api.getRules(scenario);
                if (rulesResult.success) {
                    setRules(rulesResult.data);
                }
            } else {
                showNotification(`Failed to create rule: ${result.error || 'Unknown error'}`, 'error');
            }
        } catch (error) {
            console.error('Error creating rule:', error);
            showNotification('Failed to create routing rule', 'error');
        }
    };

    const handleRuleDelete = useCallback((deletedRuleUuid: string) => {
        setRules((prevRules) => prevRules.filter(r => r.uuid !== deletedRuleUuid));
    }, []);

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

    // Get the default request model from the built-in rule
    const getRequestModel = () => {
        const builtInRule = rules.find(r => r.uuid === 'built-in-opencode');
        return builtInRule?.request_model || 'tingly-opencode';
    };

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

    const header = (
        <Box sx={{ p: 2 }}>
            <BaseUrlRow
                label="Base URL"
                path="/tingly/opencode"
                baseUrl={baseUrl}
                urlLabel="OpenCode Base URL"
                onCopy={(url) => copyToClipboard(url, 'OpenCode Base URL')}
            />
            <ApiConfigRow label="API Key" showEllipsis={true}>
                <Box sx={{ display: 'flex', gap: 0.5, ml: 'auto' }}>
                    <Tooltip title="View Token">
                        <IconButton onClick={() => setShowTokenModal(true)} size="small">
                            <VisibilityIcon />
                        </IconButton>
                    </Tooltip>
                    <Tooltip title="Copy Token">
                        <IconButton onClick={() => copyToClipboard(token, 'API Key')} size="small">
                            <CopyIcon fontSize="small" />
                        </IconButton>
                    </Tooltip>
                </Box>
            </ApiConfigRow>
        </Box>
    );

    const isLoading = providersLoading || loadingRule;

    return (
        <PageLayout loading={isLoading}>
            {!providers.length ? (
                <CardGrid>
                    <UnifiedCard title="OpenCode SDK Configuration" size="full">
                        <EmptyStateGuide
                            title="No Providers Configured"
                            description="Add an API key or OAuth provider to get started"
                            onAddApiKeyClick={handleAddApiKeyClick}
                            onAddOAuthClick={handleAddOAuthClick}
                        />
                    </UnifiedCard>
                </CardGrid>
            ) : (
                <CardGrid>
                    <UnifiedCard
                        title="OpenCode SDK Configuration"
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

                        {/* Experimental Features - collapsible section */}
                        <ExperimentalFeatures scenario="opencode" />
                    </UnifiedCard>

                    <TemplatePage
                        title={
                            <Tooltip title="Use as model name in your API requests to forward">
                                Models and Forwarding Rules
                            </Tooltip>
                        }
                        scenario={scenario}
                        rules={rules}
                        collapsible={true}
                        showTokenModal={showTokenModal}
                        setShowTokenModal={setShowTokenModal}
                        token={token}
                        showNotification={showNotification}
                        providers={providers}
                        onRulesChange={setRules}
                        newlyCreatedRuleUuids={newlyCreatedRuleUuids}
                        allowDeleteRule={true}
                        onRuleDelete={handleRuleDelete}
                        onCreateRule={handleCreateRule}
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

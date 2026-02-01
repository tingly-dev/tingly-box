import React, { useCallback, useEffect, useState } from 'react';
import { ContentCopy as CopyIcon } from '@mui/icons-material';
import VisibilityIcon from '@mui/icons-material/Visibility';
import { Box, IconButton, Tooltip } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import { BaseUrlRow } from '@/components/BaseUrlRow';
import TemplatePage from '@/components/TemplatePage.tsx';
import PageLayout from '@/components/PageLayout';
import { api, getBaseUrl } from '../services/api';
import { ApiConfigRow } from "@/components/ApiConfigRow";
import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import { useFunctionPanelData } from '../hooks/useFunctionPanelData';
import EmptyStateGuide from '@/components/EmptyStateGuide';

const scenario = "openai";

const UseOpenAIPage: React.FC = () => {
    const {
        showTokenModal,
        setShowTokenModal,
        token,
        showNotification,
        providers,
        loading: providersLoading,
        notification,
    } = useFunctionPanelData();
    const [baseUrl, setBaseUrl] = useState<string>('');
    const [rules, setRules] = useState<any[]>([]);
    const [loadingRule, setLoadingRule] = useState(true);
    const [newlyCreatedRuleUuids, setNewlyCreatedRuleUuids] = useState<Set<string>>(new Set());
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

    useEffect(() => {
        let isMounted = true;

        const loadDataAsync = async () => {
            const url = await getBaseUrl();
            if (isMounted) setBaseUrl(url);

            const result = await api.getRules(scenario);
            if (isMounted) {
                if (result.success) {
                    const ruleData = Array.isArray(result.data) ? result.data : [];
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

    const header = (
        <Box sx={{ p: 2 }}>
            <BaseUrlRow
                label="Base URL"
                path="/tingly/openai"
                baseUrl={baseUrl}
                onCopy={(url) => copyToClipboard(url, 'OpenAI Base URL')}
                urlLabel="OpenAI Base URL"
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
        <PageLayout loading={isLoading} notification={notification}>
            {!providers.length ? (
                <CardGrid>
                    <UnifiedCard title="OpenAI SDK Configuration" size="full">
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
                    <UnifiedCard title="OpenAI SDK Configuration" size="full">
                        {header}
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
                    />
                </CardGrid>
            )}
        </PageLayout>
    );
};

export default UseOpenAIPage;

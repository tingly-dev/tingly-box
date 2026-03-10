import CardGrid from "@/components/CardGrid.tsx";
import CodexConfigModal from "@/components/CodexConfigModal.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import ProviderConfigCard from "@/components/ProviderConfigCard.tsx";
import { Box, Button, IconButton, Tooltip } from '@mui/material';
import InfoIcon from '@mui/icons-material/Info';
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import EmptyStateGuide from '@/components/EmptyStateGuide';
import PageLayout from '@/components/PageLayout';
import TemplatePage from './components/TemplatePage.tsx';
import { useFunctionPanelData } from '@/hooks/useFunctionPanelData';
import { useHeaderHeight } from '@/hooks/useHeaderHeight';
import { api, getBaseUrl } from '@/services/api';

const scenario = "codex";

const UseCodexPage: React.FC = () => {
    const {
        showTokenModal,
        setShowTokenModal,
        token,
        showNotification,
        providers,
        loading: providersLoading,
        notification,
    } = useFunctionPanelData();
    const headerRef = useRef<HTMLDivElement>(null);
    const [baseUrl, setBaseUrl] = useState<string>('');
    const [rules, setRules] = useState<any[]>([]);
    const [loadingRule, setLoadingRule] = useState(true);
    const [newlyCreatedRuleUuids, setNewlyCreatedRuleUuids] = useState<Set<string>>(new Set());
    const [configModalOpen, setConfigModalOpen] = useState(false);
    const navigate = useNavigate();

    const headerHeight = useHeaderHeight(
        headerRef,
        providers.length > 0,
        []
    );

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

    const handleOpenConfigModal = () => {
        setConfigModalOpen(true);
    };

    const handleRuleDelete = useCallback((deletedRuleUuid: string) => {
        setRules((prevRules) => prevRules.filter(r => r.uuid !== deletedRuleUuid));
    }, []);

    const handleRulesChange = useCallback((updatedRules: any[]) => {
        setRules(updatedRules);
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

    const isLoading = providersLoading || loadingRule;

    return (
        <PageLayout loading={isLoading} notification={notification}>
            {!providers.length ? (
                <CardGrid>
                    <UnifiedCard title="Codex SDK Configuration" size="full">
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
                                <span>Codex SDK Configuration</span>
                                <Tooltip title={`Base URL: ${baseUrl}/tingly/codex`}>
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
                                Config Codex
                            </Button>
                        }
                    >
                        <ProviderConfigCard
                            headerRef={headerRef}
                            title="Codex SDK Configuration"
                            baseUrlPath="/tingly/codex"
                            baseUrl={baseUrl}
                            onCopy={copyToClipboard}
                            token={token}
                            onShowTokenModal={() => setShowTokenModal(true)}
                            scenario={scenario}
                        />
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
                        headerHeight={headerHeight}
                    />

                    <CodexConfigModal
                        open={configModalOpen}
                        onClose={() => setConfigModalOpen(false)}
                        baseUrl={baseUrl}
                        token={token}
                        copyToClipboard={copyToClipboard}
                    />
                </CardGrid>
            )}
        </PageLayout>
    );
};

export default UseCodexPage;

import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import { ContentCopy as CopyIcon } from '@mui/icons-material';
import VisibilityIcon from '@mui/icons-material/Visibility';
import { Box, IconButton, Tooltip } from '@mui/material';
import React, { useCallback, useEffect, useState, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { ApiConfigRow } from '@/components/ApiConfigRow';
import { BaseUrlRow } from '@/components/BaseUrlRow';
import EmptyStateGuide from '@/components/EmptyStateGuide';
import PageLayout from '@/components/PageLayout';
import TemplatePage from '@/components/TemplatePage.tsx';
import { useFunctionPanelData } from '../hooks/useFunctionPanelData';
import { api, getBaseUrl } from '../services/api';

const scenario = "anthropic";

const UseAnthropicPage: React.FC = () => {
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
    const [headerHeight, setHeaderHeight] = useState<number>(0);
    const [baseUrl, setBaseUrl] = useState<string>('');
    const [rules, setRules] = useState<any[]>([]);
    const [loadingRule, setLoadingRule] = useState(true);
    const [newlyCreatedRuleUuids, setNewlyCreatedRuleUuids] = useState<Set<string>>(new Set());
    const navigate = useNavigate();

    // Measure header height
    useEffect(() => {
        if (providers.length === 0) {
            return;
        }

        const timeoutId = setTimeout(() => {
            if (!headerRef.current) {
                return;
            }

            const updateHeight = () => {
                if (headerRef.current) {
                    const height = headerRef.current.offsetHeight || 0;
                    setHeaderHeight(height);
                }
            };

            updateHeight();

            const resizeObserver = new ResizeObserver(() => {
                updateHeight();
            });

            resizeObserver.observe(headerRef.current);

            return () => {
                resizeObserver.disconnect();
            };
        }, 200);

        return () => {
            clearTimeout(timeoutId);
        };
    }, [providers.length]);

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

    const header = (
        <Box sx={{ p: 2 }}>
            <BaseUrlRow
                label="Base URL"
                path="/tingly/anthropic"
                baseUrl={baseUrl}
                urlLabel="Anthropic Base URL"
                onCopy={(url) => copyToClipboard(url, 'Anthropic Base URL')}
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
                    <UnifiedCard title="Anthropic SDK Configuration" size="full">
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
                    <UnifiedCard ref={headerRef} title="Anthropic SDK Configuration" size="full">
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
                        headerHeight={headerHeight}
                    />
                </CardGrid>
            )}
        </PageLayout>
    );
};

export default UseAnthropicPage;

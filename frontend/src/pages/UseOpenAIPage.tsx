import React, { useCallback } from 'react';
import {Add as AddIcon, ContentCopy as CopyIcon, Edit as EditIcon, Key as KeyIcon} from '@mui/icons-material';
import VisibilityIcon from '@mui/icons-material/Visibility';
import {Box, Button, IconButton, Stack, Tooltip, Typography} from '@mui/material';
import {useNavigate} from 'react-router-dom';
import {BaseUrlRow} from '../components/BaseUrlRow';
import TemplatePage from '../components/TemplatePage.tsx';
import PageLayout from '../components/PageLayout';
import {api, getBaseUrl} from '../services/api';
import type { Provider } from '../types/provider';
import { ApiConfigRow } from "@/components/ApiConfigRow";
import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";
import { useFunctionPanelData } from '../hooks/useFunctionPanelData';
import { useProviderDialog } from '../hooks/useProviderDialog';
import EmptyStateGuide from '../components/EmptyStateGuide';
import ProviderFormDialog from '../components/ProviderFormDialog';
import OAuthDialog from '../components/OAuthDialog';
import { v4 as uuidv4 } from 'uuid';

const ruleId = "built-in-openai";
const scenario = "openai";

const UseOpenAIPage: React.FC = () => {
    const {
        showTokenModal,
        setShowTokenModal,
        token,
        showNotification,
        providers,
    } = useFunctionPanelData();
    const [baseUrl, setBaseUrl] = React.useState<string>('');
    const [rules, setRules] = React.useState<any>(null);
    const [loadingRule, setLoadingRule] = React.useState(true);
    const [newlyCreatedRuleUuids, setNewlyCreatedRuleUuids] = React.useState<Set<string>>(new Set());
    const navigate = useNavigate();

    // Provider dialog hook
    const providerDialog = useProviderDialog(showNotification, {
        defaultApiStyle: 'openai',
        onProviderAdded: () => window.location.reload(),
    });

    // OAuth dialog state
    const [oauthDialogOpen, setOAuthDialogOpen] = React.useState(false);

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
                // Add the new rule UUID to the set so it auto-expands
                setNewlyCreatedRuleUuids(prev => new Set(prev).add(result.data.uuid));
                showNotification('Routing rule created successfully!', 'success');
                loadData(); // Reload the rules list
            } else {
                showNotification(`Failed to create rule: ${result.error || 'Unknown error'}`, 'error');
            }
        } catch (error) {
            console.error('Error creating rule:', error);
            showNotification('Failed to create routing rule', 'error');
        }
    };

    const handleRuleDelete = useCallback((deletedRuleUuid: string) => {
        setRules((prevRules: any[]) => (prevRules || []).filter(r => r.uuid !== deletedRuleUuid));
    }, []);

    const loadData = async () => {
        const url = await getBaseUrl();
        setBaseUrl(url);

        // Fetch rule information
        const result = await api.getRules(scenario);
        console.log('getRules result:', result);
        if (result.success) {
            // getRules returns an array, we need the first item or filter by ruleId
            const ruleData = Array.isArray(result.data)
                ? result.data : [];
            console.log('ruleData:', ruleData);
            setRules(ruleData);
        }
        setLoadingRule(false);
    };

    React.useEffect(() => {
        loadData();
    }, []);

    // const modelName = rules?.request_model;

    const header = (
        <Box sx={{p: 2}}>
            <BaseUrlRow
                label="Base URL"
                path="/tingly/openai"
                // legacyLabel="(Legacy) Base URL"
                // legacyPath="/openai"
                baseUrl={baseUrl}
                onCopy={(url) => copyToClipboard(url, 'OpenAI Base URL')}
                urlLabel="OpenAI Base URL"
            />
            <ApiConfigRow label="API Key" showEllipsis={true}>
                <Box sx={{display: 'flex', gap: 0.5, ml: 'auto'}}>
                    <Tooltip title="View Token">
                        <IconButton onClick={() => setShowTokenModal(true)} size="small">
                            <VisibilityIcon/>
                        </IconButton>
                    </Tooltip>
                    <Tooltip title="Copy Token">
                        <IconButton onClick={() => copyToClipboard(token, 'API Key')} size="small">
                            <CopyIcon fontSize="small"/>
                        </IconButton>
                    </Tooltip>
                </Box>
            </ApiConfigRow>
            {/*<ApiConfigRow*/}
            {/*    label="Model Name"*/}
            {/*    value={modelName}*/}
            {/*    onCopy={() => copyToClipboard(modelName, 'Model Name')}*/}
            {/*    isClickable={true}*/}
            {/*>*/}
            {/*    <Box sx={{display: 'flex', gap: 0.5, ml: 'auto'}}>*/}
            {/*        /!* <Tooltip title="Edit Rule">*/}
            {/*            <IconButton onClick={() => navigate('/routing?expand=openai')} size="small">*/}
            {/*                <EditIcon fontSize="small"/>*/}
            {/*            </IconButton>*/}
            {/*        </Tooltip> *!/*/}
            {/*        <Tooltip title="Copy Model">*/}
            {/*            <IconButton onClick={() => copyToClipboard(modelName, 'Model Name')} size="small">*/}
            {/*                <CopyIcon fontSize="small"/>*/}
            {/*            </IconButton>*/}
            {/*        </Tooltip>*/}
            {/*    </Box>*/}
            {/*</ApiConfigRow>*/}
        </Box>
    );

    // Show empty state if no providers
    if (!providers.length) {
        return (
            <PageLayout>
                <CardGrid>
                    <UnifiedCard title="OpenAI SDK Configuration" size="full">
                        <EmptyStateGuide
                            title="No Providers Configured"
                            description="Add an API key or OAuth provider to get started"
                            onAddApiKeyClick={providerDialog.handleAddProviderClick}
                            onAddOAuthClick={() => setOAuthDialogOpen(true)}
                        />
                    </UnifiedCard>
                    <ProviderFormDialog
                        open={providerDialog.providerDialogOpen}
                        onClose={providerDialog.handleCloseDialog}
                        onSubmit={providerDialog.handleProviderSubmit}
                        data={providerDialog.providerFormData}
                        onChange={providerDialog.handleFieldChange}
                        mode="add"
                        isFirstProvider={providers.length === 0}
                    />
                    <OAuthDialog
                        open={oauthDialogOpen}
                        onClose={() => setOAuthDialogOpen(false)}
                    />
                </CardGrid>
            </PageLayout>
        );
    }

    return (
        <PageLayout>
            <CardGrid>
                <UnifiedCard
                    title="OpenAI SDK Configuration"
                    size="full"
                    rightAction={
                        <Stack direction="row" spacing={1}>
                            <Tooltip title="Add new API Key">
                                <Button
                                    variant="outlined"
                                    startIcon={<KeyIcon />}
                                    onClick={providerDialog.handleAddProviderClick}
                                    size="small"
                                >
                                    Add API Key
                                </Button>
                            </Tooltip>
                            <Tooltip title="Create new routing rule">
                                <Button
                                    variant="contained"
                                    startIcon={<AddIcon />}
                                    onClick={handleCreateRule}
                                    size="small"
                                >
                                    New Rule
                                </Button>
                            </Tooltip>
                        </Stack>
                    }
                >
                    {header}
                </UnifiedCard>
                <TemplatePage
                    title={
                        <Tooltip title="Use as model name in your API requests to forward">
                            Models and Forwarding Rules
                        </Tooltip>
                    }
                    rules={rules}
                    collapsible={true}
                    showTokenModal={showTokenModal}
                    setShowTokenModal={setShowTokenModal}
                    token={token}
                    showNotification={showNotification}
                    providers={providers}
                    onRulesChange={(rules) => setRules(rules)}
                    newlyCreatedRuleUuids={newlyCreatedRuleUuids}
                    allowDeleteRule={true}
                    onRuleDelete={handleRuleDelete}
                />
                <ProviderFormDialog
                    open={providerDialog.providerDialogOpen}
                    onClose={providerDialog.handleCloseDialog}
                    onSubmit={providerDialog.handleProviderSubmit}
                    data={providerDialog.providerFormData}
                    onChange={providerDialog.handleFieldChange}
                    mode="add"
                    isFirstProvider={providers.length === 0}
                />
            </CardGrid>
        </PageLayout>
    );
};

export default UseOpenAIPage;

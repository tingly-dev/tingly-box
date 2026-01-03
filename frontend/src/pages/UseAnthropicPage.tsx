import React from 'react';
import {ContentCopy as CopyIcon, Edit as EditIcon} from '@mui/icons-material';
import VisibilityIcon from '@mui/icons-material/Visibility';
import {Box, IconButton, Tooltip, Typography} from '@mui/material';
import {useNavigate} from 'react-router-dom';
import {ApiConfigRow} from '../components/ApiConfigRow';
import {BaseUrlRow} from '../components/BaseUrlRow';
import TabTemplatePage from '../components/TabTemplatePage';
import {api, getBaseUrl} from '../services/api';
import type { Provider } from '../types/provider';
import CardGrid from "@/components/CardGrid.tsx";
import UnifiedCard from "@/components/UnifiedCard.tsx";

interface UseAnthropicPageProps {
    showTokenModal: boolean;
    setShowTokenModal: (show: boolean) => void;
    token: string;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
    providers: Provider[];
}

const ruleId = "built-in-anthropic";
const scenario = "anthropic";

const UseAnthropicPage: React.FC<UseAnthropicPageProps> = ({
    showTokenModal,
    setShowTokenModal,
    token,
    showNotification,
    providers
}) => {
    const [baseUrl, setBaseUrl] = React.useState<string>('');
    const [rules, setRules] = React.useState<any>(null);
    const [loadingRule, setLoadingRule] = React.useState(true);
    const navigate = useNavigate();

    const copyToClipboard = async (text: string, label: string) => {
        try {
            await navigator.clipboard.writeText(text);
            showNotification(`${label} copied to clipboard!`, 'success');
        } catch (err) {
            showNotification('Failed to copy to clipboard', 'error');
        }
    };

    const loadData = async () => {
        const url = await getBaseUrl();
        setBaseUrl(url);

        // Fetch rule information
        const result = await api.getRules(scenario);
        console.log('getRules result:', result);
        if (result.success) {
            console.log('result.data:', result.data);
            // getRules returns an array, we need the first item or filter by ruleId
            const ruleData = result.data;
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
                path="/tingly/anthropic"
                legacyPath ="/anthropic"
                legacyLabel="(Legacy) Base URL "
                baseUrl={baseUrl}
                urlLabel="Anthropic Base URL"
                onCopy={(url) => copyToClipboard(url, 'Anthropic Base URL')}
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
            {/*            <IconButton onClick={() => navigate('/routing?expand=anthropic')} size="small">*/}
            {/*                <EditIcon fontSize="small"/>*/}
            {/*            </IconButton>*/}
            {/*        </Tooltip> *!/*/}
            {/*        <Tooltip title="Copy Model">*/}
            {/*            <IconButton onClick={() => copyToClipboard(modelName || ruleId, 'Model Name')} size="small">*/}
            {/*                <CopyIcon fontSize="small"/>*/}
            {/*            </IconButton>*/}
            {/*        </Tooltip>*/}
            {/*    </Box>*/}
            {/*</ApiConfigRow>*/}
        </Box>
    );

    return (
        <CardGrid>
            <UnifiedCard
                title="Use Anthropic SDK"
                size="full"
            >
                {header}
            </UnifiedCard>
            <TabTemplatePage
                rules={rules}
                collapsible={true}
                showTokenModal={showTokenModal}
                setShowTokenModal={setShowTokenModal}
                token={token}
                showNotification={showNotification}
                providers={providers}
                onRulesChange={async () => await loadData()}
            />
        </CardGrid>
    );
};

export default UseAnthropicPage;

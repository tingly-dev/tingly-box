import React from 'react';
import {ContentCopy as CopyIcon, Edit as EditIcon} from '@mui/icons-material';
import VisibilityIcon from '@mui/icons-material/Visibility';
import {Box, IconButton, Tooltip, Typography} from '@mui/material';
import {useNavigate} from 'react-router-dom';
import {ApiConfigRow} from '../components/ApiConfigRow';
import TabTemplatePage from '../components/TabTemplatePage';
import {api, getBaseUrl} from '../services/api';
import type { Provider } from '../types/provider';

interface UseOpenAIPageProps {
    showTokenModal: boolean;
    setShowTokenModal: (show: boolean) => void;
    token: string;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
    providers: Provider[];
}

const ruleId = "built-in-openai";

const UseOpenAIPage: React.FC<UseOpenAIPageProps> = ({
    showTokenModal,
    setShowTokenModal,
    token,
    showNotification,
    providers
}) => {
    const [baseUrl, setBaseUrl] = React.useState<string>('');
    const [rule, setRule] = React.useState<any>(null);
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
        const result = await api.getRule(ruleId);
        if (result.success) {
            setRule(result.data);
            console.log(rule)
        }
        setLoadingRule(false);
    };

    React.useEffect(() => {
        loadData();
    }, []);

    const openaiBaseUrl = `${baseUrl}/openai`;
    const modelName = rule?.request_model;

    const header = (
        <Box sx={{p: 2}}>
            <ApiConfigRow
                label="Base URL"
                value={openaiBaseUrl}
                onCopy={() => copyToClipboard(openaiBaseUrl, 'OpenAI Base URL')}
                isClickable={true}
            >
                <Box sx={{display: 'flex', gap: 0.5, ml: 'auto'}}>
                    <Tooltip title="Copy Base URL">
                        <IconButton onClick={() => copyToClipboard(openaiBaseUrl, 'OpenAI Base URL')} size="small">
                            <CopyIcon fontSize="small"/>
                        </IconButton>
                    </Tooltip>
                </Box>
            </ApiConfigRow>
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
            <ApiConfigRow
                label="Model Name"
                value={modelName}
                onCopy={() => copyToClipboard(modelName, 'Model Name')}
                isClickable={true}
            >
                <Box sx={{display: 'flex', gap: 0.5, ml: 'auto'}}>
                    <Tooltip title="Edit Rule">
                        <IconButton onClick={() => navigate('/routing?expand=openai')} size="small">
                            <EditIcon fontSize="small"/>
                        </IconButton>
                    </Tooltip>
                    <Tooltip title="Copy Model">
                        <IconButton onClick={() => copyToClipboard(modelName, 'Model Name')} size="small">
                            <CopyIcon fontSize="small"/>
                        </IconButton>
                    </Tooltip>
                </Box>
            </ApiConfigRow>
        </Box>
    );

    return (
        <TabTemplatePage
            title="OpenAI Configuration"
            rule={rule}
            header={header}
            showTokenModal={showTokenModal}
            setShowTokenModal={setShowTokenModal}
            token={token}
            showNotification={showNotification}
            providers={providers}
            onRuleChange={setRule}
        />
    );
};

export default UseOpenAIPage;

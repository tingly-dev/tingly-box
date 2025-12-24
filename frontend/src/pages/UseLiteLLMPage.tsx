import React from 'react';
import {ContentCopy as CopyIcon, Edit as EditIcon} from '@mui/icons-material';
import VisibilityIcon from '@mui/icons-material/Visibility';
import {Box, IconButton, Tooltip, Typography} from '@mui/material';
import {useNavigate} from 'react-router-dom';
import {ApiConfigRow} from '../components/ApiConfigRow';
import TabTemplatePage from '../components/TabTemplatePage';
import {api, getBaseUrl} from '../services/api';

interface UseLiteLLMPageProps {
    showTokenModal: boolean;
    setShowTokenModal: (show: boolean) => void;
    token: string;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
}

const ruleId = "build-in-tingly-litellm";

const UseLiteLLMPage: React.FC<UseLiteLLMPageProps> = ({
                                                           showTokenModal,
                                                           setShowTokenModal,
                                                           token,
                                                           showNotification
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
        }
        setLoadingRule(false);
    };

    React.useEffect(() => {
        loadData();
    }, []);

    const litellmBaseUrl = `${baseUrl}/litellm`;
    const modelName = rule?.request_model;

    const header = (
        <Box sx={{p: 2}}>
            <Typography variant="h6" sx={{fontWeight: 600, mb: 2}}>
                Use LiteLLM
            </Typography>
            <ApiConfigRow
                label="Base URL"
                value={litellmBaseUrl}
                onCopy={() => copyToClipboard(litellmBaseUrl, 'LiteLLM Base URL')}
                isClickable={true}
            >
                <Box sx={{display: 'flex', gap: 0.5, ml: 'auto'}}>
                    <Tooltip title="Copy Base URL">
                        <IconButton onClick={() => copyToClipboard(litellmBaseUrl, 'LiteLLM Base URL')} size="small">
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
                value={modelName || ruleId}
                onCopy={() => copyToClipboard(modelName || ruleId, 'Model Name')}
                isClickable={true}
            >
                <Box sx={{display: 'flex', gap: 0.5, ml: 'auto'}}>
                    <Tooltip title="Edit Rule">
                        <IconButton onClick={() => navigate('/routing?expand=litellm')} size="small">
                            <EditIcon fontSize="small"/>
                        </IconButton>
                    </Tooltip>
                    <Tooltip title="Copy Model">
                        <IconButton onClick={() => copyToClipboard(modelName || ruleId, 'Model Name')} size="small">
                            <CopyIcon fontSize="small"/>
                        </IconButton>
                    </Tooltip>
                </Box>
            </ApiConfigRow>
        </Box>
    );

    return (
        <TabTemplatePage
            title="LiteLLM Configuration"
            rule={rule}
            header={header}
            showTokenModal={showTokenModal}
            setShowTokenModal={setShowTokenModal}
            token={token}
            showNotification={showNotification}
            onRuleChange={setRule}
        />
    );
};

export default UseLiteLLMPage;

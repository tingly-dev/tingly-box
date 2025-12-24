import React from 'react';
import {ContentCopy as CopyIcon, Edit as EditIcon} from '@mui/icons-material';
import VisibilityIcon from '@mui/icons-material/Visibility';
import {Box, IconButton, Tooltip, Typography} from '@mui/material';
import {useNavigate} from 'react-router-dom';
import {ApiConfigRow} from '../components/ApiConfigRow';
import TabTemplatePage from '../components/TabTemplatePage';
import {getBaseUrl} from '../services/api';

interface UseLiteLLMPageProps {
    showTokenModal: boolean;
    setShowTokenModal: (show: boolean) => void;
    token: string;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
}

const UseLiteLLMPage: React.FC<UseLiteLLMPageProps> = ({
                                                           showTokenModal,
                                                           setShowTokenModal,
                                                           token,
                                                           showNotification
                                                       }) => {
    const [baseUrl, setBaseUrl] = React.useState<string>('');
    const navigate = useNavigate();

    const copyToClipboard = async (text: string, label: string) => {
        try {
            await navigator.clipboard.writeText(text);
            showNotification(`${label} copied to clipboard!`, 'success');
        } catch (err) {
            showNotification('Failed to copy to clipboard', 'error');
        }
    };

    React.useEffect(() => {
        const loadBaseUrl = async () => {
            const url = await getBaseUrl();
            setBaseUrl(url);
        };
        loadBaseUrl();
    }, []);

    const litellmBaseUrl = `${baseUrl}/litellm`;

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
                value="litellm"
                onCopy={() => copyToClipboard('litellm', 'Model Name')}
                isClickable={true}
            >
                <Box sx={{display: 'flex', gap: 0.5, ml: 'auto'}}>
                    <Tooltip title="Edit Rule">
                        <IconButton onClick={() => navigate('/routing?expand=litellm')} size="small">
                            <EditIcon fontSize="small"/>
                        </IconButton>
                    </Tooltip>
                    <Tooltip title="Copy Model">
                        <IconButton onClick={() => copyToClipboard('litellm', 'Model Name')} size="small">
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
            ruleName="litellm"
            header={header}
            showTokenModal={showTokenModal}
            setShowTokenModal={setShowTokenModal}
            token={token}
            showNotification={showNotification}
        />
    );
};

export default UseLiteLLMPage;

import React from 'react';
import {ContentCopy as CopyIcon, Edit as EditIcon} from '@mui/icons-material';
import VisibilityIcon from '@mui/icons-material/Visibility';
import {Box, IconButton, Tooltip, Typography} from '@mui/material';
import {useNavigate} from 'react-router-dom';
import {ApiConfigRow} from '../components/ApiConfigRow';
import TabTemplatePage from '../components/TabTemplatePage';
import {getBaseUrl} from '../services/api';

interface UseOpenAIPageProps {
    showTokenModal: boolean;
    setShowTokenModal: (show: boolean) => void;
    token: string;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
}

const ruleName = "tingly/openai"

const UseOpenAIPage: React.FC<UseOpenAIPageProps> = ({
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

    const openaiBaseUrl = `${baseUrl}/openai`;

    const header = (
        <Box sx={{p: 2}}>
            <Typography variant="h6" sx={{fontWeight: 600, mb: 2}}>
                Use OpenAI
            </Typography>
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
                value={ruleName}
                onCopy={() => copyToClipboard('openai', 'Model Name')}
                isClickable={true}
            >
                <Box sx={{display: 'flex', gap: 0.5, ml: 'auto'}}>
                    <Tooltip title="Edit Rule">
                        <IconButton onClick={() => navigate('/routing?expand=openai')} size="small">
                            <EditIcon fontSize="small"/>
                        </IconButton>
                    </Tooltip>
                    <Tooltip title="Copy Model">
                        <IconButton onClick={() => copyToClipboard('openai', 'Model Name')} size="small">
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
            ruleName={ruleName}
            header={header}
            showTokenModal={showTokenModal}
            setShowTokenModal={setShowTokenModal}
            token={token}
            showNotification={showNotification}
        />
    );
};

export default UseOpenAIPage;

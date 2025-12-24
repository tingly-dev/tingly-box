import React from 'react';
import {ContentCopy as CopyIcon, Edit as EditIcon} from '@mui/icons-material';
import VisibilityIcon from '@mui/icons-material/Visibility';
import {Box, IconButton, Tooltip, Typography} from '@mui/material';
import {useNavigate} from 'react-router-dom';
import {ApiConfigRow} from '../components/ApiConfigRow';
import TabTemplatePage from '../components/TabTemplatePage';
import {getBaseUrl} from '../services/api';

interface UseAnthropicPageProps {
    showTokenModal: boolean;
    setShowTokenModal: (show: boolean) => void;
    token: string;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
}

const ruleName = "tingly/anthropic"

const UseAnthropicPage: React.FC<UseAnthropicPageProps> = ({
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

    const anthropicBaseUrl = `${baseUrl}/anthropic`;

    const header = (
        <Box sx={{p: 2}}>
            <Typography variant="h6" sx={{fontWeight: 600, mb: 2}}>
                Use Anthropic
            </Typography>
            <ApiConfigRow
                label="Base URL"
                value={anthropicBaseUrl}
                onCopy={() => copyToClipboard(anthropicBaseUrl, 'Anthropic Base URL')}
                isClickable={true}
            >
                <Box sx={{display: 'flex', gap: 0.5, ml: 'auto'}}>
                    <Tooltip title="Copy Base URL">
                        <IconButton onClick={() => copyToClipboard(anthropicBaseUrl, 'Anthropic Base URL')}
                                    size="small">
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
                onCopy={() => copyToClipboard('anthropic', 'Model Name')}
                isClickable={true}
            >
                <Box sx={{display: 'flex', gap: 0.5, ml: 'auto'}}>
                    <Tooltip title="Edit Rule">
                        <IconButton onClick={() => navigate('/routing?expand=anthropic')} size="small">
                            <EditIcon fontSize="small"/>
                        </IconButton>
                    </Tooltip>
                    <Tooltip title="Copy Model">
                        <IconButton onClick={() => copyToClipboard('anthropic', 'Model Name')} size="small">
                            <CopyIcon fontSize="small"/>
                        </IconButton>
                    </Tooltip>
                </Box>
            </ApiConfigRow>
        </Box>
    );

    return (
        <TabTemplatePage
            title="Anthropic Configuration"
            ruleName={ruleName}
            header={header}
            showTokenModal={showTokenModal}
            setShowTokenModal={setShowTokenModal}
            token={token}
            showNotification={showNotification}
        />
    );
};

export default UseAnthropicPage;

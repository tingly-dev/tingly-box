import React from 'react';
import {ContentCopy as CopyIcon, Edit as EditIcon, ExpandMore as ExpandMoreIcon} from '@mui/icons-material';
import VisibilityIcon from '@mui/icons-material/Visibility';
import {
    Accordion,
    AccordionDetails,
    AccordionSummary,
    Box,
    Button,
    Chip,
    IconButton,
    Stack,
    TextField,
    Tooltip,
    Typography
} from '@mui/material';
import {useNavigate} from 'react-router-dom';
import {ApiConfigRow} from '../components/ApiConfigRow';
import TabTemplatePage from '../components/TabTemplatePage';
import {getBaseUrl} from '../services/api';

interface UseClaudeCodePageProps {
    showTokenModal: boolean;
    setShowTokenModal: (show: boolean) => void;
    token: string;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
}

const ruleName = "tingly/ide"

const UseClaudeCodePage: React.FC<UseClaudeCodePageProps> = ({
                                                                 showTokenModal,
                                                                 setShowTokenModal,
                                                                 token,
                                                                 showNotification
                                                             }) => {
    const [baseUrl, setBaseUrl] = React.useState<string>('');
    const [configPath, setConfigPath] = React.useState('~/Library/Application Support/Claude/claude_desktop_config.json');
    const [defaultModel, setDefaultModel] = React.useState(ruleName);
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

    const claudeCodeBaseUrl = `${baseUrl}/anthropic`;

    const generateConfig = () => {
        return JSON.stringify({
            env: {
                DISABLE_TELEMETRY: "1",
                DISABLE_ERROR_REPORTING: "1",
                CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC: "1",
                API_TIMEOUT_MS: "3000000",
                ANTHROPIC_AUTH_TOKEN: token,
                ANTHROPIC_BASE_URL: claudeCodeBaseUrl,
                ANTHROPIC_DEFAULT_HAIKU_MODEL: defaultModel,
                ANTHROPIC_DEFAULT_OPUS_MODEL: defaultModel,
                ANTHROPIC_DEFAULT_SONNET_MODEL: defaultModel,
                ANTHROPIC_MODEL: defaultModel
            },
        }, null, 2);
    };

    const header = (
        <Box sx={{p: 2}}>
            <Typography variant="h6" sx={{fontWeight: 600, mb: 2}}>
                Use Claude Code
            </Typography>

            <ApiConfigRow
                label="Base URL"
                value={claudeCodeBaseUrl}
                onCopy={() => copyToClipboard(claudeCodeBaseUrl, 'Base URL')}
                isClickable={true}
            >
                <Box sx={{display: 'flex', gap: 0.5, ml: 'auto'}}>
                    <Tooltip title="Copy Base URL">
                        <IconButton onClick={() => copyToClipboard(claudeCodeBaseUrl, 'Base URL')} size="small">
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
                        <IconButton onClick={() => copyToClipboard(token, 'API Token')} size="small">
                            <CopyIcon fontSize="small"/>
                        </IconButton>
                    </Tooltip>
                </Box>
            </ApiConfigRow>

            <ApiConfigRow
                label="Model Name"
                value={defaultModel}
                onCopy={() => copyToClipboard(defaultModel, 'Model Name')}
                isClickable={true}
            >
                <Box sx={{display: 'flex', gap: 0.5, ml: 'auto'}}>
                    <Tooltip title="Edit Rule">
                        <IconButton onClick={() => navigate('/routing?expand=claude-code')} size="small">
                            <EditIcon fontSize="small"/>
                        </IconButton>
                    </Tooltip>
                    <Tooltip title="Copy Model">
                        <IconButton onClick={() => copyToClipboard(defaultModel, 'Model Name')} size="small">
                            <CopyIcon fontSize="small"/>
                        </IconButton>
                    </Tooltip>
                </Box>
            </ApiConfigRow>

            <Box sx={{mt: 3}}>
                <Typography variant="subtitle2" sx={{fontWeight: 600, mb: 2}}>
                    Configuration Instructions
                </Typography>

                <Box sx={{mb: 2}}>
                    <Typography variant="body2" color="text.secondary" sx={{mb: 1}}>
                        Config file location:
                    </Typography>
                    <TextField
                        fullWidth
                        size="small"
                        value={configPath}
                        onChange={(e) => setConfigPath(e.target.value)}
                        sx={{
                            '& .MuiOutlinedInput-root': {
                                fontFamily: 'monospace',
                                fontSize: '0.85rem',
                            }
                        }}
                    />
                </Box>

                <Accordion defaultExpanded>
                    <AccordionSummary expandIcon={<ExpandMoreIcon/>}>
                        <Box sx={{
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'space-between',
                            width: '100%',
                            mr: 1
                        }}>
                            <Typography variant="subtitle2" sx={{fontWeight: 600}}>
                                Configuration JSON
                            </Typography>
                            <Chip label="Click to expand" size="small" variant="outlined" sx={{fontSize: '0.7rem'}}/>
                        </Box>
                    </AccordionSummary>
                    <AccordionDetails>
                        <Stack spacing={2}>
                            <Box
                                sx={{
                                    p: 2,
                                    bgcolor: 'grey.900',
                                    borderRadius: 1,
                                    fontFamily: 'monospace',
                                    fontSize: '0.75rem',
                                    color: 'grey.100',
                                    overflow: 'auto',
                                    maxHeight: 300,
                                    '&::-webkit-scrollbar': {width: '8px'},
                                    '&::-webkit-scrollbar-track': {bgcolor: 'grey.800'},
                                    '&::-webkit-scrollbar-thumb': {bgcolor: 'grey.600', borderRadius: 4},
                                }}
                            >
                                <pre style={{margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-all'}}>
                                    {generateConfig()}
                                </pre>
                            </Box>

                            <Button
                                variant="contained"
                                startIcon={<CopyIcon fontSize="small"/>}
                                onClick={() => copyToClipboard(generateConfig(), 'Claude Code Configuration')}
                                fullWidth
                            >
                                Copy Configuration
                            </Button>

                            <Box>
                                <Typography variant="body2" color="text.secondary" sx={{mb: 1}}>
                                    Default model name in config:
                                </Typography>
                                <TextField
                                    size="small"
                                    value={defaultModel}
                                    onChange={(e) => setDefaultModel(e.target.value)}
                                    placeholder="e.g., tingly, cc, claude-sonnet-4"
                                    sx={{minWidth: 300}}
                                />
                            </Box>
                        </Stack>
                    </AccordionDetails>
                </Accordion>

                <Box sx={{mt: 2, p: 2, bgcolor: 'info.50', borderRadius: 1}}>
                    <Typography variant="subtitle2" sx={{fontWeight: 600, mb: 1}}>
                        Quick Tips:
                    </Typography>
                    <Typography variant="body2" color="text.secondary" sx={{mb: 0.5}}>
                        • Copy the configuration JSON above
                    </Typography>
                    <Typography variant="body2" color="text.secondary" sx={{mb: 0.5}}>
                        • Open the config file at the location shown
                    </Typography>
                    <Typography variant="body2" color="text.secondary" sx={{mb: 0.5}}>
                        • Replace the content with the copied configuration
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        • Restart Claude Code for changes to take effect
                    </Typography>
                </Box>
            </Box>
        </Box>
    );

    return (
        <TabTemplatePage
            title="Claude Code Configuration"
            ruleName={ruleName}
            header={header}
            showTokenModal={showTokenModal}
            setShowTokenModal={setShowTokenModal}
            token={token}
            showNotification={showNotification}
        />
    );
};

export default UseClaudeCodePage;

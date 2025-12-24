import { Box, Typography } from '@mui/material';
import React from 'react';
import { useNavigate } from 'react-router-dom';
import CodeBlock from '../components/CodeBlock';
import TabTemplatePage from '../components/TabTemplatePage';
import { api, getBaseUrl } from '../services/api';
import type { Provider } from '../types/provider';

interface UseClaudeCodePageProps {
    showTokenModal: boolean;
    setShowTokenModal: (show: boolean) => void;
    token: string;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
    providers: Provider[];
}

const ruleId = "built-in-cc";

const UseClaudeCodePage: React.FC<UseClaudeCodePageProps> = ({
    showTokenModal,
    setShowTokenModal,
    token,
    showNotification,
    providers
}) => {
    const [baseUrl, setBaseUrl] = React.useState<string>('');
    const [configPath] = React.useState('~/.claude/settings.json');
    const [rule, setRule] = React.useState<any>(null);
    const [defaultModel, setDefaultModel] = React.useState("");
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

    // Update defaultModel when rule changes
    React.useEffect(() => {
        if (rule?.request_model) {
            setDefaultModel(rule.request_model);
        }
    }, [rule]);

    const claudeCodeBaseUrl = `${baseUrl}/anthropic`;

    const generateConfig = () => {
        let res = JSON.stringify({
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
        return res.trim().substring(1, res.length - 1).trim();
    };

    const header = (
        <Box sx={{ p: 2 }}>
            <Box sx={{ mb: 2 }}>
                <Typography>
                    Add env config into claude code config file <code
                        style={{ fontSize: '0.85rem' }}>{configPath}</code>
                </Typography>
            </Box>
            <CodeBlock
                code={generateConfig()}
                language="json"
                filename="settings.json"
                onCopy={(code) => copyToClipboard(code, 'Configuration')}
                maxHeight={280}
            />
        </Box>
    );

    return (
        <TabTemplatePage
            title="Claude Code Config"
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

export default UseClaudeCodePage;

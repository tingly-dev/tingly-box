import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Tab, Tabs, Typography } from '@mui/material';
import React from 'react';
import CodeBlock from './CodeBlock';
import { shouldIgnoreDialogClose } from './dialogClose';

interface CodexConfigModalProps {
    open: boolean;
    onClose: () => void;
    baseUrl: string;
    token: string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
}

type ScriptTab = 'json' | 'windows' | 'unix';

const CodexConfigModal: React.FC<CodexConfigModalProps> = ({
    open,
    onClose,
    baseUrl,
    token,
    copyToClipboard,
}) => {
    const [configTab, setConfigTab] = React.useState<ScriptTab>('json');
    const [authTab, setAuthTab] = React.useState<ScriptTab>('json');

    const codexBaseUrl = `${baseUrl}/tingly/codex`;

    const configToml = `model = "tingly-codex"
model_provider = "tingly-box"

[model_providers.tingly-box]
name = "OpenAI using Tingly Box"
base_url = "${codexBaseUrl}"
preferred_auth_method = "apikey"
wire_api = "responses"`;

    const authJson = `{
  "OPENAI_API_KEY": "${token}"
}`;

    const windowsConfigScript = `$configDir = Join-Path $HOME ".codex"
$configPath = Join-Path $configDir "config.toml"

New-Item -ItemType Directory -Force -Path $configDir | Out-Null

@'
${configToml}
'@ | Set-Content -Path $configPath`;

    const unixConfigScript = `mkdir -p ~/.codex

cat > ~/.codex/config.toml <<'EOF'
${configToml}
EOF`;

    const windowsAuthScript = `$configDir = Join-Path $HOME ".codex"
$authPath = Join-Path $configDir "auth.json"

New-Item -ItemType Directory -Force -Path $configDir | Out-Null

@'
${authJson}
'@ | Set-Content -Path $authPath`;

    const unixAuthScript = `mkdir -p ~/.codex

cat > ~/.codex/auth.json <<'EOF'
${authJson}
EOF`;

    return (
        <Dialog
            open={open}
            onClose={(event, reason) => {
                if (shouldIgnoreDialogClose(reason)) {
                    return;
                }
                onClose();
            }}
            maxWidth="lg"
            fullWidth
            PaperProps={{
                sx: {
                    borderRadius: 3,
                    maxHeight: '90vh',
                },
            }}
        >
            <DialogTitle sx={{ pb: 1, borderBottom: 1, borderColor: 'divider' }}>
                <Typography variant="h6" fontWeight={600}>
                    Configure Codex
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                    Configure Codex to use Tingly Box through `~/.codex/config.toml` and `~/.codex/auth.json`
                </Typography>
            </DialogTitle>

            <DialogContent sx={{ p: 3 }}>
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
                    <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                        <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                            <Typography variant="subtitle2" color="text.secondary">
                                Step 1 · Create or update `~/.codex/config.toml`
                            </Typography>
                            <Tabs
                                value={configTab}
                                onChange={(_, value) => setConfigTab(value)}
                                variant="standard"
                                sx={{ minHeight: 32, '& .MuiTabs-indicator': { height: 3 } }}
                            >
                                <Tab label="TOML" value="json" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                <Tab label="Windows" value="windows" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                <Tab label="Linux/macOS" value="unix" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                            </Tabs>
                        </Box>
                        <Box>
                            {configTab === 'json' && (
                                <CodeBlock
                                    code={configToml}
                                    language="toml"
                                    filename="Create or update ~/.codex/config.toml"
                                    wrap={true}
                                    onCopy={(code) => copyToClipboard(code, 'config.toml')}
                                    maxHeight={220}
                                    minHeight={180}
                                />
                            )}
                            {configTab === 'windows' && (
                                <CodeBlock
                                    code={windowsConfigScript}
                                    language="js"
                                    filename="PowerShell script to setup ~/.codex/config.toml"
                                    wrap={true}
                                    onCopy={(code) => copyToClipboard(code, 'Windows config script')}
                                    maxHeight={260}
                                    minHeight={220}
                                />
                            )}
                            {configTab === 'unix' && (
                                <CodeBlock
                                    code={unixConfigScript}
                                    language="js"
                                    filename="Bash script to setup ~/.codex/config.toml"
                                    wrap={true}
                                    onCopy={(code) => copyToClipboard(code, 'Unix config script')}
                                    maxHeight={260}
                                    minHeight={220}
                                />
                            )}
                        </Box>
                    </Box>

                    <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                        <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                            <Typography variant="subtitle2" color="text.secondary">
                                Step 2 · Create or update `~/.codex/auth.json`
                            </Typography>
                            <Tabs
                                value={authTab}
                                onChange={(_, value) => setAuthTab(value)}
                                variant="standard"
                                sx={{ minHeight: 32, '& .MuiTabs-indicator': { height: 3 } }}
                            >
                                <Tab label="JSON" value="json" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                <Tab label="Windows" value="windows" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                <Tab label="Linux/macOS" value="unix" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                            </Tabs>
                        </Box>
                        <Box sx={{ mb: 1.5 }}>
                            <Typography variant="body2" color="text.secondary">
                                Set `OPENAI_API_KEY` in `~/.codex/auth.json` to the API key generated by Tingly Box. If the file already exists, update the existing value.
                            </Typography>
                        </Box>
                        <Box>
                            {authTab === 'json' && (
                                <CodeBlock
                                    code={authJson}
                                    language="json"
                                    filename="Create or update ~/.codex/auth.json"
                                    wrap={true}
                                    onCopy={(code) => copyToClipboard(code, 'auth.json')}
                                    maxHeight={140}
                                    minHeight={100}
                                />
                            )}
                            {authTab === 'windows' && (
                                <CodeBlock
                                    code={windowsAuthScript}
                                    language="js"
                                    filename="PowerShell script to setup ~/.codex/auth.json"
                                    wrap={true}
                                    onCopy={(code) => copyToClipboard(code, 'Windows auth script')}
                                    maxHeight={220}
                                    minHeight={180}
                                />
                            )}
                            {authTab === 'unix' && (
                                <CodeBlock
                                    code={unixAuthScript}
                                    language="js"
                                    filename="Bash script to setup ~/.codex/auth.json"
                                    wrap={true}
                                    onCopy={(code) => copyToClipboard(code, 'Unix auth script')}
                                    maxHeight={220}
                                    minHeight={180}
                                />
                            )}
                        </Box>
                    </Box>
                </Box>
            </DialogContent>

            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Button onClick={onClose} variant="contained">
                    Close
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default CodexConfigModal;

import { Box, Dialog, DialogActions, DialogContent, DialogTitle, Button, Typography } from '@mui/material';
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

const CodexConfigModal: React.FC<CodexConfigModalProps> = ({
    open,
    onClose,
    baseUrl,
    token,
    copyToClipboard,
}) => {
    const codexBaseUrl = `${baseUrl}/tingly/codex`;
    const modelName = 'tingly-codex';

    const userConfig = `model = "${modelName}"
model_provider = "tingly-box"

[model_providers.tingly-box]
name = "OpenAI using Tingly Box"
base_url = "${codexBaseUrl}"
env_key = "OPENAI_API_KEY"
wire_api = "responses"`;

    const projectConfig = `export OPENAI_API_KEY="${token}"

# Then start Codex normally
codex`;

    const providerNotes = `[model_providers.tingly-box]
name = "OpenAI using Tingly Box"
base_url = "${codexBaseUrl}"
env_key = "OPENAI_API_KEY"
wire_api = "responses"

# Optional if your upstream requires custom headers:
# http_headers = { "X-Example-Header" = "example-value" }
# env_http_headers = { "X-Example-Features" = "EXAMPLE_FEATURES" }`;

    const projectOverrideConfig = `# <your-repo>/.codex/config.toml
model = "${modelName}"
model_provider = "tingly-box"`;

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
                    Configure Codex using the official Custom model providers pattern
                </Typography>
            </DialogTitle>

            <DialogContent sx={{ p: 3 }}>
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
                    <Box sx={{ p: 2, bgcolor: 'info.50', borderRadius: 1 }}>
                        <Typography variant="body2" color="info.dark">
                            <strong>Config Directory:</strong> ~/.codex
                        </Typography>
                        <Typography variant="body2" color="info.dark">
                            <strong>Main Config:</strong> ~/.codex/config.toml
                        </Typography>
                        <Typography variant="body2" color="info.dark" sx={{ mt: 0.5 }}>
                            <strong>Project Config:</strong> .codex/config.toml
                        </Typography>
                        <Typography variant="body2" color="info.dark" sx={{ mt: 0.5 }}>
                            <strong>Base URL:</strong> {codexBaseUrl}
                        </Typography>
                        <Typography variant="body2" color="info.dark" sx={{ mt: 0.5 }}>
                            <strong>API Key Env:</strong> OPENAI_API_KEY
                        </Typography>
                    </Box>

                    <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                        <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                            <Typography variant="subtitle2" color="text.secondary">
                                Step 1 · Define a custom model provider in `~/.codex/config.toml`
                            </Typography>
                        </Box>
                        <Box>
                            <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
                                According to the Custom model providers docs, define a provider first, then point `model_provider` at it.
                            </Typography>
                            <CodeBlock
                                code={userConfig}
                                language="toml"
                                filename="~/.codex/config.toml"
                                wrap={true}
                                onCopy={(code) => copyToClipboard(code, 'User config.toml')}
                                maxHeight={220}
                                minHeight={180}
                            />
                        </Box>
                    </Box>

                    <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                        <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                            <Typography variant="subtitle2" color="text.secondary">
                                Step 2 · Optional provider details
                            </Typography>
                        </Box>
                        <Box>
                            <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
                                `base_url` and `env_key` are the essential fields. `wire_api = "responses"` keeps the provider aligned with the Responses API path used by Codex.
                            </Typography>
                            <CodeBlock
                                code={providerNotes}
                                language="toml"
                                filename="Provider reference"
                                wrap={true}
                                onCopy={(code) => copyToClipboard(code, 'Provider notes')}
                                maxHeight={220}
                                minHeight={180}
                            />
                        </Box>
                    </Box>

                    <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                        <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                            <Typography variant="subtitle2" color="text.secondary">
                                Step 3 · Optional project override in `.codex/config.toml`
                            </Typography>
                        </Box>
                        <Box>
                            <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
                                If you want this repo to always use the Tingly Box Codex route, add a project-scoped override. Keep `OPENAI_API_KEY` in your shell environment.
                            </Typography>
                            <CodeBlock
                                code={projectOverrideConfig}
                                language="toml"
                                filename=".codex/config.toml"
                                wrap={true}
                                onCopy={(code) => copyToClipboard(code, 'Project override config')}
                                maxHeight={180}
                                minHeight={120}
                            />
                        </Box>
                    </Box>

                    <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                        <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                            <Typography variant="subtitle2" color="text.secondary">
                                Step 4 · Export your API key before launching Codex
                            </Typography>
                        </Box>
                        <Box>
                            <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
                                `env_key = "OPENAI_API_KEY"` means Codex reads the credential from your environment instead of storing it in `config.toml`.
                            </Typography>
                            <CodeBlock
                                code={projectConfig}
                                language="bash"
                                filename="Terminal"
                                wrap={true}
                                onCopy={(code) => copyToClipboard(code, 'Launch commands')}
                                maxHeight={160}
                                minHeight={120}
                            />
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

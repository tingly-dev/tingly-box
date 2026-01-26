import { Box, Dialog, DialogTitle, DialogContent, DialogActions, Button, Typography, Tab, Tabs } from '@mui/material';
import React from 'react';
import CodeBlock from './CodeBlock';
import { useTranslation } from 'react-i18next';

interface OpenCodeConfigModalProps {
    open: boolean;
    onClose: () => void;
    baseUrl: string;
    token: string;
    requestModel: string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
}

type ScriptTab = 'json' | 'windows' | 'unix';

const OpenCodeConfigModal: React.FC<OpenCodeConfigModalProps> = ({
    open,
    onClose,
    baseUrl,
    token,
    requestModel,
    copyToClipboard,
}) => {
    const { t } = useTranslation();
    const [configTab, setConfigTab] = React.useState<ScriptTab>('json');

    // Generate OpenCode config JSON
    const generateConfigJson = () => {
        const configBaseUrl = baseUrl ? `${baseUrl}/tingly/opencode` : 'http://localhost:12580/tingly/opencode';
        return JSON.stringify({
            "$schema": "https://opencode.ai/config.json",
            "provider": {
                "tingly-box": {
                    "name": "tingly-box",
                    "npm": "@ai-sdk/anthropic",
                    "options": {
                        "baseURL": configBaseUrl,
                        "apiKey": `tingly-box-${token}`
                    },
                    "models": {
                        [requestModel]: {
                            "name": requestModel
                        }
                    }
                }
            }
        }, null, 2);
    };

    // Generate Windows PowerShell script
    const generateWindowsScript = () => {
        const configBaseUrl = baseUrl ? `${baseUrl}/tingly/opencode` : 'http://localhost:12580/tingly/opencode';
        const nodeCode = `const fs = require("fs");
const path = require("path");
const os = require("os");

const homeDir = os.homedir();
const configDir = path.join(homeDir, ".config", "opencode");
const configPath = path.join(configDir, "opencode.json");

// Create config directory if it doesn't exist
if (!fs.existsSync(configDir)) {
    fs.mkdirSync(configDir, { recursive: true });
}

const newProvider = {
    "tingly-box": {
        "name": "tingly-box",
        "npm": "@ai-sdk/anthropic",
        "options": {
            "baseURL": "${configBaseUrl}",
            "apiKey": "tingly-box-${token}"
        },
        "models": {
            "${requestModel}": {
                "name": "${requestModel}"
            }
        }
    }
};

let existingConfig = {};
if (fs.existsSync(configPath)) {
    const content = fs.readFileSync(configPath, "utf-8");
    existingConfig = JSON.parse(content);
}

// Merge providers
const newConfig = {
    ...existingConfig,
    "$schema": existingConfig["$schema"] || "https://opencode.ai/config.json",
    "provider": {
        ...(existingConfig.provider || {}),
        ...newProvider
    }
};

fs.writeFileSync(configPath, JSON.stringify(newConfig, null, 2));
console.log("OpenCode config written to", configPath);`;

        return `# PowerShell - Run in PowerShell
node -e @"
${nodeCode}
"@`;
    };

    // Generate Unix/Linux/macOS script
    const generateUnixScript = () => {
        const configBaseUrl = baseUrl ? `${baseUrl}/tingly/opencode` : 'http://localhost:12580/tingly/opencode';
        const nodeCode = `const fs = require("fs");
const path = require("path");
const os = require("os");

const homeDir = os.homedir();
const configDir = path.join(homeDir, ".config", "opencode");
const configPath = path.join(configDir, "opencode.json");

// Create config directory if it doesn't exist
if (!fs.existsSync(configDir)) {
    fs.mkdirSync(configDir, { recursive: true });
}

const newProvider = {
    "tingly-box": {
        "name": "tingly-box",
        "npm": "@ai-sdk/anthropic",
        "options": {
            "baseURL": "${configBaseUrl}",
            "apiKey": "tingly-box-${token}"
        },
        "models": {
            "${requestModel}": {
                "name": "${requestModel}"
            }
        }
    }
};

let existingConfig = {};
if (fs.existsSync(configPath)) {
    const content = fs.readFileSync(configPath, "utf-8");
    existingConfig = JSON.parse(content);
}

// Merge providers
const newConfig = {
    ...existingConfig,
    "$schema": existingConfig["$schema"] || "https://opencode.ai/config.json",
    "provider": {
        ...(existingConfig.provider || {}),
        ...newProvider
    }
};

fs.writeFileSync(configPath, JSON.stringify(newConfig, null, 2));
console.log("OpenCode config written to", configPath);`;

        return `# Bash - Run in terminal
node -e '${nodeCode.replace(/'/g, "'\\''")}'`;
    };

    return (
        <Dialog
            open={open}
            onClose={(event, reason) => {
                if (reason === 'backdropClick' || reason === 'escapeKeyDown') {
                    return;
                }
                onClose();
            }}
            maxWidth="lg"
            fullWidth
            disableEscapeKeyDown
            PaperProps={{
                sx: {
                    borderRadius: 3,
                    maxHeight: '90vh',
                }
            }}
        >
            <DialogTitle sx={{
                pb: 1,
                borderBottom: 1,
                borderColor: 'divider',
            }}>
                <Typography variant="h6" fontWeight={600}>
                    Configure OpenCode
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                    Set up OpenCode to use Tingly Box as your AI model proxy
                </Typography>
            </DialogTitle>

            <DialogContent sx={{ p: 3 }}>
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                    {/* Config file location info */}
                    <Box sx={{ p: 2, bgcolor: 'info.50', borderRadius: 1 }}>
                        <Typography variant="body2" color="info.dark">
                            <strong>Config Location:</strong> ~/.config/opencode/opencode.json
                        </Typography>
                    </Box>

                    {/* Config section */}
                    <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                        <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                            <Typography variant="subtitle2" color="text.secondary">
                                Configuration
                            </Typography>
                            <Tabs
                                value={configTab}
                                onChange={(_, value) => setConfigTab(value)}
                                variant="standard"
                                sx={{ minHeight: 32, '& .MuiTabs-indicator': { height: 3 } }}
                            >
                                <Tab label="JSON" value="json" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                <Tab label="Windows" value="windows" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                <Tab label="Linux/macOS" value="unix" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                            </Tabs>
                        </Box>
                        <Box>
                            {configTab === 'json' && (
                                <CodeBlock
                                    code={generateConfigJson()}
                                    language="json"
                                    filename="~/.config/opencode/opencode.json"
                                    wrap={true}
                                    onCopy={(code) => copyToClipboard(code, 'opencode.json')}
                                    maxHeight={350}
                                    minHeight={300}
                                />
                            )}
                            {configTab === 'windows' && (
                                <CodeBlock
                                    code={generateWindowsScript()}
                                    language="js"
                                    filename="PowerShell script to setup opencode.json"
                                    wrap={true}
                                    onCopy={(code) => copyToClipboard(code, 'Windows script')}
                                    maxHeight={350}
                                    minHeight={300}
                                />
                            )}
                            {configTab === 'unix' && (
                                <CodeBlock
                                    code={generateUnixScript()}
                                    language="js"
                                    filename="Bash script to setup opencode.json"
                                    wrap={true}
                                    onCopy={(code) => copyToClipboard(code, 'Unix script')}
                                    maxHeight={350}
                                    minHeight={300}
                                />
                            )}
                        </Box>
                    </Box>
                </Box>
            </DialogContent>

            <DialogActions sx={{ px: 3, pb: 2, pt: 1, justifyContent: 'flex-end' }}>
                <Button onClick={onClose} variant="contained" color="primary">
                    Done
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default OpenCodeConfigModal;

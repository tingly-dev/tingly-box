import { Box, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, Button, Typography, Tab, Tabs } from '@mui/material';
import React from 'react';
import CodeBlock from './CodeBlock';
import { useTranslation } from 'react-i18next';

interface OpenCodeConfigModalProps {
    open: boolean;
    onClose: () => void;
    // Config generators from backend
    generateConfigJson: () => string;
    generateScriptWindows: () => string;
    generateScriptUnix: () => string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
    // Apply handler
    onApply?: () => Promise<void>;
    isApplyLoading?: boolean;
    isLoading?: boolean;
}

type ScriptTab = 'json' | 'windows' | 'unix';

const OpenCodeConfigModal: React.FC<OpenCodeConfigModalProps> = ({
    open,
    onClose,
    generateConfigJson,
    generateScriptWindows,
    generateScriptUnix,
    copyToClipboard,
    onApply,
    isApplyLoading = false,
    isLoading = false,
}) => {
    const { t } = useTranslation();
    const [configTab, setConfigTab] = React.useState<ScriptTab>('json');

    // Show loading indicator
    const showLoading = isLoading || !generateConfigJson() || generateConfigJson() === '// Loading...';

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
                            {showLoading ? (
                                <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: 300 }}>
                                    <CircularProgress />
                                </Box>
                            ) : (
                                <>
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
                                    code={generateScriptWindows()}
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
                                    code={generateScriptUnix()}
                                    language="js"
                                    filename="Bash script to setup opencode.json"
                                    wrap={true}
                                    onCopy={(code) => copyToClipboard(code, 'Unix script')}
                                    maxHeight={350}
                                    minHeight={300}
                                />
                            )}
                                </>
                            )}
                        </Box>
                    </Box>
                </Box>
            </DialogContent>

            <DialogActions sx={{ px: 3, pb: 2, pt: 1, justifyContent: 'flex-end' }}>
                {onApply && (
                    <Button
                        onClick={onApply}
                        variant="contained"
                        color="primary"
                        disabled={isApplyLoading}
                        startIcon={isApplyLoading ? <CircularProgress size={16} color="inherit" /> : null}
                    >
                        {isApplyLoading ? 'Applying...' : 'Apply Configuration'}
                    </Button>
                )}
                <Button onClick={onClose} variant="contained" color="primary">
                    Done
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default OpenCodeConfigModal;

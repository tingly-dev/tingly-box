import { Box, Button, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle, Link, Tab, Tabs, Typography } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import CodeBlock from './CodeBlock';
import { isFullEdition } from '@/utils/edition';

type ConfigMode = 'unified' | 'separate' | 'smart';

interface ClaudeCodeConfigModalProps {
    open: boolean;
    onClose: () => void;
    configMode: ConfigMode;
    // Settings.json scripts
    generateSettingsConfig: () => string;
    generateSettingsScriptWindows: () => string;
    generateSettingsScriptUnix: () => string;
    // .claude.json scripts
    generateClaudeJsonConfig: () => string;
    generateScriptWindows: () => string;
    generateScriptUnix: () => string;
    // Status line scripts
    generateStatusLineConfig?: () => string;
    generateStatusLineScriptWindows?: () => string;
    generateStatusLineScriptUnix?: () => string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
    // Apply handlers
    onApply?: () => Promise<void>;
    onApplyWithStatusLine?: () => Promise<void>;
    isApplyLoading?: boolean;
}

type ScriptTab = 'json' | 'windows' | 'unix';

const ClaudeCodeConfigModal: React.FC<ClaudeCodeConfigModalProps> = ({
    open,
    onClose,
    configMode,
    generateSettingsConfig,
    generateSettingsScriptWindows,
    generateSettingsScriptUnix,
    generateClaudeJsonConfig,
    generateScriptWindows,
    generateScriptUnix,
    generateStatusLineConfig,
    generateStatusLineScriptWindows,
    generateStatusLineScriptUnix,
    copyToClipboard,
    onApply,
    onApplyWithStatusLine,
    isApplyLoading = false,
}) => {
    const { t } = useTranslation();
    const [settingsTab, setSettingsTab] = React.useState<ScriptTab>('json');
    const [claudeJsonTab, setClaudeJsonTab] = React.useState<ScriptTab>('json');
    const [statusLineTab, setStatusLineTab] = React.useState<ScriptTab>('json');

    const handleApplyClick = () => {
        if (onApply) {
            onApply();
        }
    };

    const handleApplyWithStatusLineClick = () => {
        if (onApplyWithStatusLine) {
            onApplyWithStatusLine();
        }
    };

    return (
        <Dialog
            open={open}
            onClose={(event, reason) => {
                // Only allow closing via the confirm button, not backdrop click or ESC
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
                    {t('claudeCode.modal.title')}
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                    {t('claudeCode.modal.subtitle')}
                </Typography>
            </DialogTitle>

            <DialogContent sx={{ p: 3 }}>
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
                    {/* Settings.json section */}
                    <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                        <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                            <Typography variant="subtitle2" color="text.secondary">
                                {t('claudeCode.step1')}
                            </Typography>
                            <Tabs
                                value={settingsTab}
                                onChange={(_, value) => setSettingsTab(value)}
                                variant="standard"
                                sx={{ minHeight: 32, '& .MuiTabs-indicator': { height: 3 } }}
                            >
                                <Tab label="JSON" value="json" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                <Tab label="Windows" value="windows" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                <Tab label="Linux/macOS" value="unix" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                            </Tabs>
                        </Box>
                        <Box>
                            {settingsTab === 'json' && (
                                <CodeBlock
                                    code={generateSettingsConfig()}
                                    language="json"
                                    filename="Add the env section into ~/.claude/settings.json"
                                    wrap={true}
                                    onCopy={(code) => copyToClipboard(code, 'settings.json')}
                                    maxHeight={280}
                                    minHeight={280}
                                />
                            )}
                            {settingsTab === 'windows' && (
                                <CodeBlock
                                    code={generateSettingsScriptWindows()}
                                    // bash, but use js for highlight
                                    language="js"
                                    filename="PowerShell script to setup ~/.claude/settings.json"
                                    wrap={true}
                                    onCopy={(code) => copyToClipboard(code, 'Windows script')}
                                    maxHeight={280}
                                    minHeight={280}
                                />
                            )}
                            {settingsTab === 'unix' && (
                                <CodeBlock
                                    code={generateSettingsScriptUnix()}
                                    // bash, but use js for highlight
                                    language="js"
                                    filename="Bash script to setup ~/.claude/settings.json"
                                    wrap={true}
                                    onCopy={(code) => copyToClipboard(code, 'Unix script')}
                                    maxHeight={280}
                                    minHeight={280}
                                />
                            )}
                        </Box>
                    </Box>

                    {/* .claude.json section */}
                    <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                        <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                            <Typography variant="subtitle2" color="text.secondary">
                                {t('claudeCode.step2')}
                            </Typography>
                            <Tabs
                                value={claudeJsonTab}
                                onChange={(_, value) => setClaudeJsonTab(value)}
                                variant="standard"
                                sx={{ minHeight: 32, '& .MuiTabs-indicator': { height: 3 } }}
                            >
                                <Tab label="JSON" value="json" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                <Tab label="Windows" value="windows" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                <Tab label="Linux/macOS" value="unix" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                            </Tabs>
                        </Box>
                        <Box>
                            {claudeJsonTab === 'json' && (
                                <CodeBlock
                                    code={generateClaudeJsonConfig()}
                                    language="json"
                                    filename="Set hasCompletedOnboarding as true into ~/.claude.json"
                                    wrap={true}
                                    onCopy={(code) => copyToClipboard(code, '.claude.json')}
                                    maxHeight={120}
                                    minHeight={80}
                                />
                            )}
                            {claudeJsonTab === 'windows' && (
                                <CodeBlock
                                    code={generateScriptWindows()}
                                    // bash, but use js for highlight
                                    language="js"
                                    filename="PowerShell script to setup ~/.claude.json"
                                    wrap={true}
                                    onCopy={(code) => copyToClipboard(code, 'Windows script')}
                                    maxHeight={120}
                                    minHeight={80}
                                />
                            )}
                            {claudeJsonTab === 'unix' && (
                                <CodeBlock
                                    code={generateScriptUnix()}
                                    // bash, but use js for highlight
                                    language="js"
                                    filename="Bash script to setup ~/.claude.json"
                                    wrap={true}
                                    onCopy={(code) => copyToClipboard(code, 'Unix script')}
                                    maxHeight={120}
                                    minHeight={80}
                                />
                            )}
                        </Box>
                    </Box>

                    {/* Status Line section */}
                    {(generateStatusLineConfig || generateStatusLineScriptWindows || generateStatusLineScriptUnix) && (
                        <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                            <Box sx={{ mb: 1, display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                                <Typography variant="subtitle2" color="text.secondary">
                                    {t('claudeCode.step3')}
                                </Typography>
                                <Tabs
                                    value={statusLineTab}
                                    onChange={(_, value) => setStatusLineTab(value)}
                                    variant="standard"
                                    sx={{ minHeight: 32, '& .MuiTabs-indicator': { height: 3 } }}
                                >
                                    {generateStatusLineConfig && (
                                        <Tab label="JSON" value="json" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                    )}
                                    {generateStatusLineScriptWindows && (
                                        <Tab label="Windows" value="windows" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                    )}
                                    {generateStatusLineScriptUnix && (
                                        <Tab label="Linux/macOS" value="unix" sx={{ minHeight: 32, py: 0.5, fontSize: '0.875rem' }} />
                                    )}
                                </Tabs>
                            </Box>
                            <Box>
                                {statusLineTab === 'json' && generateStatusLineConfig && (
                                    <>
                                        <Box sx={{ mb: 2 }}>
                                            <Typography variant="body2" sx={{ mb: 1 }}>
                                                {t('claudeCode.statusLine.jsonDescription')}
                                            </Typography>
                                            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                                                {t('claudeCode.statusLine.addToSettingsJson')}
                                            </Typography>
                                            <Typography variant="body2" color="text.secondary">
                                                {t('claudeCode.statusLine.manualSetup')}{' '}
                                                <Link
                                                    href="https://raw.githubusercontent.com/tingly-dev/tingly-box/refs/heads/main/internal/script/tingly-statusline.sh"
                                                    target="_blank"
                                                    rel="noopener noreferrer"
                                                >
                                                    {t('claudeCode.statusLine.downloadLink')}
                                                </Link>
                                            </Typography>
                                        </Box>
                                        <CodeBlock
                                            code={generateStatusLineConfig()}
                                            language="json"
                                            filename="Add statusLine config to ~/.claude/settings.json"
                                            wrap={true}
                                            onCopy={(code) => copyToClipboard(code, 'statusLine config')}
                                            maxHeight={200}
                                            minHeight={150}
                                        />
                                    </>
                                )}
                                {(statusLineTab === 'windows' || statusLineTab === 'unix') && (
                                    <Box sx={{ mb: 2 }}>
                                        <Typography variant="body2" sx={{ mb: 1 }}>
                                            {t('claudeCode.statusLine.description')}
                                        </Typography>
                                        <Typography variant="body2" color="text.secondary">
                                            {t('claudeCode.statusLine.manualSetup')}{' '}
                                            <Link
                                                href="https://raw.githubusercontent.com/tingly-dev/tingly-box/refs/heads/main/internal/script/tingly-statusline.sh"
                                                target="_blank"
                                                rel="noopener noreferrer"
                                            >
                                                {t('claudeCode.statusLine.downloadLink')}
                                            </Link>
                                        </Typography>
                                    </Box>
                                )}
                                {statusLineTab === 'windows' && generateStatusLineScriptWindows && (
                                    <CodeBlock
                                        code={generateStatusLineScriptWindows()}
                                        language="js"
                                        filename="PowerShell script to install status line"
                                        wrap={true}
                                        onCopy={(code) => copyToClipboard(code, 'Status line script')}
                                        maxHeight={280}
                                        minHeight={280}
                                    />
                                )}
                                {statusLineTab === 'unix' && generateStatusLineScriptUnix && (
                                    <CodeBlock
                                        code={generateStatusLineScriptUnix()}
                                        language="js"
                                        filename="Bash script to install status line"
                                        wrap={true}
                                        onCopy={(code) => copyToClipboard(code, 'Status line script')}
                                        maxHeight={280}
                                        minHeight={280}
                                    />
                                )}
                            </Box>
                        </Box>
                    )}
                </Box>
            </DialogContent>

            <DialogActions sx={{ px: 3, pb: 2, pt: 1, gap: 1, justifyContent: 'flex-end', flexWrap: 'wrap' }}>
                <Button onClick={onClose} color="inherit">
                    {t('common.cancel')}
                </Button>
                {/* Hide Apply buttons in lite edition */}
                {isFullEdition && onApply && (
                    <Button
                        onClick={handleApplyClick}
                        variant="contained"
                        disabled={isApplyLoading}
                        startIcon={isApplyLoading ? <CircularProgress size={16} color="inherit" /> : null}
                    >
                        {t('claudeCode.quickApply')}
                    </Button>
                )}
                {isFullEdition && onApplyWithStatusLine && (
                    <Button
                        onClick={handleApplyWithStatusLineClick}
                        variant="contained"
                        disabled={isApplyLoading}
                        startIcon={isApplyLoading ? <CircularProgress size={16} color="inherit" /> : null}
                    >
                        {t('claudeCode.quickApplyWithStatusLine')}
                    </Button>
                )}
            </DialogActions>
        </Dialog>
    );
};

export default ClaudeCodeConfigModal;

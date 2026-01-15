import { Box, Dialog, DialogTitle, DialogContent, DialogActions, Button, Typography, Checkbox, FormControlLabel, Tab, Tabs } from '@mui/material';
import React from 'react';
import CodeBlock from './CodeBlock';
import { useTranslation } from 'react-i18next';

type ConfigMode = 'unified' | 'separate' | 'smart';

interface ClaudeCodeConfigModalProps {
    open: boolean;
    onClose: () => void;
    dontRemindAgain: boolean;
    onDontRemindChange: (checked: boolean) => void;
    configMode: ConfigMode;
    // Settings.json scripts
    generateSettingsConfig: () => string;
    generateSettingsScriptWindows: () => string;
    generateSettingsScriptUnix: () => string;
    // .claude.json scripts
    generateClaudeJsonConfig: () => string;
    generateScriptWindows: () => string;
    generateScriptUnix: () => string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
}

type ScriptTab = 'json' | 'windows' | 'unix';

const ClaudeCodeConfigModal: React.FC<ClaudeCodeConfigModalProps> = ({
    open,
    onClose,
    dontRemindAgain,
    onDontRemindChange,
    configMode,
    generateSettingsConfig,
    generateSettingsScriptWindows,
    generateSettingsScriptUnix,
    generateClaudeJsonConfig,
    generateScriptWindows,
    generateScriptUnix,
    copyToClipboard,
}) => {
    const { t } = useTranslation();
    const [settingsTab, setSettingsTab] = React.useState<ScriptTab>('json');
    const [claudeJsonTab, setClaudeJsonTab] = React.useState<ScriptTab>('json');

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
                                    maxHeight={280}
                                    minHeight={280}
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
                                    maxHeight={280}
                                    minHeight={280}
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
                                    maxHeight={280}
                                    minHeight={280}
                                />
                            )}
                        </Box>
                    </Box>
                </Box>
            </DialogContent>

            <DialogActions sx={{ px: 3, pb: 2, pt: 1, flexDirection: 'column', alignItems: 'stretch' }}>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    {/*<FormControlLabel*/}
                    {/*    control={*/}
                    {/*        <Checkbox*/}
                    {/*            checked={dontRemindAgain}*/}
                    {/*            onChange={(e) => onDontRemindChange(e.target.checked)}*/}
                    {/*            size="small"*/}
                    {/*        />*/}
                    {/*    }*/}
                    {/*    label={t('claudeCode.modal.dontRemindAgain')}*/}
                    {/*    sx={{ mr: 0 }}*/}
                    {/*/>*/}
                    <FormControlLabel
                        control={
                            <></>
                        }
                        label={""}
                        sx={{ mr: 0 }}
                    />
                    <Button onClick={onClose} variant="contained" color="primary">
                        {t('common.confirm')}
                    </Button>
                </Box>
            </DialogActions>
        </Dialog>
    );
};

export default ClaudeCodeConfigModal;

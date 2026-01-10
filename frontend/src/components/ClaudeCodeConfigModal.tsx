import { Box, Dialog, DialogTitle, DialogContent, DialogActions, Button, Typography, ToggleButton, ToggleButtonGroup, Checkbox, FormControlLabel } from '@mui/material';
import React from 'react';
import CodeBlock from './CodeBlock';
import { useTranslation } from 'react-i18next';

type ClaudeJsonMode = 'json' | 'script';
type ConfigMode = 'unified' | 'separate';

interface ClaudeCodeConfigModalProps {
    open: boolean;
    onClose: () => void;
    dontRemindAgain: boolean;
    onDontRemindChange: (checked: boolean) => void;
    // Content generation props
    claudeJsonMode: ClaudeJsonMode;
    onClaudeJsonModeChange: (mode: ClaudeJsonMode) => void;
    configMode: ConfigMode;
    generateSettingsConfig: () => string;
    generateSettingsScript: () => string;
    generateClaudeJsonConfig: () => string;
    generateScript: () => string;
    copyToClipboard: (text: string, label: string) => Promise<void>;
}

const ClaudeCodeConfigModal: React.FC<ClaudeCodeConfigModalProps> = ({
    open,
    onClose,
    dontRemindAgain,
    onDontRemindChange,
    claudeJsonMode,
    onClaudeJsonModeChange,
    configMode,
    generateSettingsConfig,
    generateSettingsScript,
    generateClaudeJsonConfig,
    generateScript,
    copyToClipboard,
}) => {
    const { t } = useTranslation();

    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth="lg"
            fullWidth
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
                <Box sx={{ display: 'flex', flexDirection: { xs: 'column', md: 'row' }, gap: 2 }}>
                    {/* Settings.json section */}
                    <Box sx={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column' }}>
                        <Box sx={{ mb: 1 }}>
                            <Typography variant="subtitle2" color="text.secondary">
                                {t('claudeCode.step1')}
                            </Typography>
                        </Box>
                        <Box sx={{ flex: 1 }}>
                            <CodeBlock
                                code={claudeJsonMode === 'json' ? generateSettingsConfig() : generateSettingsScript()}
                                language={claudeJsonMode === 'json' ? 'json' : 'js'}
                                filename={claudeJsonMode === 'json' ? 'Add the env section into ~/.claude/setting.json' : 'Script to setup ~/.claude/settings.json'}
                                wrap={true}
                                onCopy={(code) => copyToClipboard(code, claudeJsonMode === 'json' ? 'settings.json' : 'script')}
                                maxHeight={280}
                                minHeight={280}
                                headerActions={
                                    <ToggleButtonGroup
                                        value={claudeJsonMode}
                                        exclusive
                                        size="small"
                                        onChange={(_, value) => value && onClaudeJsonModeChange(value)}
                                        sx={{ bgcolor: 'grey.700', '& .MuiToggleButton-root': { color: 'grey.300', padding: '2px 8px', fontSize: '0.75rem' } }}
                                    >
                                        <ToggleButton value="json" sx={{ '&.Mui-selected': { bgcolor: 'primary.main', color: 'white' } }}>
                                            JSON
                                        </ToggleButton>
                                        <ToggleButton value="script" sx={{ '&.Mui-selected': { bgcolor: 'primary.main', color: 'white' } }}>
                                            Script
                                        </ToggleButton>
                                    </ToggleButtonGroup>
                                }
                            />
                        </Box>
                    </Box>

                    {/* .claude.json section */}
                    <Box sx={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column' }}>
                        <Box sx={{ mb: 1 }}>
                            <Typography variant="subtitle2" color="text.secondary">
                                {t('claudeCode.step2')}
                            </Typography>
                        </Box>
                        <Box sx={{ flex: 1 }}>
                            <CodeBlock
                                code={claudeJsonMode === 'json' ? generateClaudeJsonConfig() : generateScript()}
                                language={claudeJsonMode === 'json' ? 'json' : 'js'}
                                filename={claudeJsonMode === 'json' ? 'Set hasCompletedOnboarding into ~/.claude.json' : 'Script to setup ~/.claude.json'}
                                wrap={true}
                                onCopy={(code) => copyToClipboard(code, claudeJsonMode === 'json' ? '.claude.json' : 'script')}
                                maxHeight={280}
                                minHeight={280}
                                headerActions={
                                    <ToggleButtonGroup
                                        value={claudeJsonMode}
                                        exclusive
                                        size="small"
                                        onChange={(_, value) => value && onClaudeJsonModeChange(value)}
                                        sx={{ bgcolor: 'grey.700', '& .MuiToggleButton-root': { color: 'grey.300', padding: '2px 8px', fontSize: '0.75rem' } }}
                                    >
                                        <ToggleButton value="json" sx={{ '&.Mui-selected': { bgcolor: 'primary.main', color: 'white' } }}>
                                            JSON
                                        </ToggleButton>
                                        <ToggleButton value="script" sx={{ '&.Mui-selected': { bgcolor: 'primary.main', color: 'white' } }}>
                                            Script
                                        </ToggleButton>
                                    </ToggleButtonGroup>
                                }
                            />
                        </Box>
                    </Box>
                </Box>
            </DialogContent>

            <DialogActions sx={{ px: 3, pb: 2, pt: 1, flexDirection: 'column', alignItems: 'stretch' }}>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <FormControlLabel
                        control={
                            <Checkbox
                                checked={dontRemindAgain}
                                onChange={(e) => onDontRemindChange(e.target.checked)}
                                size="small"
                            />
                        }
                        label={t('claudeCode.modal.dontRemindAgain')}
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

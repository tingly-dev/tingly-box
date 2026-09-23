import { Box, CircularProgress, DialogActions, DialogContent, Button, Typography } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { isFullEdition } from '@/utils/edition';
import { ConfigModalShell } from './config/ConfigModalShell';
import { ManualFileSection } from './config/ManualFileSection';

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

    // Show loading indicator
    const showLoading = isLoading || !generateConfigJson() || generateConfigJson() === '// Loading...';

    return (
        <ConfigModalShell
            open={open}
            onClose={onClose}
            title={t('openCodeConfig.title')}
            subtitle={t('openCodeConfig.subtitle')}
        >
            <DialogContent sx={{ p: 3 }}>
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                    {/* Config file location info */}
                    <Box sx={{ p: 2, bgcolor: 'info.50', borderRadius: 1 }}>
                        <Typography variant="body2" sx={{
                            color: "info.dark"
                        }}>
                            <strong>{t('openCodeConfig.configLocation')}</strong> ~/.config/opencode/opencode.json
                        </Typography>
                    </Box>

                    {/* Config section */}
                    <ManualFileSection
                        heading={t('openCodeConfig.configurationTitle')}
                        copyToClipboard={copyToClipboard}
                        loading={showLoading ? (
                            <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: 300 }}>
                                <CircularProgress />
                            </Box>
                        ) : null}
                        tabs={[
                            {
                                label: 'JSON',
                                value: 'json',
                                code: generateConfigJson(),
                                language: 'json',
                                filename: '~/.config/opencode/opencode.json',
                                copyLabel: 'opencode.json',
                                maxHeight: 350,
                                minHeight: 300,
                            },
                            {
                                label: 'Windows',
                                value: 'windows',
                                code: generateScriptWindows(),
                                language: 'js',
                                filename: 'PowerShell script to setup opencode.json',
                                copyLabel: 'Windows script',
                                maxHeight: 350,
                                minHeight: 300,
                            },
                            {
                                label: 'Linux/macOS',
                                value: 'unix',
                                code: generateScriptUnix(),
                                language: 'js',
                                filename: 'Bash script to setup opencode.json',
                                copyLabel: 'Unix script',
                                maxHeight: 350,
                                minHeight: 300,
                            },
                        ]}
                    />
                </Box>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2, pt: 1, gap: 1, justifyContent: 'flex-end' }}>
                <Button onClick={onClose} color="inherit">
                    {t('common.cancel')}
                </Button>
                {/* Hide Apply button in lite edition */}
                {isFullEdition && onApply && (
                    <Button
                        onClick={onApply}
                        variant="contained"
                        disabled={isApplyLoading}
                        startIcon={isApplyLoading ? <CircularProgress size={16} color="inherit" /> : null}
                    >
                        {isApplyLoading ? t('common.applying') : t('scenarioPage.autoConfig')}
                    </Button>
                )}
            </DialogActions>
        </ConfigModalShell>
    );
};

export default OpenCodeConfigModal;

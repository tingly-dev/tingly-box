import { Box, Button, Dialog, DialogContent, DialogTitle, IconButton, Stack, Typography } from '@mui/material';
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Close, Launch } from '@/components/icons';
import { Claude, OpenAI } from '@/components/BrandIcons';
import OAuthDialog from '@/components/OAuthDialog';
import { useNotify } from '@/hooks/useNotify.ts';

export type OAuthGuideAgent = 'claude_code' | 'codex';

const AGENT_META: Record<OAuthGuideAgent, { name: string; icon: React.ReactNode; color: string; agentPath: string }> = {
    claude_code: { name: 'Claude Code', icon: <Claude size={28} />, color: '#D97757', agentPath: '/agent/claude_code' },
    codex: { name: 'Codex', icon: <OpenAI size={28} />, color: '#10A37F', agentPath: '/agent/codex' },
};

interface OAuthGuideDialogProps {
    open: boolean;
    onClose: () => void;
    agent: OAuthGuideAgent;
}

/**
 * Per-agent OAuth walkthrough, launched from HelpPage. Claude Code and Codex
 * each get their own instance of this dialog (not a shared tab picker — the
 * two flows share no state, so a mode picker here would just be an extra
 * click, see .design/ux-principles.md #2) — only which agentPath/branding to
 * use differs, so the two steps are shared as one parameterized component.
 *
 * The two steps mirror the real two-step mechanism (.design/oauth.md):
 * connecting the OAuth credential (handled here, via the same OAuthDialog
 * used everywhere else) and pointing the actual CLI at Tingly Box (handled
 * on the agent's own setup page, via AgentSetupCard — not rebuilt here).
 */
export const OAuthGuideDialog = ({ open, onClose, agent }: OAuthGuideDialogProps) => {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const notify = useNotify();
    const meta = AGENT_META[agent];
    const [oauthDialogOpen, setOAuthDialogOpen] = useState(false);

    const handleConnected = () => {
        setOAuthDialogOpen(false);
        notify.success(t('help.oauth.guide.connected', { agent: meta.name }));
    };

    const handleOpenSetup = () => {
        onClose();
        navigate(meta.agentPath);
    };

    return (
        <>
            <Dialog open={open && !oauthDialogOpen} onClose={onClose} maxWidth="sm" fullWidth>
                <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                    <Box
                        sx={{
                            width: 40, height: 40, borderRadius: 2, flexShrink: 0,
                            display: 'flex', alignItems: 'center', justifyContent: 'center',
                            bgcolor: `${meta.color}15`,
                        }}
                    >
                        {meta.icon}
                    </Box>
                    <Box sx={{ flex: 1, minWidth: 0 }}>
                        <Typography variant="h6">{t('help.oauth.guide.title', { agent: meta.name })}</Typography>
                    </Box>
                    <IconButton onClick={onClose} size="small" aria-label={t('common.close', { defaultValue: 'Close' })}>
                        <Close />
                    </IconButton>
                </DialogTitle>
                <DialogContent>
                    <Stack spacing={2.5} sx={{ pb: 1 }}>
                        <Typography variant="body2" sx={{ color: 'text.secondary' }}>
                            {t('help.oauth.guide.intro', { agent: meta.name })}
                        </Typography>

                        <Stack direction="row" spacing={1.5}>
                            <StepNumber n={1} />
                            <Box sx={{ flex: 1 }}>
                                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                                    {t('help.oauth.guide.step1.title', { agent: meta.name })}
                                </Typography>
                                <Typography variant="body2" sx={{ color: 'text.secondary', mb: 1 }}>
                                    {t('help.oauth.guide.step1.description', { agent: meta.name })}
                                </Typography>
                                <Button variant="contained" size="small" onClick={() => setOAuthDialogOpen(true)}>
                                    {t('help.oauth.guide.step1.action')}
                                </Button>
                            </Box>
                        </Stack>

                        <Stack direction="row" spacing={1.5}>
                            <StepNumber n={2} />
                            <Box sx={{ flex: 1 }}>
                                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                                    {t('help.oauth.guide.step2.title', { agent: meta.name })}
                                </Typography>
                                <Typography variant="body2" sx={{ color: 'text.secondary', mb: 1 }}>
                                    {t('help.oauth.guide.step2.description', { agent: meta.name })}
                                </Typography>
                                <Button variant="outlined" size="small" endIcon={<Launch fontSize="small" />} onClick={handleOpenSetup}>
                                    {t('help.oauth.guide.step2.action', { agent: meta.name })}
                                </Button>
                            </Box>
                        </Stack>
                    </Stack>
                </DialogContent>
            </Dialog>

            <OAuthDialog
                open={oauthDialogOpen}
                onClose={() => setOAuthDialogOpen(false)}
                onSuccess={handleConnected}
                autoStartProviderId={agent}
            />
        </>
    );
};

const StepNumber = ({ n }: { n: number }) => (
    <Box sx={{
        width: 22, height: 22, borderRadius: '50%', flexShrink: 0, mt: 0.25,
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        bgcolor: 'action.selected', color: 'text.secondary',
        fontSize: '0.7rem', fontWeight: 700,
    }}>
        {n}
    </Box>
);

export default OAuthGuideDialog;

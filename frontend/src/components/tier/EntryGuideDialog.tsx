import {
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    Paper,
    Step,
    StepLabel,
    Stepper,
    Typography,
    useMediaQuery,
    useTheme,
} from '@mui/material';
import { Close as CloseIcon } from '@/components/icons';
import React, { useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { AGENT_APPLY_STEP, AGENT_INSTALL_STEP, AGENT_VERIFY_STEP, ROUTING_GUIDE_STEPS, type GuideAgent } from './diagrams';
import { StaticGraphViewer } from './StaticGraphViewer';
import { GuideToolbarPreview } from './GuideToolbarPreview';
import { GuideLanguageToggle } from './GuideLanguageToggle';
import { fontMono, fontSizes } from '@/theme/fonts';

export interface EntryGuideDialogProps {
    open: boolean;
    onClose: () => void;
    mode?: 'direct' | 'smart';
    /** An agent page's guide: install, connect, add a model, apply, verify, then the routing steps. */
    agent?: GuideAgent;
}

export const EntryGuideDialog: React.FC<EntryGuideDialogProps> = ({
    open,
    onClose,
    mode = 'direct',
    agent,
}) => {
    const { t } = useTranslation();
    const theme = useTheme();
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
    const triggerRef = useRef<HTMLElement | null>(null);

    // Steps for the current mode are the source of truth; activeStep is a plain
    // local index into this list (no array-offset arithmetic).
    const filteredSteps = React.useMemo(() => {
        const routing = ROUTING_GUIDE_STEPS.filter((step) => step.mode === mode);
        if (!agent || mode !== 'direct') return routing;
        // Connect AI and Add model are the routing guide's own first two steps.
        return [AGENT_INSTALL_STEP, ...routing.slice(0, 2), AGENT_APPLY_STEP, AGENT_VERIFY_STEP, ...routing.slice(2)];
    }, [mode, agent]);
    const stepParams = { agent: agent?.name ?? '' };
    const [activeStep, setActiveStep] = React.useState(0);
    const totalSteps = filteredSteps.length;
    const safeActiveStep = Math.min(activeStep, totalSteps - 1);
    const currentStep = filteredSteps[safeActiveStep] || filteredSteps[0];

    // Store trigger element for focus restoration
    React.useEffect(() => {
        if (open && !triggerRef.current) {
            triggerRef.current = document.activeElement as HTMLElement;
        }
        return () => {
            triggerRef.current = null;
        };
    }, [open]);

    // Handle keyboard escape
    React.useEffect(() => {
        const handleEscape = (e: KeyboardEvent) => {
            if (e.key === 'Escape' && open) {
                handleClose();
            }
        };
        document.addEventListener('keydown', handleEscape);
        return () => document.removeEventListener('keydown', handleEscape);
    }, [open]);

    // Restart from the first step whenever the mode changes or the dialog opens.
    React.useEffect(() => {
        if (open) setActiveStep(0);
    }, [mode, open]);

    const handleNext = () => {
        if (activeStep < totalSteps - 1) {
            setActiveStep(activeStep + 1);
        } else {
            handleClose();
        }
    };

    const handleBack = () => {
        setActiveStep(Math.max(0, activeStep - 1));
    };

    const handleClose = () => {
        setActiveStep(0);
        onClose();
        // Restore focus to trigger element
        if (triggerRef.current) {
            triggerRef.current.focus();
        }
    };

    const handleStepChange = (step: number) => {
        setActiveStep(step);
    };

    const handleKeyDown = (e: React.KeyboardEvent) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            handleNext();
        }
    };

    const guideTitle = agent && mode === 'direct'
        ? t('rule.routing.guide.connectTitle', { agent: agent.name })
        : mode === 'direct'
        ? t('rule.routing.guide.directTitle', { defaultValue: 'Direct Routing Guide' })
        : t('rule.routing.guide.smartTitle', { defaultValue: 'Smart Routing Guide' });

    return (
        <Dialog
            open={open}
            onClose={handleClose}
            fullScreen={fullScreen}
            maxWidth="lg"
            aria-labelledby="entry-guide-dialog-title"
            aria-describedby="entry-guide-dialog-description"
            onKeyDown={handleKeyDown}
            slotProps={{
                paper: {
                    sx: {
                        borderRadius: fullScreen ? 0 : 2,
                        maxHeight: '90vh',
                        width: fullScreen ? '100%' : '90vw',
                        maxWidth: fullScreen ? '100vw' : '900px',
                    }
                }
            }}
        >
            <DialogTitle id="entry-guide-dialog-title" sx={{ display: 'flex', alignItems: 'flex-start', gap: 2, pr: 8 }}>
                <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Typography variant="h6" component="div">
                        {guideTitle}
                    </Typography>
                    <Typography variant="caption" sx={{
                        color: "text.secondary"
                    }}>
                        {t('rule.routing.guide.subtitle', { defaultValue: 'Step {{current}} of {{total}}', current: safeActiveStep + 1, total: totalSteps })}
                    </Typography>
                </Box>
                <Box sx={{ pt: 0.5 }}>
                    <GuideLanguageToggle />
                </Box>
            </DialogTitle>
            <IconButton
                aria-label={t('common.close', { defaultValue: 'Close' })}
                onClick={handleClose}
                sx={{
                    position: 'absolute',
                    right: 8,
                    top: 8,
                    color: (theme) => theme.palette.grey[500],
                }}
            >
                <CloseIcon />
            </IconButton>
            <DialogContent id="entry-guide-dialog-description" dividers sx={{ p: 0 }}>
                <Box sx={{ display: 'flex', flexDirection: { xs: 'column', sm: 'row' }, height: '100%' }}>
                    {/* Left side: step navigation rail */}
                    <Box sx={{
                        width: { xs: '100%', sm: 240 },
                        py: { xs: 2, sm: 3 },
                        px: { xs: 2, sm: 3 },
                        display: 'flex',
                        flexDirection: { xs: 'row', sm: 'column' },
                        alignItems: { xs: 'center', sm: 'stretch' },
                        overflowX: { xs: 'auto', sm: 'visible' },
                        flexShrink: 0,
                        bgcolor: 'background.default',
                        borderRight: { xs: 'none', sm: '1px solid' },
                        borderBottom: { xs: '1px solid', sm: 'none' },
                        borderColor: { xs: 'divider', sm: 'divider' },
                    }}>
                        <Stepper
                            activeStep={safeActiveStep}
                            orientation={fullScreen ? 'horizontal' : 'vertical'}
                            sx={{
                                '& .MuiStepLabel-root': {
                                    cursor: 'pointer',
                                },
                                '& .Mui-completed,& .Mui-active': {
                                    '& .MuiStepLabel-iconContainer': {
                                        color: 'primary.main',
                                    },
                                },
                            }}
                        >
                            {filteredSteps.map((step, index) => (
                                <Step key={index} onClick={() => handleStepChange(index)}>
                                    <StepLabel>
                                        {fullScreen ? '' : t(step.title, { defaultValue: `Step ${index + 1}`, ...stepParams })}
                                    </StepLabel>
                                </Step>
                            ))}
                        </Stepper>
                    </Box>

                    {/* Right side: Content */}
                    <Box sx={{
                        flex: 1,
                        display: 'flex',
                        flexDirection: 'column',
                        minWidth: 0,
                        py: { xs: 2, sm: 3 },
                        px: { xs: 2, sm: 4 },
                    }}>
                        {/* Explanation Text */}
                        <Box sx={{ mb: 2.5 }}>
                            <Typography variant="body1" sx={{ lineHeight: 1.8 }}>
                                {t(currentStep.content, { defaultValue: currentStep.content, ...stepParams })}
                            </Typography>

                            {/* Annotations */}
                            {currentStep.annotations && currentStep.annotations.length > 0 && (
                                <Box sx={{ mt: 2, display: 'flex', flexWrap: 'wrap', gap: 1 }}>
                                    {currentStep.annotations.map((annotation, idx) => (
                                        <Paper
                                            key={idx}
                                            variant="outlined"
                                            sx={{
                                                p: 1.5,
                                                bgcolor: 'action.hover',
                                                borderRadius: 1,
                                                flex: '1 1 45%',
                                            }}
                                        >
                                            <Typography variant="caption" color="primary" sx={{ fontWeight: 600, display: 'block', mb: 0.5 }}>
                                                {t(annotation.text, { defaultValue: annotation.text })}
                                            </Typography>
                                        </Paper>
                                    ))}
                                </Box>
                            )}
                        </Box>

                        {currentStep.panel ? (
                            <Box sx={{ bgcolor: 'background.default', p: { xs: 2, sm: 3 }, borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
                                {currentStep.panel === 'install' && agent && (
                                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5 }}>
                                        {[
                                            { label: t('rule.routing.guide.connect.official'), command: agent.installCommand },
                                            { label: t('rule.routing.guide.connect.mirror'), command: agent.installMirrorCommand },
                                        ].filter((row) => row.command).map((row) => (
                                            <Box key={row.label}>
                                                <Typography variant="caption" sx={{ color: 'text.secondary' }}>{row.label}</Typography>
                                                <Box sx={{ fontFamily: fontMono, fontSize: fontSizes.sm, p: 1, borderRadius: 1, bgcolor: 'action.hover', wordBreak: 'break-all' }}>
                                                    {row.command}
                                                </Box>
                                            </Box>
                                        ))}
                                        {agent.siteUrl && (
                                            <Button href={agent.siteUrl} target="_blank" rel="noopener noreferrer" size="small" variant="text" sx={{ alignSelf: 'flex-start' }}>
                                                {t('rule.routing.guide.connect.website', { agent: agent.name })}
                                            </Button>
                                        )}
                                    </Box>
                                )}
                                {currentStep.panel === 'apply' && (
                                    <Typography variant="body2" sx={{ color: 'text.secondary', lineHeight: 1.8 }}>
                                        {t('rule.routing.guide.connect.applyHint', stepParams)}
                                    </Typography>
                                )}
                                {currentStep.panel === 'verify' && (
                                    <Typography variant="body2" sx={{ color: 'text.secondary', lineHeight: 1.8 }}>
                                        {t('rule.routing.guide.connect.verifyHint', stepParams)}
                                    </Typography>
                                )}
                            </Box>
                        ) : (
                        <>
                        {/* Static Graph Diagram */}
                        <Box sx={{
                            bgcolor: 'background.default',
                            p: { xs: 2, sm: 3 },
                            display: 'flex',
                            flexDirection: 'column',
                            alignItems: 'center',
                            minHeight: 280,
                            maxHeight: 400,
                            overflow: 'auto',
                            borderRadius: 1,
                            border: '1px solid',
                            borderColor: 'divider',
                            position: 'relative',
                        }}>
                            {currentStep.toolbarHighlight && (
                                <GuideToolbarPreview highlight={currentStep.toolbarHighlight} />
                            )}
                            <Box sx={{ width: '100%', maxWidth: 700 }}>
                                <StaticGraphViewer
                                    scenario={currentStep.diagram}
                                    interactive={true}
                                />
                            </Box>
                            {/* Hover hint */}
                            <Box sx={{
                                position: 'absolute',
                                bottom: 8,
                                right: 8,
                                bgcolor: 'rgba(0,0,0,0.7)',
                                color: 'white',
                                px: 1.5,
                                py: 0.5,
                                borderRadius: 1,
                                fontSize: fontSizes.sm,
                                opacity: 0.8,
                            }}>
                                💡 {t('rule.routing.guide.hoverHint', { defaultValue: 'Hover over a node in the diagram to see its actions' })}
                            </Box>
                        </Box>
                        </>
                        )}
                    </Box>
                </Box>
            </DialogContent>
            <DialogActions sx={{ justifyContent: 'flex-end', gap: 1.5, px: { xs: 2, sm: 3 }, py: 2 }}>
                <Button
                    disabled={safeActiveStep === 0}
                    onClick={handleBack}
                    variant="outlined"
                    size="small"
                    sx={{ minWidth: 100 }}
                >
                    {t('rule.routing.guide.previous', { defaultValue: 'Previous' })}
                </Button>
                <Button
                    onClick={handleNext}
                    variant="contained"
                    size="small"
                    sx={{ minWidth: 100 }}
                >
                    {safeActiveStep === totalSteps - 1
                        ? t('rule.routing.guide.gotIt', { defaultValue: 'Got it!' })
                        : t('rule.routing.guide.next', { defaultValue: 'Next' })
                    }
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default EntryGuideDialog;
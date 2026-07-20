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
import { ROUTING_GUIDE_STEPS } from './diagrams';
import { StaticGraphViewer } from './StaticGraphViewer';
import { GuideToolbarPreview } from './GuideToolbarPreview';
import { GuideLanguageToggle } from './GuideLanguageToggle';

export interface EntryGuideDialogProps {
    open: boolean;
    onClose: () => void;
    mode?: 'direct' | 'smart';
}

export const EntryGuideDialog: React.FC<EntryGuideDialogProps> = ({
    open,
    onClose,
    mode = 'direct',
}) => {
    const { t } = useTranslation();
    const theme = useTheme();
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
    const triggerRef = useRef<HTMLElement | null>(null);

    // Steps for the current mode are the source of truth; activeStep is a plain
    // local index into this list (no array-offset arithmetic).
    const filteredSteps = React.useMemo(
        () => ROUTING_GUIDE_STEPS.filter((step) => step.mode === mode),
        [mode],
    );
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

    const guideTitle = mode === 'direct'
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
                <Box sx={{ display: 'flex', flexDirection: 'row', height: '100%', gap: 2 }}>
                    {/* Left side: Vertical Stepper */}
                    <Box sx={{
                        width: { xs: '100%', sm: 220 },
                        py: { xs: 2, sm: 3 },
                        px: { xs: 2, sm: 1 },
                        display: 'flex',
                        flexDirection: { xs: 'row', sm: 'column' },
                        alignItems: { xs: 'center', sm: 'flex-start' },
                        overflowX: { xs: 'auto', sm: 'visible' },
                        flexShrink: 0,
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
                                        {fullScreen ? '' : t(step.title, { defaultValue: `Step ${index + 1}` })}
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
                        px: { xs: 2, sm: 3 },
                    }}>
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
                                fontSize: '0.75rem',
                                opacity: 0.8,
                            }}>
                                💡 {t('rule.routing.guide.hoverHint', { defaultValue: 'Action buttons shown - try hovering over nodes!' })}
                            </Box>
                        </Box>

                        {/* Explanation Text */}
                        <Box sx={{ mt: 2, p: { xs: 2, sm: 3 }, bgcolor: 'background.default', borderRadius: 1 }}>
                            <Typography variant="body1" sx={{ lineHeight: 1.8 }}>
                                {t(currentStep.content, { defaultValue: currentStep.content })}
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
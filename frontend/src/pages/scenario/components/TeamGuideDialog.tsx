import {
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    List,
    ListItem,
    ListItemText,
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
import { GuideLanguageToggle } from '@/components/tier/GuideLanguageToggle';

export interface TeamGuideDialogProps {
    open: boolean;
    onClose: () => void;
}

// Concept, not routing: this walks through what a Team is, how it's isolated
// from other Teams/scenarios, how to mint a Sharing Key, and how a client
// actually uses one. It deliberately has no diagram — the routing/tier guide
// (EntryGuideDialog, opened from the rule list's own "?") already covers how
// the rules below route requests, so this doesn't repeat that.
const TEAM_GUIDE_STEP_IDS = ['usage', 'isolation', 'keys', 'principles'] as const;

export const TeamGuideDialog: React.FC<TeamGuideDialogProps> = ({ open, onClose }) => {
    const { t } = useTranslation();
    const theme = useTheme();
    const fullScreen = useMediaQuery(theme.breakpoints.down('sm'));
    const [activeStep, setActiveStep] = React.useState(0);
    const totalSteps = TEAM_GUIDE_STEP_IDS.length;
    const stepId = TEAM_GUIDE_STEP_IDS[activeStep];
    const triggerRef = useRef<HTMLElement | null>(null);

    React.useEffect(() => {
        if (open && !triggerRef.current) {
            triggerRef.current = document.activeElement as HTMLElement;
        }
        return () => {
            triggerRef.current = null;
        };
    }, [open]);

    React.useEffect(() => {
        const handleEscape = (e: KeyboardEvent) => {
            if (e.key === 'Escape' && open) handleClose();
        };
        document.addEventListener('keydown', handleEscape);
        return () => document.removeEventListener('keydown', handleEscape);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [open]);

    React.useEffect(() => {
        if (open) setActiveStep(0);
    }, [open]);

    const handleClose = () => {
        setActiveStep(0);
        onClose();
        if (triggerRef.current) triggerRef.current.focus();
    };

    const handleNext = () => {
        if (activeStep < totalSteps - 1) setActiveStep(activeStep + 1);
        else handleClose();
    };

    const handleBack = () => setActiveStep(Math.max(0, activeStep - 1));

    const handleKeyDown = (e: React.KeyboardEvent) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            handleNext();
        }
    };

    // Up to 3 bullets per step; missing keys just render nothing.
    const bulletKeys = ['bullet1', 'bullet2', 'bullet3']
        .map((suffix) => `teams.guide.steps.${stepId}.${suffix}`)
        .filter((key) => t(key, { defaultValue: '' }) !== '');

    return (
        <Dialog
            open={open}
            onClose={handleClose}
            fullScreen={fullScreen}
            maxWidth="sm"
            aria-labelledby="team-guide-dialog-title"
            aria-describedby="team-guide-dialog-description"
            onKeyDown={handleKeyDown}
            slotProps={{
                paper: {
                    sx: {
                        borderRadius: fullScreen ? 0 : 2,
                        maxHeight: '90vh',
                        width: fullScreen ? '100%' : '90vw',
                        maxWidth: fullScreen ? '100vw' : '640px',
                    },
                },
            }}
        >
            <DialogTitle id="team-guide-dialog-title" sx={{ display: 'flex', alignItems: 'flex-start', gap: 2, pr: 8 }}>
                <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Typography variant="h6" component="div">
                        {t(`teams.guide.steps.${stepId}.title`)}
                    </Typography>
                    <Typography variant="caption" sx={{ color: 'text.secondary' }}>
                        {t('teams.guide.subtitle', { current: activeStep + 1, total: totalSteps })}
                    </Typography>
                </Box>
                <Box sx={{ pt: 0.5 }}>
                    <GuideLanguageToggle />
                </Box>
            </DialogTitle>
            <IconButton
                aria-label={t('common.close', { defaultValue: 'Close' })}
                onClick={handleClose}
                sx={{ position: 'absolute', right: 8, top: 8, color: (theme) => theme.palette.grey[500] }}
            >
                <CloseIcon />
            </IconButton>
            <DialogContent id="team-guide-dialog-description" dividers>
                <Stepper
                    activeStep={activeStep}
                    alternativeLabel={!fullScreen}
                    orientation="horizontal"
                    sx={{
                        mb: 3,
                        '& .MuiStepLabel-root': { cursor: 'pointer' },
                        '& .Mui-completed, & .Mui-active': {
                            '& .MuiStepLabel-iconContainer': { color: 'primary.main' },
                        },
                    }}
                >
                    {TEAM_GUIDE_STEP_IDS.map((id, index) => (
                        <Step key={id} onClick={() => setActiveStep(index)}>
                            <StepLabel>{fullScreen ? '' : t(`teams.guide.steps.${id}.title`)}</StepLabel>
                        </Step>
                    ))}
                </Stepper>

                <Typography variant="body1" sx={{ lineHeight: 1.8 }}>
                    {t(`teams.guide.steps.${stepId}.content`)}
                </Typography>

                {bulletKeys.length > 0 && (
                    <List dense sx={{ mt: 1, listStyleType: 'disc', pl: 3 }}>
                        {bulletKeys.map((key) => (
                            <ListItem key={key} sx={{ display: 'list-item', p: 0, mb: 0.75 }}>
                                <ListItemText
                                    primary={t(key)}
                                    slotProps={{ primary: { variant: 'body2', sx: { lineHeight: 1.7 } } }}
                                />
                            </ListItem>
                        ))}
                    </List>
                )}
            </DialogContent>
            <DialogActions sx={{ justifyContent: 'flex-end', gap: 1.5, px: { xs: 2, sm: 3 }, py: 2 }}>
                <Button disabled={activeStep === 0} onClick={handleBack} variant="outlined" size="small" sx={{ minWidth: 100 }}>
                    {t('teams.guide.previous')}
                </Button>
                <Button onClick={handleNext} variant="contained" size="small" sx={{ minWidth: 100 }}>
                    {activeStep === totalSteps - 1 ? t('teams.guide.gotIt') : t('teams.guide.next')}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default TeamGuideDialog;

import {useState} from 'react';
import {useNavigate} from 'react-router-dom';
import {useTranslation} from 'react-i18next';
import {
    Alert,
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogContentText,
    DialogTitle,
    Snackbar,
} from '@mui/material';
import PageLayout from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import ConnectAIDialogs from '@/components/ConnectAIDialogs';
import {ProviderListContent} from '@/components/ConnectProviderDialog';
import {useProviderDialog} from '@/hooks/useProviderDialog';

const Onboarding: React.FC = () => {
    const {t} = useTranslation();
    const navigate = useNavigate();
    const [browseQuery, setBrowseQuery] = useState('');

    const [snackbar, setSnackbar] = useState<{open: boolean; message: string; severity: 'success' | 'error' | 'info'}>({
        open: false,
        message: '',
        severity: 'info',
    });
    const [successDialogOpen, setSuccessDialogOpen] = useState(false);

    const showMessage = (message: string, severity: 'success' | 'error' | 'info' = 'info') => {
        setSnackbar({open: true, message, severity});
    };

    // The same Connect AI flow every other surface uses: the provider list is
    // rendered inline (page content instead of a picker dialog), and every
    // card — key / custom / self-hosted / OAuth / import / paste & detect —
    // routes through the shared hook + dialog stack.
    const connectAI = useProviderDialog(showMessage, {
        onProviderAdded: () => setSuccessDialogOpen(true),
    });

    const handleGoToAgents = () => {
        setSuccessDialogOpen(false);
        navigate('/agent');
    };

    const handleStayOnOnboarding = () => {
        setSuccessDialogOpen(false);
        showMessage(t('onboarding.success', {defaultValue: 'Provider added successfully! You can now create scenarios.'}), 'success');
    };

    return (
        <PageLayout loading={false}>
            <Box sx={{py: 3, px: {xs: 2, md: 3}}}>
                <UnifiedCard
                    size="full"
                    title={t('onboarding.title', {defaultValue: 'Welcome to Tingly Box'})}
                    titleHeadingLevel={1}
                    subtitle={t('onboarding.subtitle', {
                        defaultValue: 'Add your first AI provider to get started. Browse the catalog, or use Paste & detect with a config snippet — we\'ll figure out the rest.',
                    })}
                >
                    <ProviderListContent
                        onSelect={connectAI.handleConnectSelect}
                        query={browseQuery}
                        onQueryChange={setBrowseQuery}
                        hideOfficialInfo={true}
                        showDetails={true}
                        wide={true}
                    />
                </UnifiedCard>
            </Box>

            {/* Shared Connect AI dialog stack (form / OAuth / paste / import).
                inline: the provider list above is the picker, so no picker
                dialog and no "← Back to picker" in the form. */}
            <ConnectAIDialogs flow={connectAI} inline isFirstProvider/>

            <Dialog
                open={successDialogOpen}
                onClose={() => setSuccessDialogOpen(false)}
                aria-labelledby="success-dialog-title"
                aria-describedby="success-dialog-description"
            >
                <DialogTitle id="success-dialog-title">
                    {t('onboarding.dialog.title', {defaultValue: 'Provider Added'})}
                </DialogTitle>
                <DialogContent>
                    <DialogContentText id="success-dialog-description">
                        {t('onboarding.dialog.message', {defaultValue: 'Your AI provider has been added successfully. Would you like to go to the agents page to start using it?'})}
                    </DialogContentText>
                </DialogContent>
                <DialogActions>
                    <Button onClick={handleStayOnOnboarding}>
                        {t('onboarding.dialog.stay', {defaultValue: 'Stay Here'})}
                    </Button>
                    <Button onClick={handleGoToAgents} variant="contained" autoFocus>
                        {t('onboarding.dialog.goToAgents', {defaultValue: 'Go to Agents'})}
                    </Button>
                </DialogActions>
            </Dialog>

            <Snackbar
                open={snackbar.open}
                autoHideDuration={4000}
                onClose={() => setSnackbar(prev => ({...prev, open: false}))}
                anchorOrigin={{vertical: 'bottom', horizontal: 'center'}}
            >
                <Alert
                    severity={snackbar.severity}
                    onClose={() => setSnackbar(prev => ({...prev, open: false}))}
                >
                    {snackbar.message}
                </Alert>
            </Snackbar>
        </PageLayout>
    );
};

export default Onboarding;

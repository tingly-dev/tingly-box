import {useState} from 'react';
import {Link, useNavigate} from 'react-router-dom';
import {useTranslation} from 'react-i18next';
import {
    Alert,
    Box,
    Button,
    Card,
    CardContent,
    Dialog,
    DialogActions,
    DialogContent,
    DialogContentText,
    DialogTitle,
    Stack,
    Snackbar,
    Tab,
    Tabs,
    Typography,
} from '@mui/material';
import {
    Home as HomeIcon,
    Help as HelpIcon,
} from '@/components/icons';
import PageLayout from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import ProviderFormDialog, {type EnhancedProviderFormData} from '@/components/ProviderFormDialog';
import BrowseProviders from '@/components/onboarding/BrowseProviders';
import PasteAndDetect from '@/components/onboarding/PasteAndDetect';
import {api} from '@/services/api';

type OnboardingTab = 'browse' | 'paste';

const emptyForm = (): EnhancedProviderFormData => ({
    name: '',
    apiBase: '',
    apiStyle: undefined,
    token: '',
    enabled: true,
    noKeyRequired: false,
    proxyUrl: '',
});

const Onboarding: React.FC = () => {
    const {t} = useTranslation();
    const navigate = useNavigate();
    const [tab, setTab] = useState<OnboardingTab>('browse');
    const [dialogOpen, setDialogOpen] = useState(false);
    const [formData, setFormData] = useState<EnhancedProviderFormData>(emptyForm());
    const [snackbar, setSnackbar] = useState<{open: boolean; message: string; severity: 'success' | 'error' | 'info'}>({
        open: false,
        message: '',
        severity: 'info',
    });
    const [successDialogOpen, setSuccessDialogOpen] = useState(false);

    const showMessage = (message: string, severity: 'success' | 'error' | 'info' = 'info') => {
        setSnackbar({open: true, message, severity});
    };

    const openDialogWith = (prefill: EnhancedProviderFormData) => {
        setFormData({
            ...emptyForm(),
            ...prefill,
        });
        setDialogOpen(true);
    };

    const handleFieldChange = (field: keyof EnhancedProviderFormData, value: any) => {
        setFormData(prev => ({...prev, [field]: value}));
    };

    const submitProvider = async (force: boolean, resolved?: Partial<EnhancedProviderFormData>) => {
        // Merge dialog-resolved fields over form state; they arrive via async
        // onChange and may not be in state yet at submit time.
        const fd = { ...formData, ...(resolved || {}) };
        const payload = {
            name: fd.name,
            api_base: fd.apiBase,
            api_style: fd.apiStyle,
            token: fd.token,
            no_key_required: fd.noKeyRequired,
            proxy_url: fd.proxyUrl,
        };
        const result = await api.addProvider(payload, force);
        if (result?.success) {
            setDialogOpen(false);
            setSuccessDialogOpen(true);
        } else {
            showMessage(`Failed to add provider: ${result?.error || 'unknown error'}`, 'error');
        }
    };

    const handleSubmit = async (e: React.FormEvent, resolved?: Partial<EnhancedProviderFormData>) => {
        e.preventDefault();
        await submitProvider(false, resolved);
    };

    const handleForceAdd = async () => {
        await submitProvider(true);
    };

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
                    subtitle={t('onboarding.subtitle', {
                        defaultValue: 'Add your first AI provider to get started. Browse the catalog or paste a config snippet — we’ll figure out the rest.',
                    })}
                >
                    <Tabs
                        value={tab}
                        onChange={(_, v) => setTab(v as OnboardingTab)}
                        sx={{
                            mb: 2,
                            borderBottom: 1,
                            borderColor: 'divider',
                            '& .MuiTab-root': {
                                textTransform: 'none',
                                fontWeight: 600,
                                fontSize: '0.95rem',
                                minHeight: 44,
                            },
                        }}
                    >
                        <Tab
                            value="browse"
                            label={t('onboarding.tab.browse', {defaultValue: 'Browse providers'})}
                        />
                        <Tab
                            value="paste"
                            label={t('onboarding.tab.paste', {defaultValue: 'Paste & detect'})}
                        />
                    </Tabs>

                    {tab === 'browse' && (
                        <BrowseProviders onPick={openDialogWith}/>
                    )}
                    {tab === 'paste' && (
                        <PasteAndDetect
                            onPick={openDialogWith}
                            onManualFill={() => openDialogWith(emptyForm())}
                        />
                    )}

                    <Box sx={{mt: 3}}>
                        <Typography variant="caption" color="text.secondary">
                            {t('onboarding.hint', {
                                defaultValue: 'Detection runs locally in the box; pasted text is not sent to any third party.',
                            })}
                        </Typography>
                    </Box>
                </UnifiedCard>
            </Box>

            <ProviderFormDialog
                open={dialogOpen}
                onClose={() => setDialogOpen(false)}
                onSubmit={handleSubmit}
                onForceAdd={handleForceAdd}
                data={formData}
                onChange={handleFieldChange}
                mode="add"
                isFirstProvider
            />

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

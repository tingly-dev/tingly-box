import {useState} from 'react';
import {Link, useNavigate} from 'react-router-dom';
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
import {ProviderListContent, type ConnectSelection} from '@/components/ConnectProviderDialog';
import OAuthDialog from '@/components/OAuthDialog';
import PasteAndDetect from '@/components/onboarding/PasteAndDetect';
import {api} from '@/services/api';
import {buildProviderFormData} from '@/hooks/useProviderDialog';

type OnboardingTab = 'browse' | 'paste';

type ProviderFormData = EnhancedProviderFormData;

const emptyForm = (): ProviderFormData => ({
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
    const [browseQuery, setBrowseQuery] = useState('');

    // Dialog state
    const [apiKeyDialogOpen, setApiKeyDialogOpen] = useState(false);
    const [providerFormData, setProviderFormData] = useState<ProviderFormData>(emptyForm());
    const [isLocalProvider, setIsLocalProvider] = useState(false);
    const [isCustomMode, setIsCustomMode] = useState(false);
    const [isDualMode, setIsDualMode] = useState(false);

    // OAuth Dialog state
    const [oauthDialogOpen, setOAuthDialogOpen] = useState(false);
    const [oauthAutoStartId, setOAuthAutoStartId] = useState<string | null>(null);

    const [snackbar, setSnackbar] = useState<{open: boolean; message: string; severity: 'success' | 'error' | 'info'}>({
        open: false,
        message: '',
        severity: 'info',
    });
    const [successDialogOpen, setSuccessDialogOpen] = useState(false);

    const showMessage = (message: string, severity: 'success' | 'error' | 'info' = 'info') => {
        setSnackbar({open: true, message, severity});
    };

    const openDialogWith = (prefill: ProviderFormData) => {
        setProviderFormData({
            ...emptyForm(),
            ...prefill,
        });
        setApiKeyDialogOpen(true);
    };

    // Route a pick from the provider list to the matching dialog.
    // This is the single truth for provider selection - same as CredentialPage.
    const handleBrowseSelect = (selection: ConnectSelection) => {
        if (selection.kind === 'oauth') {
            setOAuthAutoStartId(selection.providerId);
            setOAuthDialogOpen(true);
            return;
        }
        if (selection.kind === 'import') return;

        const built = buildProviderFormData(selection)!;

        if (selection.kind === 'custom') {
            setIsCustomMode(true);
            setIsLocalProvider(false);
            openDialogWith(built.formData);
            return;
        }

        setIsCustomMode(false);
        setIsLocalProvider(selection.kind === 'local');
        openDialogWith(built.formData);
    };

    const handleFieldChange = (field: keyof ProviderFormData, value: any) => {
        setProviderFormData(prev => ({...prev, [field]: value}));
    };

    const submitProvider = async (force: boolean, resolved?: Partial<ProviderFormData>) => {
        const fd = {...providerFormData, ...(resolved || {})};
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
            setApiKeyDialogOpen(false);
            setSuccessDialogOpen(true);
        } else {
            showMessage(`Failed to add provider: ${result?.error || 'unknown error'}`, 'error');
        }
    };

    const handleSubmit = async (e: React.FormEvent, resolved?: Partial<ProviderFormData>) => {
        e.preventDefault();
        await submitProvider(false, resolved);
    };

    const handleForceAdd = async () => {
        await submitProvider(true);
    };

    const handleOAuthSuccess = () => {
        showMessage('Provider added successfully!', 'success');
        setSuccessDialogOpen(true);
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
                        defaultValue: 'Add your first AI provider to get started. Browse the catalog or paste a config snippet — we\'ll figure out the rest.',
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
                        <ProviderListContent
                            onSelect={handleBrowseSelect}
                            query={browseQuery}
                            onQueryChange={setBrowseQuery}
                            hideOfficialInfo={true}
                            showDetails={true}
                            wide={true}
                        />
                    )}
                    {tab === 'paste' && (
                        <PasteAndDetect
                            onPick={openDialogWith}
                            onManualFill={() => openDialogWith(emptyForm())}
                        />
                    )}
                </UnifiedCard>
            </Box>

            {/* API Key Provider Dialog */}
            <ProviderFormDialog
                open={apiKeyDialogOpen}
                onClose={() => {
                    setApiKeyDialogOpen(false);
                    setIsLocalProvider(false);
                    setIsCustomMode(false);
                    setIsDualMode(false);
                }}
                onSubmit={handleSubmit}
                onForceAdd={handleForceAdd}
                data={providerFormData}
                onChange={handleFieldChange}
                mode="add"
                isFirstProvider
                optionalEditableToken={isLocalProvider}
                customMode={isCustomMode}
                dualMode={isDualMode}
            />

            {/* OAuth Add Dialog - now functional in onboarding */}
            <OAuthDialog
                open={oauthDialogOpen}
                autoStartProviderId={oauthAutoStartId}
                onClose={() => {
                    setOAuthDialogOpen(false);
                    setOAuthAutoStartId(null);
                }}
                onSuccess={handleOAuthSuccess}
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

import {useState} from 'react';
import {Link, useNavigate} from 'react-router-dom';
import {useTranslation} from 'react-i18next';
import {
    Alert,
    Box,
    Button,
    Card,
    CardContent,
    Stack,
    Snackbar,
    Tab,
    Tabs,
    Typography,
} from '@mui/material';
import {
    Home as HomeIcon,
    Help as HelpIcon,
} from '@mui/icons-material';
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

    const submitProvider = async (force: boolean) => {
        const payload = {
            name: formData.name,
            api_base: formData.apiBase,
            api_style: formData.apiStyle,
            token: formData.token,
            no_key_required: formData.noKeyRequired,
            proxy_url: formData.proxyUrl,
        };
        const result = await api.addProvider(payload, force);
        if (result?.success) {
            showMessage('Provider added', 'success');
            setDialogOpen(false);
            const target = formData.apiStyle === 'anthropic' ? '/agent/anthropic' : '/agent/openai';
            navigate(target, {replace: true});
        } else {
            showMessage(`Failed to add provider: ${result?.error || 'unknown error'}`, 'error');
        }
    };

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        await submitProvider(false);
    };

    const handleForceAdd = async () => {
        await submitProvider(true);
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
                        sx={{mb: 2, borderBottom: 1, borderColor: 'divider'}}
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

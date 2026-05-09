import { PageLayout } from '@/components/PageLayout';
import ModelSelectDialog, { type ProviderSelectTabOption } from '@/components/ModelSelectDialog';
import ToolCard from '@/components/ToolCard';
import { api } from '@/services/api';
import type { Provider } from '@/types/provider';
import {
    Box,
    Button,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Snackbar,
    Alert,
    Stack,
    Typography,
} from '@mui/material';
import { IconBrain } from '@tabler/icons-react';
import { useEffect, useState } from 'react';
import {
    BUILTIN_ADVISOR_ID,
    BUILTIN_IDS,
    BUILTIN_WEBTOOLS_ID,
    type MCPConfigResponse,
    type MCPSourceConfig,
} from '../mcp/types';

// ─── Advisor ToolCard ─────────────────────────────────────────────────────────

interface AdvisorCardProps {
    advisorSource: MCPSourceConfig | undefined;
    onSave: (patch: MCPSourceConfig) => Promise<void>;
}

const AdvisorCard: React.FC<AdvisorCardProps> = ({ advisorSource, onSave }) => {
    const [model, setModel] = useState('');
    const [selectedProviderUuid, setSelectedProviderUuid] = useState('');
    const [saving, setSaving] = useState(false);
    const [providerCatalog, setProviderCatalog] = useState<Provider[]>([]);
    const [modelDialogOpen, setModelDialogOpen] = useState(false);

    useEffect(() => {
        const providerUuid = advisorSource?.env?.['ADVISOR_PROVIDER_UUID'] ?? '';
        const m = advisorSource?.advisor?.model ?? advisorSource?.env?.['ADVISOR_MODEL'] ?? '';
        setSelectedProviderUuid(providerUuid);
        setModel(m);
    }, [advisorSource]);

    useEffect(() => {
        const load = async () => {
            const result = await api.getProviders();
            if (result?.success && Array.isArray(result.data)) {
                setProviderCatalog(result.data as Provider[]);
            }
        };
        void load();
    }, []);

    const enabled = advisorSource?.enabled ?? false;

    const handleToggle = (next: boolean) => {
        if (!advisorSource) return;
        const { visibility, transport, command, args, cwd, ...rest } = advisorSource as any;
        void onSave({ ...rest, enabled: next });
    };

    const handleSave = async () => {
        setSaving(true);
        try {
            const selectedProvider = providerCatalog.find((p) => p.uuid === selectedProviderUuid);
            const { visibility, transport, command, args, cwd, ...base } = (advisorSource ?? {
                id: BUILTIN_ADVISOR_ID,
                name: 'Built-in Adviser',
                tools: ['advisor'],
                enabled: false,
            }) as any;
            await onSave({
                ...base,
                advisor: {
                    ...(base.advisor ?? {}),
                    base_url: selectedProvider?.api_base || undefined,
                    api_key: selectedProvider?.token || undefined,
                    model: model || undefined,
                },
                env: {
                    ...(selectedProviderUuid ? { ADVISOR_PROVIDER_UUID: selectedProviderUuid } : {}),
                    ...(selectedProvider?.api_base ? { ADVISOR_BASE_URL: selectedProvider.api_base } : {}),
                    ...(selectedProvider?.token ? { ADVISOR_API_KEY: selectedProvider.token } : {}),
                    ...(model ? { ADVISOR_MODEL: model } : {}),
                },
            });
        } finally {
            setSaving(false);
        }
    };

    const selectedProvider = providerCatalog.find((p) => p.uuid === selectedProviderUuid);

    const settings = (
        <Stack spacing={1.5}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                <Button size="small" variant="outlined" onClick={() => setModelDialogOpen(true)}>
                    Choose Model
                </Button>
                <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem', color: 'text.secondary' }}>
                    {selectedProvider
                        ? `${selectedProvider.name} (${selectedProvider.api_style}) / ${model || '(no model)'}`
                        : '(no provider selected)'}
                </Typography>
            </Box>
            <Box sx={{ display: 'flex', justifyContent: 'flex-end' }}>
                <Button variant="contained" size="small" onClick={() => void handleSave()} disabled={saving}>
                    {saving ? 'Saving...' : 'Save'}
                </Button>
            </Box>

            <Dialog open={modelDialogOpen} onClose={() => setModelDialogOpen(false)} maxWidth="lg" fullWidth>
                <DialogTitle sx={{ textAlign: 'center' }}>Choose Model</DialogTitle>
                <DialogContent sx={{ height: '70vh' }}>
                    <ModelSelectDialog
                        providers={providerCatalog}
                        selectedProvider={selectedProviderUuid || undefined}
                        selectedModel={model || undefined}
                        onSelected={(option: ProviderSelectTabOption) => {
                            setSelectedProviderUuid(option.provider.uuid);
                            setModel(option.model || '');
                            setModelDialogOpen(false);
                        }}
                    />
                </DialogContent>
                <DialogActions>
                    <Button size="small" onClick={() => setModelDialogOpen(false)}>Close</Button>
                </DialogActions>
            </Dialog>
        </Stack>
    );

    return (
        <ToolCard
            icon={<IconBrain size={18} />}
            name="Advisor"
            description="Sub-LLM consultation tool for hard decisions. An in-process tool agents can call to consult a second model."
            enabled={enabled}
            onToggle={handleToggle}
            toggleDisabled={saving}
            badges={[
                { label: 'Server', color: 'green' },
                { label: 'Experimental', color: 'orange' },
            ]}
            tags={['advisor']}
            settings={settings}
            defaultExpanded
        />
    );
};

// ─── Page ─────────────────────────────────────────────────────────────────────

const ServerToolPage = () => {
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [allSources, setAllSources] = useState<MCPSourceConfig[]>([]);
    const [notification, setNotification] = useState({ open: false, message: '', severity: 'success' as 'success' | 'error' });

    useEffect(() => { void loadData(); }, []);

    const loadData = async () => {
        setLoading(true);
        const result: MCPConfigResponse = await api.getMCPConfig();
        if (result.success && result.config) {
            setAllSources(result.config.sources || []);
        }
        setLoading(false);
    };

    const saveConfig = async (sources: MCPSourceConfig[]): Promise<void> => {
        setSaving(true);
        const result = await api.setMCPConfig({ sources });
        if (result.success) {
            setAllSources(sources);
            setNotification({ open: true, message: 'Saved.', severity: 'success' });
        } else {
            setNotification({ open: true, message: result.error || 'Failed to save', severity: 'error' });
        }
        setSaving(false);
    };

    const upsertSource = async (patch: MCPSourceConfig): Promise<void> => {
        const next = [...allSources];
        const idx = next.findIndex((s) => s.id === patch.id);
        if (idx >= 0) { next[idx] = patch; } else { next.push(patch); }
        await saveConfig(next);
    };

    const advisorSource = allSources.find((s) => s.id === BUILTIN_ADVISOR_ID);

    if (loading) {
        return (
            <PageLayout loading={true}>
                <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 400 }}>
                    <CircularProgress />
                </Box>
            </PageLayout>
        );
    }

    return (
        <PageLayout loading={false}>
            <Stack spacing={2.5}>
                {/* Section header */}
                <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 2 }}>
                    <Typography
                        sx={{ fontFamily: 'monospace', fontSize: '0.7rem', fontWeight: 600, color: 'text.disabled', mt: 0.5, flexShrink: 0, userSelect: 'none' }}
                    >
                        01
                    </Typography>
                    <Box>
                        <Typography variant="h6" sx={{ fontWeight: 700, lineHeight: 1.2, mb: 0.5 }}>
                            Server tools
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            In-process tools injected by the gateway into every AI request. Click a card to configure.
                        </Typography>
                    </Box>
                </Box>

                <AdvisorCard advisorSource={advisorSource} onSave={upsertSource} />
            </Stack>

            <Snackbar
                open={notification.open}
                autoHideDuration={3000}
                onClose={() => setNotification({ open: false, message: '', severity: 'success' })}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
            >
                <Alert severity={notification.severity} sx={{ width: '100%' }}>
                    {notification.message}
                </Alert>
            </Snackbar>
        </PageLayout>
    );
};

export default ServerToolPage;

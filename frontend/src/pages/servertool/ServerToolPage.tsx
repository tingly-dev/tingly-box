import { PageLayout } from '@/components/PageLayout';
import ModelSelectDialog, { type ProviderSelectTabOption } from '@/components/ModelSelectDialog';
import ToolCard from '@/components/ToolCard';
import ToolFilterBar, { type ToolFilter } from '@/components/ToolFilterBar';
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
    Stack,
    Typography,
} from '@mui/material';
import { Psychology as IconBrain } from '@/components/icons';
import { useEffect, useState } from 'react';
import { useNotify } from '@/hooks/useNotify';
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
    expanded?: boolean;
}

const AdvisorCard: React.FC<AdvisorCardProps> = ({ advisorSource, onSave, expanded }) => {
    const [model, setModel] = useState('');
    const [selectedProviderUuid, setSelectedProviderUuid] = useState('');
    const [saving, setSaving] = useState(false);
    const [providerCatalog, setProviderCatalog] = useState<Provider[]>([]);
    const [modelDialogOpen, setModelDialogOpen] = useState(false);

    useEffect(() => {
        const providerUuid = advisorSource?.advisor?.provider_uuid ?? '';
        const m = advisorSource?.advisor?.model ?? '';
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
                    provider_uuid: selectedProviderUuid || undefined,
                    model: model || undefined,
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
            icon={<IconBrain sx={{ fontSize: 18 }} />}
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
            expanded={expanded}
        />
    );
};

// ─── Page ─────────────────────────────────────────────────────────────────────

const ServerToolPage = () => {
    const notify = useNotify();
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [allSources, setAllSources] = useState<MCPSourceConfig[]>([]);
    const [filter, setFilter] = useState<ToolFilter>('all');
    const [allExpanded, setAllExpanded] = useState(true);

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
            notify.success('Saved.');
        } else {
            notify.error(result.error || 'Failed to save');
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
                        sx={{ fontFamily: 'monospace', fontSize: '0.85rem', fontWeight: 700, color: 'text.primary', opacity: 0.35, mt: 0.35, flexShrink: 0, userSelect: 'none', letterSpacing: '0.05em' }}
                    >
                        01
                    </Typography>
                    <Box>
                        <Typography variant="h5" sx={{ fontWeight: 700, lineHeight: 1.2, mb: 0.5 }}>
                            Config your server tools
                        </Typography>
                        <Typography variant="body2" color="text.secondary">
                            In-process tools injected by the gateway into every AI request.
                        </Typography>
                    </Box>
                </Box>

                <ToolFilterBar
                    filter={filter}
                    onFilterChange={setFilter}
                    allExpanded={allExpanded}
                    onToggleExpand={setAllExpanded}
                />
                {(filter === 'all' || (filter === 'active' ? (advisorSource?.enabled ?? false) : !(advisorSource?.enabled ?? false))) && (
                    <AdvisorCard advisorSource={advisorSource} onSave={upsertSource} expanded={allExpanded ? true : undefined} />
                )}            </Stack>
        </PageLayout>
    );
};

export default ServerToolPage;

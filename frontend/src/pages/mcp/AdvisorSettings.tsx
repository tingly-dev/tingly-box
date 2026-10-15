import ModelSelectDialog, { type ProviderSelectTabOption } from '@/components/ModelSelectDialog';
import { api } from '@/services/api';
import type { Provider } from '@/types/provider';
import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { BUILTIN_ADVISOR_ID, type MCPSourceConfig } from './types';
interface AdvisorCardProps {
    advisorSource: MCPSourceConfig | undefined;
    onSave: (patch: MCPSourceConfig) => Promise<void>;
}

const AdvisorSettings: React.FC<AdvisorCardProps> = ({ advisorSource, onSave }) => {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const [model, setModel] = useState(advisorSource?.advisor?.model ?? '');
    const [selectedProviderUuid, setSelectedProviderUuid] = useState(advisorSource?.advisor?.provider_uuid ?? '');
    const [saving, setSaving] = useState(false);
    const [providerCatalog, setProviderCatalog] = useState<Provider[]>([]);
    const [modelDialogOpen, setModelDialogOpen] = useState(false);

    useEffect(() => {
        const load = async () => {
            const result = await api.getProviders();
            if (result?.success && Array.isArray(result.data)) {
                setProviderCatalog(result.data as Provider[]);
            }
        };
        void load();
    }, []);

    const handleSave = async () => {
        setSaving(true);
        try {
            const base = advisorSource ?? {
                id: BUILTIN_ADVISOR_ID,
                transport: 'advisor',
                name: 'Built-in Adviser',
                tools: ['advisor'],
                enabled: false,
            };
            await onSave({
                id: base.id,
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
                    {label('chooseModel', 'Choose consultation model')}
                </Button>
                <Typography
                    variant="body2"
                    sx={{ fontFamily: 'monospace', fontSize: '0.8rem', color: 'text.secondary' }}
                >
                    {selectedProvider
                        ? `${selectedProvider.name} (${selectedProvider.api_style}) / ${model || label('noModel', 'No model selected')}`
                        : label('noProvider', 'No consultation provider selected')}
                </Typography>
            </Box>
            <Box sx={{ display: 'flex', justifyContent: 'flex-end' }}>
                <Button variant="contained" size="small" onClick={() => void handleSave()} disabled={saving}>
                    {saving ? label('saving', 'Saving…') : label('save', 'Save')}
                </Button>
            </Box>

            <Dialog open={modelDialogOpen} onClose={() => setModelDialogOpen(false)} maxWidth="lg" fullWidth>
                <DialogTitle sx={{ textAlign: 'center' }}>
                    {label('chooseModel', 'Choose consultation model')}
                </DialogTitle>
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
                    <Button size="small" onClick={() => setModelDialogOpen(false)}>
                        {label('closeModel', 'Close')}
                    </Button>
                </DialogActions>
            </Dialog>
        </Stack>
    );

    return settings;
};

export default AdvisorSettings;

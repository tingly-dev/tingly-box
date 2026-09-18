import React, { useMemo, useState } from 'react';
import { Box, Button, Dialog, DialogContent, DialogTitle, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import ModelSelectDialog, { type ProviderSelectTabOption } from '@/components/ModelSelectDialog';
import type { BenchTarget } from './benchState';
import type { TargetCatalog } from './useTargetCatalog';

// TargetPicker: the same card-based provider→model picker the routing graph
// already trained users on (ModelSelectDialog — left rail of providers,
// right grid of model cards), instead of a searchable-but-cramped Autocomplete
// dropdown. Bench only ever targets a real provider model directly; there is
// no Rule concept here to fold into the picker (benchState.ts, BenchTarget).

export const TargetPicker: React.FC<{
    catalog: TargetCatalog;
    value: BenchTarget | null;
    onChange: (target: BenchTarget | null) => void;
}> = ({ catalog, value, onChange }) => {
    const { t } = useTranslation();
    const [open, setOpen] = useState(false);
    const provider = useMemo(() => (value ? catalog.providers.find((p) => p.uuid === value.providerUuid) ?? null : null), [catalog.providers, value]);
    const missing = !!value && !catalog.loading && !provider;

    const handleSelected = (option: ProviderSelectTabOption) => {
        onChange({ providerUuid: option.provider.uuid, model: option.model });
        setOpen(false);
    };

    return (
        <Box>
            <Button
                fullWidth
                size="small"
                variant="outlined"
                color={missing ? 'error' : 'inherit'}
                onClick={() => setOpen(true)}
                sx={{ justifyContent: 'flex-start', textTransform: 'none', fontWeight: 400, py: 0.75 }}
            >
                <Typography variant="body2" noWrap sx={{ color: value && provider ? 'text.primary' : 'text.secondary' }}>
                    {value && provider
                        ? `${provider.name} · ${value.model}`
                        : catalog.loading
                          ? t('bench.targetLoading', { defaultValue: 'Loading providers…' })
                          : t('bench.targetPlaceholder', { defaultValue: 'Pick a provider & model…' })}
                </Typography>
            </Button>
            <Typography variant="caption" sx={{ display: 'block', mt: 0.5, color: missing || catalog.error ? 'error.main' : 'text.secondary' }}>
                {catalog.error
                    ? catalog.error
                    : missing
                      ? t('bench.targetMissing', { defaultValue: 'The saved target no longer exists — pick another.' })
                      : !value
                        ? t('bench.targetEmpty', { defaultValue: 'Pick a provider model to start.' })
                        : ' '}
            </Typography>
            <Dialog open={open} onClose={() => setOpen(false)} maxWidth="lg" fullWidth slotProps={{ paper: { sx: { height: '80vh' } } }}>
                <DialogTitle sx={{ textAlign: 'center' }}>{t('bench.targetDialogTitle', { defaultValue: 'Choose a provider & model' })}</DialogTitle>
                <DialogContent>
                    <ModelSelectDialog providers={catalog.providers} selectedProvider={value?.providerUuid} selectedModel={value?.model} onSelected={handleSelected} />
                </DialogContent>
            </Dialog>
        </Box>
    );
};

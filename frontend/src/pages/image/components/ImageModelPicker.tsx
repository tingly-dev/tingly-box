import { useCallback, useEffect, useMemo, useState } from 'react';
import {
    Box,
    Button,
    Dialog,
    DialogContent,
    DialogTitle,
    Divider,
    FormControl,
    InputLabel,
    ListSubheader,
    MenuItem,
    Select,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { Rule } from '@/components/RoutingGraphTypes';
import ModelSelectDialog, { type ProviderSelectTabOption } from '@/components/ModelSelectDialog';
import { Add } from '@/components/icons';
import { api } from '@/services/api';
import type { Provider } from '@/types/provider';
import { resolveRuleName } from '@/pages/scenario/hooks/useTemplatePageRules';
import { findDirectRule, imageModelOptions, type ImageModelOption } from './imageModelRules';

const CHOOSE_ANOTHER = '__choose_another_model__';

interface ImageModelPickerProps {
    rules: Rule[];
    // The request_model a run sends.
    value: string;
    onChange: (requestModel: string) => void;
    // Reloads the imagegen rules after the picker adds one.
    onRulesReload: () => Promise<void>;
    showNotification: (message: string, severity: 'success' | 'info' | 'warning' | 'error') => void;
    // The scenario whose rules these are; the Image to Prompt page picks a
    // vision model the same way, from its own scenario.
    scenario?: string;
    label?: string;
    chooserTitle?: string;
}

// Picks a provider · model, the way a service is picked in a rule. Behind it
// the imagegen rule for that pair is found or created, so runs keep going
// through /tingly/imagegen and API clients can call the same name. Rules a
// person built with several services are listed by name. See
// .design/image-layout.md §4.
const ImageModelPicker: React.FC<ImageModelPickerProps> = ({
    rules,
    value,
    onChange,
    onRulesReload,
    showNotification,
    scenario = 'imagegen',
    label,
    chooserTitle,
}) => {
    const { t } = useTranslation();
    const [providers, setProviders] = useState<Provider[]>([]);
    const [chooserOpen, setChooserOpen] = useState(false);
    const [saving, setSaving] = useState(false);

    // Only for labels and the chooser; the picker renders before they arrive.
    useEffect(() => {
        let cancelled = false;
        void api.getProviders().then((result: any) => {
            if (!cancelled && result?.success && Array.isArray(result.data)) setProviders(result.data);
        });
        return () => { cancelled = true; };
    }, []);

    const providerNames = useMemo(
        () => new Map(providers.map((provider) => [provider.uuid, provider.name])),
        [providers],
    );
    const options = useMemo(() => imageModelOptions(rules), [rules]);
    const directOptions = options.filter((option) => option.direct);
    const ruleOptions = options.filter((option) => !option.direct);

    const pairLabel = useCallback((option: ImageModelOption) => {
        if (!option.direct) return option.requestModel;
        const providerName = providerNames.get(option.direct.provider);
        return providerName ? `${providerName} · ${option.direct.model}` : option.direct.model;
    }, [providerNames]);

    // Two rules can point at the same pair; name the rule only then.
    const labelCounts = useMemo(() => {
        const counts = new Map<string, number>();
        for (const option of directOptions) {
            const label = pairLabel(option);
            counts.set(label, (counts.get(label) ?? 0) + 1);
        }
        return counts;
    }, [directOptions, pairLabel]);

    const handleChosen = useCallback(async ({ provider, model }: ProviderSelectTabOption) => {
        setChooserOpen(false);
        if (!model) return;
        const label = `${provider.name} · ${model}`;
        const existing = findDirectRule(rules, provider.uuid, model);
        if (existing) {
            onChange(existing.request_model.trim());
            return;
        }
        setSaving(true);
        try {
            const existingNames = new Set(rules.map((rule) => rule.request_model).filter(Boolean));
            const body = {
                scenario,
                request_model: resolveRuleName(model, existingNames),
                response_model: '',
                active: true,
                services: [{ provider: provider.uuid, model, weight: 0, active: true, time_window: 0 }],
            };
            let result = await api.createRule('', body);
            if (!result?.success && String(result?.error ?? '').includes('already exists')) {
                body.request_model = resolveRuleName(model, new Set([...existingNames, body.request_model]));
                result = await api.createRule('', body);
            }
            if (!result?.success) {
                showNotification(t('playground.modelPicker.addFailed', {
                    defaultValue: 'Could not use {{model}}: {{error}}',
                    model: label,
                    error: result?.error ?? '',
                }), 'error');
                return;
            }
            await onRulesReload();
            onChange(result.data?.request_model ?? body.request_model);
        } finally {
            setSaving(false);
        }
    }, [rules, onChange, onRulesReload, scenario, showNotification, t]);

    const chooser = (
        <Dialog
            open={chooserOpen}
            onClose={() => setChooserOpen(false)}
            maxWidth="lg"
            fullWidth
            slotProps={{ paper: { sx: { height: '80vh' } } }}
        >
            <DialogTitle sx={{ textAlign: 'center' }}>
                {chooserTitle ?? t('playground.modelPicker.chooserTitle', { defaultValue: 'Choose an image model' })}
            </DialogTitle>
            <DialogContent>
                {chooserOpen && (
                    <ModelSelectDialog
                        providers={providers}
                        onSelected={(option) => { void handleChosen(option); }}
                    />
                )}
            </DialogContent>
        </Dialog>
    );

    if (options.length === 0) {
        return (
            <>
                <Button
                    variant="outlined"
                    fullWidth
                    startIcon={<Add />}
                    onClick={() => setChooserOpen(true)}
                    disabled={saving}
                >
                    {t('playground.modelPicker.chooseFirst', { defaultValue: 'Choose a model to start' })}
                </Button>
                {chooser}
            </>
        );
    }

    const selected = options.find((option) => option.requestModel === value);
    const fieldLabel = label ?? t('playground.model', { defaultValue: 'Model' });
    const labelId = `${scenario}-model-label`;

    return (
        <>
            <FormControl size="small" fullWidth>
                <InputLabel id={labelId}>
                    {fieldLabel}
                </InputLabel>
                <Select
                    labelId={labelId}
                    label={fieldLabel}
                    value={selected ? value : ''}
                    disabled={saving}
                    renderValue={() => (selected ? pairLabel(selected) : '')}
                    onChange={(event) => {
                        const next = String(event.target.value);
                        if (next === CHOOSE_ANOTHER) {
                            setChooserOpen(true);
                            return;
                        }
                        onChange(next);
                    }}
                >
                    {directOptions.map((option) => {
                        const label = pairLabel(option);
                        return (
                            <MenuItem key={option.requestModel} value={option.requestModel}>
                                <Box sx={{ minWidth: 0 }}>
                                    {label}
                                    {(labelCounts.get(label) ?? 0) > 1 && (
                                        <Typography component="span" variant="caption" color="text.secondary" sx={{ ml: 1 }}>
                                            {option.requestModel}
                                        </Typography>
                                    )}
                                </Box>
                            </MenuItem>
                        );
                    })}
                    {ruleOptions.length > 0 && (
                        <ListSubheader>
                            {t('playground.modelPicker.yourRules', { defaultValue: 'Your rules' })}
                        </ListSubheader>
                    )}
                    {ruleOptions.map((option) => (
                        <MenuItem key={option.requestModel} value={option.requestModel}>
                            {option.requestModel}
                        </MenuItem>
                    ))}
                    <Divider />
                    <MenuItem value={CHOOSE_ANOTHER}>
                        <Add sx={{ fontSize: 18, mr: 1 }} />
                        {t('playground.modelPicker.chooseAnother', { defaultValue: 'Choose another model…' })}
                    </MenuItem>
                </Select>
            </FormControl>
            {chooser}
        </>
    );
};

export default ImageModelPicker;

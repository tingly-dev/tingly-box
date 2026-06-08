import {
    Box,
    Button,
    Chip,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControl,
    InputLabel,
    MenuItem,
    Select,
    Stack,
    Switch,
    TextField,
    Typography,
} from '@mui/material';
import {
    Close as CloseIcon,
    Extension as ExtensionIcon,
    Input as InputIcon,
    Link as LinkIcon,
    Output as OutputIcon,
    Psychology as PsychologyIcon,
    Terminal as TerminalIcon,
    Visibility as VisibilityIcon,
} from '@/components/icons';
import React, { useEffect, useMemo, useRef, useState } from 'react';
import type { FlagSpec, RuleFlags, VisionProxyServiceRef } from '@/components/RoutingGraphTypes';
import type { Provider } from '@/types/provider';
import type { ProviderSelectTabOption } from '@/components/ModelSelectDialog';
import ModelSelectDialog from '@/components/ModelSelectDialog';
import { getFlagValue, setFlagValue, flagDefault, enumInactive, isFlagActive, normalizeEnumForStorage } from './flagHelpers';

export interface FlagCatalogDialogProps {
    open: boolean;
    flags?: RuleFlags;
    registry?: FlagSpec[];
    loading?: boolean;
    /** Providers for service_ref flags (e.g. vision_proxy_service model picker). */
    providers?: Provider[];
    onClose: () => void;
    onSave: (next: RuleFlags) => void;
}

const flagToBool = (flags: RuleFlags | undefined, key: string): boolean =>
    !!getFlagValue(flags, key);

const flagToInt = (flags: RuleFlags | undefined, key: string): number =>
    (getFlagValue(flags, key) as number) ?? 0;

const flagToString = (flags: RuleFlags | undefined, key: string): string =>
    (getFlagValue(flags, key) as string) ?? '';

const flagToServiceRef = (flags: RuleFlags | undefined, key: string): VisionProxyServiceRef | undefined =>
    getFlagValue(flags, key) as VisionProxyServiceRef | undefined;

interface CategoryMeta {
    label: string;
    icon: React.ReactElement;
}

// Display order for the category sidebar. Unknown categories are appended.
const CATEGORY_ORDER = ['app', 'request_openai', 'request_anthropic', 'response', 'reasoning', 'vision', 'routing'];

const CATEGORY_META: Record<string, CategoryMeta> = {
    app:               { label: 'App',         icon: <TerminalIcon   fontSize="small" /> },
    request_openai:    { label: 'Request (O)', icon: <InputIcon      fontSize="small" /> },
    request_anthropic: { label: 'Request (A)', icon: <InputIcon      fontSize="small" /> },
    response:          { label: 'Response',    icon: <OutputIcon     fontSize="small" /> },
    reasoning:         { label: 'Reasoning',   icon: <PsychologyIcon fontSize="small" /> },
    vision:            { label: 'Vision',      icon: <VisibilityIcon fontSize="small" /> },
    routing:           { label: 'Routing',     icon: <LinkIcon       fontSize="small" /> },
};

const categoryMeta = (category: string): CategoryMeta => CATEGORY_META[category] ?? {
    label: category.charAt(0).toUpperCase() + category.slice(1),
    icon: <ExtensionIcon fontSize="small" />,
};

export const FlagCatalogDialog: React.FC<FlagCatalogDialogProps> = ({
    open,
    flags,
    registry,
    loading,
    providers,
    onClose,
    onSave,
}) => {
    const [draft, setDraft] = useState<RuleFlags>(flags || {});
    const [activeCategory, setActiveCategory] = useState<string | undefined>();
    const [pulseKey, setPulseKey] = useState<string | undefined>();
    // Key of the service_ref flag whose model picker is open (null = closed).
    const [pickerKey, setPickerKey] = useState<string | null>(null);
    const flagRefs = useRef<Record<string, HTMLDivElement | null>>({});

    // Reset working copy whenever the dialog is (re-)opened with new flags.
    useEffect(() => {
        if (open) {
            setDraft(flags ? { ...flags } : {});
            setPulseKey(undefined);
        }
    }, [open, flags]);

    // Group registry entries by category, preserving backend order within each
    // group, then sort groups by CATEGORY_ORDER (unknown categories appended).
    const grouped = useMemo(() => {
        const groups = new Map<string, FlagSpec[]>();
        (registry || []).forEach((spec) => {
            if (!groups.has(spec.category)) groups.set(spec.category, []);
            groups.get(spec.category)!.push(spec);
        });
        const ordered = CATEGORY_ORDER.filter((cat) => groups.has(cat));
        const orderedSet = new Set(ordered);
        groups.forEach((_, cat) => { if (!orderedSet.has(cat)) ordered.push(cat); });
        return ordered.map((cat) => ({ category: cat, specs: groups.get(cat) || [] }));
    }, [registry]);

    // Default the selected category to the first one with content.
    useEffect(() => {
        if (!open) return;
        if (activeCategory && grouped.some((g) => g.category === activeCategory)) return;
        if (grouped.length > 0) setActiveCategory(grouped[0].category);
    }, [open, grouped, activeCategory]);

    const activeFlags = useMemo(() => {
        return (registry || []).filter((spec) => isFlagActive(spec, draft));
    }, [registry, draft]);

    const currentGroup = grouped.find((g) => g.category === activeCategory);

    const specLookup = useMemo(() => new Map((registry || []).map((s) => [s.key, s])), [registry]);

    const handleToggle = (key: string, value: boolean) => {
        setDraft((d) => setFlagValue(d, key, value));
    };

    const handleStringChange = (key: string, value: string) => {
        const spec = specLookup.get(key);
        const normalized = spec?.type === 'enum' ? normalizeEnumForStorage(spec, value) : value;
        setDraft((d) => setFlagValue(d, key, normalized));
    };

    const handleIntChange = (key: string, value: string) => {
        const n = parseInt(value, 10);
        setDraft((d) => setFlagValue(d, key, isNaN(n) || n < 0 ? 0 : n));
    };

    const jumpToFlag = (spec: FlagSpec) => {
        if (spec.category !== activeCategory) setActiveCategory(spec.category);
        // Defer scroll/pulse until the right pane has rendered the target.
        requestAnimationFrame(() => {
            const node = flagRefs.current[spec.key];
            if (node) node.scrollIntoView({ behavior: 'smooth', block: 'center' });
            setPulseKey(spec.key);
            window.setTimeout(() => {
                setPulseKey((k) => (k === spec.key ? undefined : k));
            }, 1200);
        });
    };

    const handleRemoveActive = (spec: FlagSpec) => {
        setDraft((d) => setFlagValue(d, spec.key, flagDefault(spec)));
    };

    return (
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
            <DialogTitle sx={{ pb: 1 }}>
                Rule Plugins
                <Typography variant="caption" component="div" color="text.secondary">
                    Plugin flags applied at the rule level.
                </Typography>
            </DialogTitle>

            {/* Active flags strip — empty state stays hidden to save vertical space. */}
            {activeFlags.length > 0 && (
                <Box
                    sx={{
                        px: 3,
                        py: 1.25,
                        borderTop: 1,
                        borderBottom: 1,
                        borderColor: 'divider',
                        bgcolor: 'action.hover',
                    }}
                >
                    <Stack direction="row" alignItems="center" spacing={1} flexWrap="wrap" useFlexGap>
                        <Typography variant="caption" sx={{ fontWeight: 600, color: 'text.secondary' }}>
                            Active ({activeFlags.length})
                        </Typography>
                        {activeFlags.map((spec) => {
                            const meta = categoryMeta(spec.category);
                            return (
                                <Chip
                                    key={spec.key}
                                    size="small"
                                    icon={meta.icon}
                                    label={spec.label}
                                    onClick={() => jumpToFlag(spec)}
                                    onDelete={() => handleRemoveActive(spec)}
                                    deleteIcon={<CloseIcon />}
                                    sx={{ maxWidth: 220 }}
                                />
                            );
                        })}
                    </Stack>
                </Box>
            )}

            <DialogContent sx={{ p: 0, display: 'flex', minHeight: 420 }} dividers={false}>
                {loading && (
                    <Box sx={{ p: 3 }}>
                        <Typography variant="body2" color="text.secondary">
                            Loading flag catalog…
                        </Typography>
                    </Box>
                )}
                {!loading && grouped.length === 0 && (
                    <Box sx={{ p: 3 }}>
                        <Typography variant="body2" color="text.secondary">
                            No flags available.
                        </Typography>
                    </Box>
                )}

                {!loading && grouped.length > 0 && (
                    <>
                        {/* Left: category sidebar */}
                        <Box
                            sx={{
                                width: 200,
                                flexShrink: 0,
                                borderRight: 1,
                                borderColor: 'divider',
                                bgcolor: 'background.paper',
                                overflowY: 'auto',
                            }}
                        >
                            {grouped.map(({ category, specs }) => {
                                const meta = categoryMeta(category);
                                const activeCount = specs.filter((s) => isFlagActive(s, draft)).length;
                                const selected = category === activeCategory;
                                return (
                                    <Box
                                        key={category}
                                        onClick={() => setActiveCategory(category)}
                                        sx={{
                                            px: 2,
                                            py: 1.25,
                                            cursor: 'pointer',
                                            display: 'flex',
                                            alignItems: 'center',
                                            gap: 1,
                                            borderLeft: 3,
                                            borderLeftColor: selected ? 'primary.main' : 'transparent',
                                            bgcolor: selected ? 'action.selected' : 'transparent',
                                            '&:hover': {
                                                bgcolor: selected ? 'action.selected' : 'action.hover',
                                            },
                                            transition: 'background-color 0.15s',
                                        }}
                                    >
                                        <Box sx={{ color: selected ? 'primary.main' : 'text.secondary', display: 'flex' }}>
                                            {meta.icon}
                                        </Box>
                                        <Typography
                                            variant="body2"
                                            sx={{
                                                flexGrow: 1,
                                                fontWeight: selected ? 600 : 400,
                                                color: selected ? 'primary.main' : 'text.primary',
                                            }}
                                        >
                                            {meta.label}
                                        </Typography>
                                        <Chip
                                            size="small"
                                            label={activeCount > 0 ? `${activeCount}/${specs.length}` : `${specs.length}`}
                                            color={activeCount > 0 ? 'primary' : 'default'}
                                            variant={activeCount > 0 ? 'filled' : 'outlined'}
                                            sx={{ height: 18, fontSize: '0.65rem' }}
                                        />
                                    </Box>
                                );
                            })}
                        </Box>

                        {/* Right: flag detail pane */}
                        <Box sx={{ flexGrow: 1, p: 2, overflowY: 'auto' }}>
                            {currentGroup && (
                                <Stack spacing={1.5}>
                                    {currentGroup.specs.map((spec) => {
                                        const enabled = isFlagActive(spec, draft);
                                        const enumValue = spec.type === 'enum'
                                            ? (flagToString(draft, spec.key) || enumInactive(spec))
                                            : '';
                                        const pulsing = pulseKey === spec.key;
                                        return (
                                            <Box
                                                key={spec.key}
                                                ref={(el: HTMLDivElement | null) => {
                                                    flagRefs.current[spec.key] = el;
                                                }}
                                                sx={{
                                                    p: 1.25,
                                                    border: '1px solid',
                                                    borderColor: pulsing
                                                        ? 'primary.main'
                                                        : enabled
                                                            ? 'primary.light'
                                                            : 'divider',
                                                    borderRadius: 1,
                                                    backgroundColor: enabled ? 'action.hover' : 'transparent',
                                                    boxShadow: pulsing ? '0 0 0 3px rgba(25,118,210,0.18)' : 'none',
                                                    transition: 'box-shadow 0.2s, border-color 0.2s',
                                                }}
                                            >
                                                <Stack direction="row" alignItems="center" spacing={1}>
                                                    <Box sx={{ flexGrow: 1, minWidth: 0 }}>
                                                        <Stack direction="row" alignItems="center" spacing={0.75}>
                                                            <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                                                {spec.label}
                                                            </Typography>
                                                            <Chip
                                                                size="small"
                                                                label={spec.key}
                                                                sx={{ height: 16, fontSize: '0.6rem' }}
                                                                variant="outlined"
                                                            />
                                                        </Stack>
                                                        <Typography variant="caption" color="text.secondary">
                                                            {spec.description}
                                                        </Typography>
                                                    </Box>
                                                    {spec.type === 'bool' && (
                                                        <Switch
                                                            size="small"
                                                            checked={flagToBool(draft, spec.key)}
                                                            onChange={(e) => handleToggle(spec.key, e.target.checked)}
                                                        />
                                                    )}
                                                </Stack>
                                                {spec.type === 'string' && (
                                                    <>
                                                        <TextField
                                                            fullWidth
                                                            size="small"
                                                            placeholder={spec.placeholder}
                                                            value={flagToString(draft, spec.key)}
                                                            onChange={(e) => handleStringChange(spec.key, e.target.value)}
                                                            sx={{ mt: 1 }}
                                                        />
                                                        {spec.suggestions && spec.suggestions.length > 0 && (
                                                            <Stack
                                                                direction="row"
                                                                spacing={0.5}
                                                                useFlexGap
                                                                flexWrap="wrap"
                                                                sx={{ mt: 1 }}
                                                            >
                                                                {spec.suggestions.map((s) => {
                                                                    const selected = flagToString(draft, spec.key) === s.value;
                                                                    return (
                                                                        <Chip
                                                                            key={s.value}
                                                                            // Show the concrete value (the literal UA
                                                                            // string actually sent); the friendly name is
                                                                            // a hover hint so nothing is hidden.
                                                                            label={s.value}
                                                                            title={s.label}
                                                                            size="small"
                                                                            sx={{ fontFamily: 'monospace' }}
                                                                            variant={selected ? 'filled' : 'outlined'}
                                                                            color={selected ? 'primary' : 'default'}
                                                                            onClick={() => handleStringChange(spec.key, s.value)}
                                                                        />
                                                                    );
                                                                })}
                                                            </Stack>
                                                        )}
                                                    </>
                                                )}
                                                {spec.type === 'int' && (
                                                    <TextField
                                                        fullWidth
                                                        size="small"
                                                        type="number"
                                                        placeholder={spec.placeholder}
                                                        value={flagToInt(draft, spec.key) ?? ''}
                                                        slotProps={{ htmlInput: { min: 0 } }}
                                                        onChange={(e) => handleIntChange(spec.key, e.target.value)}
                                                        sx={{ mt: 1 }}
                                                    />
                                                )}
                                                {spec.type === 'enum' && (
                                                    <FormControl fullWidth size="small" sx={{ mt: 1 }}>
                                                        <InputLabel id={`flag-enum-${spec.key}-label`}>
                                                            {spec.label}
                                                        </InputLabel>
                                                        <Select
                                                            labelId={`flag-enum-${spec.key}-label`}
                                                            label={spec.label}
                                                            value={enumValue}
                                                            onChange={(e) => handleStringChange(spec.key, String(e.target.value))}
                                                        >
                                                            {(spec.options || []).map((opt) => (
                                                                <MenuItem key={opt.value} value={opt.value}>
                                                                    {opt.label}
                                                                </MenuItem>
                                                            ))}
                                                        </Select>
                                                    </FormControl>
                                                )}
                                                {spec.type === 'service_ref' && (() => {
                                                    const ref = flagToServiceRef(draft, spec.key);
                                                    const label = ref && ref.provider && ref.model
                                                        ? `${providerName(providers, ref.provider)} / ${ref.model}`
                                                        : 'Select vision model…';
                                                    return (
                                                        <Button
                                                            variant="outlined"
                                                            size="small"
                                                            onClick={() => setPickerKey(spec.key)}
                                                            sx={{ mt: 1, textTransform: 'none', justifyContent: 'flex-start' }}
                                                        >
                                                            {label}
                                                        </Button>
                                                    );
                                                })()}
                                            </Box>
                                        );
                                    })}
                                </Stack>
                            )}
                        </Box>
                    </>
                )}
            </DialogContent>
            <DialogActions>
                <Button onClick={onClose} color="primary">
                    Cancel
                </Button>
                <Button onClick={() => onSave(draft)} color="primary" variant="contained">
                    Save
                </Button>
            </DialogActions>

            {/* Service picker for service_ref flags (e.g. vision_proxy_service). */}
            <Dialog
                open={pickerKey !== null}
                onClose={() => setPickerKey(null)}
                maxWidth="lg"
                fullWidth
                PaperProps={{ sx: { height: '80vh' } }}
            >
                <DialogTitle sx={{ textAlign: 'center' }}>
                    <Typography variant="h6">Pick Vision Proxy Model</Typography>
                </DialogTitle>
                <DialogContent>
                    {pickerKey !== null && (
                        <ModelSelectDialog
                            providers={providers || []}
                            selectedProvider={flagToServiceRef(draft, pickerKey)?.provider}
                            selectedModel={flagToServiceRef(draft, pickerKey)?.model}
                            onSelected={(option: ProviderSelectTabOption) => {
                                const key = pickerKey;
                                setDraft((d) => setFlagValue(d, key, { provider: option.provider.uuid, model: option.model }));
                                setPickerKey(null);
                            }}
                            onSelectionClear={() => {
                                const key = pickerKey;
                                setDraft((d) => setFlagValue(d, key, undefined));
                                setPickerKey(null);
                            }}
                        />
                    )}
                </DialogContent>
            </Dialog>
        </Dialog>
    );
};

// providerName resolves a provider UUID to its display name, falling back to
// the UUID when the provider is unknown (e.g. deleted).
const providerName = (providers: Provider[] | undefined, uuid: string): string =>
    providers?.find((p) => p.uuid === uuid)?.name || uuid;

export default FlagCatalogDialog;

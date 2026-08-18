import { Add as AddIcon, Extension as ExtensionIcon } from '@/components/icons';
import { Box, Stack, Typography } from '@mui/material';
import { alpha } from '@mui/material/styles';
import React, { useEffect, useState } from 'react';
import type { FlagSpec, RuleFlags } from '@/components/RoutingGraphTypes';
import FlagCatalogDialog from '@/components/rule-card/FlagCatalogDialog';
import { headersValue, isFlagActive } from '@/components/rule-card/flagHelpers';
import { api } from '@/services/api';

// Session-level registry cache, same pattern as RuleCard's rule registry.
let registryCache: FlagSpec[] | undefined = undefined;
let registryPromise: Promise<FlagSpec[]> | null = null;

async function loadProviderFlagRegistry(): Promise<FlagSpec[]> {
    if (registryCache !== undefined) return registryCache;
    if (registryPromise) return registryPromise;
    registryPromise = (async () => {
        try {
            const result = await api.getProviderFlagRegistry();
            const data: FlagSpec[] = Array.isArray(result?.data) ? result.data : [];
            registryCache = data;
            return data;
        } catch {
            // Don't cache failures — allow retry on next mount.
            return [];
        } finally {
            registryPromise = null;
        }
    })();
    return registryPromise;
}

export interface ProviderPluginsBlockProps {
    /** Provider-level flags in camelCase form (apiToFlags of the stored value). */
    flags?: RuleFlags;
    onChange: (next: RuleFlags) => void;
}

/**
 * ProviderPluginsBlock — the provider-level Plugins surface inside the
 * provider form's Advanced accordion: a compact card listing active
 * provider-level flags (concrete values, e.g. header names), opening the
 * shared flag catalog dialog fed by the provider registry. Same interaction
 * as the rule-side Plugins card, so the two levels share one mental model.
 */
export const ProviderPluginsBlock: React.FC<ProviderPluginsBlockProps> = ({ flags, onChange }) => {
    const [registry, setRegistry] = useState<FlagSpec[] | undefined>(registryCache);
    const [catalogOpen, setCatalogOpen] = useState(false);

    useEffect(() => {
        let mounted = true;
        loadProviderFlagRegistry().then((specs) => { if (mounted) setRegistry(specs); });
        return () => { mounted = false; };
    }, []);

    // Every provider-registry spec is configurable at this level (the model
    // list edits the same specs per model), so the whole registry renders here.
    const providerSpecs = registry || [];
    const enabled = providerSpecs.filter((spec) => isFlagActive(spec, flags ?? {}));

    return (
        <>
            <Box
                onClick={() => setCatalogOpen(true)}
                sx={(theme) => ({
                    p: 1,
                    border: '1px dashed',
                    borderColor: 'divider',
                    borderRadius: 1,
                    cursor: 'pointer',
                    transition: 'border-color 0.16s ease, background-color 0.16s ease',
                    '&:hover': {
                        borderColor: 'primary.main',
                        backgroundColor: alpha(theme.palette.primary.main, theme.palette.mode === 'dark' ? 0.06 : 0.03),
                    },
                })}
            >
                <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center' }}>
                    <ExtensionIcon sx={{ fontSize: 15, color: 'text.secondary' }} />
                    <Typography variant="caption" sx={{ fontWeight: 700, color: 'text.secondary', flexGrow: 1 }}>
                        Plugins{enabled.length > 0 ? ` (${enabled.length})` : ''}
                    </Typography>
                    <AddIcon sx={{ fontSize: 15, color: 'text.secondary' }} />
                </Stack>
                {enabled.length === 0 ? (
                    <Typography variant="caption" sx={{ color: 'text.disabled', display: 'block', mt: 0.5 }}>
                        None enabled. Click to configure custom headers and other provider plugins.
                    </Typography>
                ) : (
                    <Stack sx={{ gap: 0.4, mt: 0.75 }}>
                        {enabled.map((spec) => {
                            // Show the concrete configured values (e.g. the
                            // literal header names), never a bare count.
                            const detail = spec.type === 'headers'
                                ? Object.keys(headersValue(flags, spec.key)).join(', ')
                                : '';
                            return (
                                <Stack key={spec.key} direction="row" spacing={0.5} sx={{ alignItems: 'baseline', minWidth: 0 }}>
                                    <Typography variant="caption" sx={{ fontWeight: 700, color: 'primary.main', flexShrink: 0 }}>
                                        {spec.label}
                                    </Typography>
                                    {detail && (
                                        <Typography
                                            variant="caption"
                                            sx={{
                                                fontFamily: 'monospace',
                                                color: 'text.primary',
                                                overflow: 'hidden',
                                                textOverflow: 'ellipsis',
                                                whiteSpace: 'nowrap',
                                            }}
                                        >
                                            : {detail}
                                        </Typography>
                                    )}
                                </Stack>
                            );
                        })}
                    </Stack>
                )}
            </Box>
            <FlagCatalogDialog
                open={catalogOpen}
                flags={flags}
                registry={providerSpecs}
                title="Provider Plugins"
                subtitle="Plugin flags applied to every request to this provider."
                onClose={() => setCatalogOpen(false)}
                onSave={(next) => {
                    onChange(next);
                    setCatalogOpen(false);
                }}
            />
        </>
    );
};

export default ProviderPluginsBlock;

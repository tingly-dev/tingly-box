import {Add, Close, Computer, Key, Login, Search, Language, Description, Upload} from '@/components/icons';
import RegionBadge from './RegionBadge';
import {
    Box,
    Card,
    Chip,
    Dialog,
    DialogContent,
    DialogTitle,
    IconButton,
    InputAdornment,
    Stack,
    TextField,
    Typography,
    alpha,
} from '@mui/material';
import React, {useMemo, useState} from 'react';
import {type UniqueProvider, useProviderTemplates} from '../services/serviceProviders';
import ProviderIcon from './ProviderIcon';
import {FALLBACK_OAUTH_PROVIDERS, type OAuthProvider} from './OAuthDialog';

// What the picker emits when a card is chosen. The parent routes each kind to
// the matching existing dialog (API-key form, OAuth flow, or a blank custom
// form).
export type ConnectSelection =
    | {kind: 'key'; provider: UniqueProvider}
    | {kind: 'oauth'; providerId: string}
    | {kind: 'local'; provider: UniqueProvider}
    | {kind: 'custom'}
    | {kind: 'import'};

interface ConnectProviderDialogProps {
    open: boolean;
    onClose: () => void;
    onSelect: (selection: ConnectSelection) => void;
    hideOfficialInfo?: boolean; // If true, hide the official info description
}

interface ProviderListContentProps {
    onSelect: (selection: ConnectSelection) => void;
    query: string;
    onQueryChange: (value: string) => void;
    hideOfficialInfo?: boolean;
    showDetails?: boolean; // If true, show website links and other details
    wide?: boolean; // If true, use wider grid layout (2-3 columns)
}

type Accent = 'custom' | 'oauth' | 'key' | 'local';

const ACCENT: Record<Accent, string> = {
    custom: 'secondary.main',
    oauth: 'success.main',
    key: 'primary.main',
    local: 'warning.main',
};

const SectionHeader: React.FC<{icon: React.ReactNode; title: string; count?: number; accent: Accent}> = ({
    icon, title, count, accent,
}) => {
    const color = ACCENT[accent];
    return (
        <Stack
            direction="row"
            alignItems="center"
            spacing={1.25}
            sx={{mt: 2, mb: 1.25, pb: 0.75, borderBottom: 1, borderColor: 'divider'}}
        >
            <Box
                sx={{
                    width: 26, height: 26, borderRadius: '50%',
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                    color,
                    bgcolor: (theme) => alpha(theme.palette[accent === 'key' ? 'primary' : accent === 'oauth' ? 'success' : accent === 'local' ? 'warning' : 'secondary'].main, 0.12),
                }}
            >
                {icon}
            </Box>
            <Typography variant="subtitle2" fontWeight={700}>{title}</Typography>
            {typeof count === 'number' && (
                <Chip label={count} size="small" sx={{height: 18, fontWeight: 600}}/>
            )}
        </Stack>
    );
};

const ProviderCard: React.FC<{
    icon: React.ReactNode;
    name: string;
    meta: string;
    badge: {label: string; color: string; bg: string};
    onClick: () => void;
    website?: string;
    apiDoc?: string;
    showDetails?: boolean; // If true, show website and API doc icon buttons
}> = ({icon, name, meta, badge, onClick, website, apiDoc, showDetails = false}) => {
    return (
        <Card
            variant="outlined"
            sx={{
                position: 'relative',
                border: 1, borderColor: 'divider', borderRadius: 1,
                p: 1.25, display: 'flex', alignItems: 'center', gap: 1.25,
                cursor: 'pointer',
                boxShadow: 0,
                transition: 'border-color 0.15s ease, background-color 0.15s ease',
                '&:hover': {
                    borderColor: 'primary.main',
                    bgcolor: (theme) => alpha(theme.palette.primary.main, 0.04),
                },
            }}
            onClick={onClick}
        >
            <Box sx={{width: 30, height: 30, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0}}>
                {icon}
            </Box>
            <Box sx={{minWidth: 0, flex: 1}}>
                <Typography
                    variant="body2"
                    fontWeight={600}
                    sx={{
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        display: '-webkit-box',
                        WebkitLineClamp: 2,
                        WebkitBoxOrient: 'vertical',
                        lineHeight: 1.3,
                    }}
                    title={name}
                >
                    {name}
                </Typography>
                <Typography variant="caption" color="text.disabled" noWrap sx={{display: 'block', mt: 0.25, fontSize: '0.68rem', letterSpacing: '0.01em'}}>
                    {meta}
                </Typography>
            </Box>
            {showDetails && (
                <Stack direction="row" spacing={0.3} sx={{pr: 10}}>
                    {website && (
                        <IconButton
                            size="small"
                            href={website}
                            target="_blank"
                            rel="noopener noreferrer"
                            title="Official Website"
                            onClick={(e) => e.stopPropagation()}
                        >
                            <Language fontSize="small"/>
                        </IconButton>
                    )}
                    {apiDoc && (
                        <IconButton
                            size="small"
                            href={apiDoc}
                            target="_blank"
                            rel="noopener noreferrer"
                            title="API Documentation"
                            onClick={(e) => e.stopPropagation()}
                        >
                            <Description fontSize="small"/>
                        </IconButton>
                    )}
                </Stack>
            )}
            <Typography
                component="span"
                sx={{
                    position: 'absolute', top: 6, right: 6,
                    fontSize: '0.6rem', fontWeight: 600, lineHeight: 1,
                    color: badge.color, opacity: 0.7,
                    px: 0.5, py: 0.25,
                    borderRadius: 0.5,
                    bgcolor: badge.bg,
                }}
            >
                {badge.label}
            </Typography>
        </Card>
    );
};

const CardGrid: React.FC<{children: React.ReactNode; single?: boolean; wide?: boolean}> = ({children, single, wide}) => {
    // single: force 1 column (for API key providers in dialog mode)
    // wide: 3 columns for onboarding/page mode
    // default: 1-2 columns (for cards in dialog mode)
    if (single) {
        return (
            <Box sx={{display: 'grid', gridTemplateColumns: '1fr', gap: 1}}>
                {children}
            </Box>
        );
    }
    if (wide) {
        return (
            <Box sx={{
                display: 'grid',
                gridTemplateColumns: {
                    xs: '1fr',
                    sm: 'repeat(3, 1fr)',
                },
                gap: 1.5,
            }}>
                {children}
            </Box>
        );
    }
    return (
        <Box sx={{
            display: 'grid',
            gridTemplateColumns: {xs: '1fr', sm: '1fr 1fr'},
            gap: 1,
        }}>
            {children}
        </Box>
    );
};

// Extracted content component that can be used standalone (e.g., in onboarding)
export const ProviderListContent: React.FC<ProviderListContentProps> = ({
    onSelect,
    query,
    onQueryChange,
    hideOfficialInfo = false,
    showDetails = false,
    wide = false,
}) => {
    const keyProviders = useProviderTemplates();

    const oauthProviders = useMemo(
        () => FALLBACK_OAUTH_PROVIDERS.filter(
            (p: OAuthProvider) => p.enabled !== false && (!p.dev || import.meta.env.DEV)
        ),
        []
    );

    const needle = query.trim().toLowerCase();
    const filteredKey = needle
        ? keyProviders.filter((p) => (p.alias || p.name).toLowerCase().includes(needle))
        : keyProviders;
    const filteredOAuth = needle
        ? oauthProviders.filter((p) => `${p.name} ${p.displayName}`.toLowerCase().includes(needle))
        : oauthProviders;
    const showCustom = !needle || 'custom endpoint import'.includes(needle);

    // Group key providers by region (CN vs Global vs Self-hosted)
    const {cnKeyProviders, globalKeyProviders, selfHostedProviders} = useMemo(() => {
        const cn: UniqueProvider[] = [];
        const global: UniqueProvider[] = [];
        const selfHosted: UniqueProvider[] = [];
        filteredKey.forEach((p) => {
            if (p.region === 'self-hosted') {
                selfHosted.push(p);
            } else if (p.region === 'cn') {
                cn.push(p);
            } else {
                global.push(p);
            }
        });
        return {cnKeyProviders: cn, globalKeyProviders: global, selfHostedProviders: selfHosted};
    }, [filteredKey]);

    const keyBadge = {label: 'Key', color: '#1967d2', bg: '#e8f0fe'};
    const oauthBadge = {label: 'OAuth', color: '#1e8e3e', bg: '#e6f4ea'};
    const selfHostedBadge = {label: 'Self-hosted', color: '#e37400', bg: '#fef3e0'};
    const cnBadge = {label: 'CN', color: '#d93025', bg: '#fce8e6'};
    const globalBadge = {label: 'Global', color: '#1967d2', bg: '#e8f0fe'};

    const protocolMeta = (p: UniqueProvider) =>
        [p.supportsOpenAI && 'OpenAI', p.supportsAnthropic && 'Anthropic'].filter(Boolean).join(' · ') || 'Custom API';

    const nothing = filteredKey.length === 0 && filteredOAuth.length === 0 && selfHostedProviders.length === 0 && !showCustom;

    return (
        <Box>
            {!hideOfficialInfo && (
                <Typography variant="body2" color="text.secondary" sx={{mb: 1.5}}>
                    Pick a provider. We&apos;ll ask only for what that provider needs.
                </Typography>
            )}
            <TextField
                fullWidth
                size="small"
                placeholder="Search providers…"
                value={query}
                onChange={(e) => onQueryChange(e.target.value)}
                InputProps={{
                    startAdornment: (
                        <InputAdornment position="start"><Search fontSize="small"/></InputAdornment>
                    ),
                    sx: {
                        borderRadius: 1,
                        mb: 2,
                    },
                }}
            />
            <Box
                sx={{
                    pt: 1,
                    maxHeight: '60vh',
                    overflowY: 'auto',
                    scrollbarWidth: 'thin',
                    '&::-webkit-scrollbar': {width: 8},
                    '&::-webkit-scrollbar-thumb': {
                        backgroundColor: 'action.disabled',
                        borderRadius: 4,
                    },
                    '&::-webkit-scrollbar-track': {backgroundColor: 'transparent'},
                }}
            >
                {showCustom && (
                    <>
                        <SectionHeader icon={<Add fontSize="small"/>} title="Custom" accent="custom"/>
                        <CardGrid wide={wide}>
                            <ProviderCard
                                icon={<Add/>}
                                name="Custom endpoint"
                                meta="Any compatible API"
                                badge={keyBadge}
                                onClick={() => onSelect({kind: 'custom'})}
                            />
                            <ProviderCard
                                icon={<Upload/>}
                                name="Import"
                                meta="From file or clipboard"
                                badge={keyBadge}
                                onClick={() => onSelect({kind: 'import'})}
                            />
                        </CardGrid>
                    </>
                )}

                {filteredOAuth.length > 0 && (
                    <>
                        <SectionHeader icon={<Login fontSize="small"/>} title="OAuth sign-in" count={filteredOAuth.length} accent="oauth"/>
                        <CardGrid wide={wide}>
                            {filteredOAuth.map((p) => (
                                <ProviderCard
                                    key={`oauth-${p.id}`}
                                    icon={p.icon}
                                    name={p.name}
                                    meta="OAuth sign-in"
                                    badge={oauthBadge}
                                    showDetails={showDetails}
                                    onClick={() => onSelect({kind: 'oauth', providerId: p.id})}
                                />
                            ))}
                        </CardGrid>
                    </>
                )}

                {selfHostedProviders.length > 0 && (
                    <>
                        <SectionHeader icon={<Computer fontSize="small"/>} title="Self-hosted" count={selfHostedProviders.length} accent="local"/>
                        <CardGrid wide={wide}>
                            {selfHostedProviders.map((p) => (
                                <ProviderCard
                                    key={`self-${p.id}`}
                                    icon={<ProviderIcon identifier={p.icon || p.id} size={26}/>}
                                    name={p.alias || p.name}
                                    meta={p.baseUrlOpenAI || p.baseUrlAnthropic || ''}
                                    badge={selfHostedBadge}
                                    website={showDetails ? p.website : undefined}
                                    apiDoc={showDetails ? p.apiDoc : undefined}
                                    showDetails={showDetails}
                                    onClick={() => onSelect({kind: 'local', provider: p})}
                                />
                            ))}
                        </CardGrid>
                    </>
                )}

                {filteredKey.length > 0 && (
                    <>
                        <SectionHeader icon={<Key fontSize="small"/>} title="API key providers" count={filteredKey.length} accent="key"/>

                        {cnKeyProviders.length > 0 && (
                            <Box sx={{mb: 2}}>
                                <Stack direction="row" alignItems="center" spacing={1} sx={{px: 0.5, mb: 1}}>
                                    <RegionBadge region="cn" size="medium" />
                                    <Typography variant="caption" color="text.secondary">
                                        {cnKeyProviders.length} providers
                                    </Typography>
                                </Stack>
                                <CardGrid wide={wide}>
                                    {cnKeyProviders.map((p) => (
                                        <ProviderCard
                                            key={`key-${p.id}`}
                                            icon={<ProviderIcon identifier={p.icon || p.id} size={26}/>}
                                            name={p.alias || p.name}
                                            meta={protocolMeta(p)}
                                            badge={cnBadge}
                                            website={showDetails ? p.website : undefined}
                                            apiDoc={showDetails ? p.apiDoc : undefined}
                                            showDetails={showDetails}
                                            onClick={() => onSelect({kind: 'key', provider: p})}
                                        />
                                    ))}
                                </CardGrid>
                            </Box>
                        )}

                        {globalKeyProviders.length > 0 && (
                            <Box>
                                <Stack direction="row" alignItems="center" spacing={1} sx={{px: 0.5, mb: 1}}>
                                    <RegionBadge region="global" size="medium" />
                                    <Typography variant="caption" color="text.secondary">
                                        {globalKeyProviders.length} providers
                                    </Typography>
                                </Stack>
                                <CardGrid wide={wide}>
                                    {globalKeyProviders.map((p) => (
                                        <ProviderCard
                                            key={`key-${p.id}`}
                                            icon={<ProviderIcon identifier={p.icon || p.id} size={26}/>}
                                            name={p.alias || p.name}
                                            meta={protocolMeta(p)}
                                            badge={globalBadge}
                                            website={showDetails ? p.website : undefined}
                                            apiDoc={showDetails ? p.apiDoc : undefined}
                                            showDetails={showDetails}
                                            onClick={() => onSelect({kind: 'key', provider: p})}
                                        />
                                    ))}
                                </CardGrid>
                            </Box>
                        )}
                    </>
                )}

                {nothing && (
                    <Typography variant="body2" color="text.secondary" sx={{textAlign: 'center', py: 3}}>
                        No providers match &ldquo;{query}&rdquo;.
                    </Typography>
                )}
            </Box>
        </Box>
    );
};

const ConnectProviderDialog: React.FC<ConnectProviderDialogProps> = ({open, onClose, onSelect, hideOfficialInfo = false}) => {
    const [query, setQuery] = useState('');

    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth="sm"
            fullWidth
            scroll="paper"
            PaperProps={{sx: {maxHeight: '82vh', display: 'flex', flexDirection: 'column'}}}
        >
            {/* Locked header: title and close button never scroll. */}
            <DialogTitle sx={{pb: 1, flexShrink: 0}}>
                <Stack direction="row" alignItems="center" justifyContent="space-between">
                    <Typography variant="h6">Connect AI</Typography>
                    <IconButton onClick={onClose} size="small"><Close/></IconButton>
                </Stack>
            </DialogTitle>
            <DialogContent
                dividers
                sx={{
                    pt: 2,
                    flex: 1,
                    overflowY: 'hidden', // Content handles its own scrolling
                }}
            >
                <ProviderListContent
                    onSelect={onSelect}
                    query={query}
                    onQueryChange={setQuery}
                    hideOfficialInfo={hideOfficialInfo}
                    showDetails={false}
                    wide={false}
                />
            </DialogContent>
        </Dialog>
    );
};

export default ConnectProviderDialog;

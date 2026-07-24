import {Add, Close, Computer, Key, Login, Search, Language, Description, Upload} from '@/components/icons';
import RegionBadge from './RegionBadge';
import {
    Box,
    ButtonBase,
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
import {type UniqueProvider, useProviderTemplates, searchProviders} from '../services/serviceProviders';
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
            spacing={1.25}
            sx={{
                alignItems: "center",
                mt: 2,
                mb: 1.25,
                pb: 0.75,
                borderBottom: 1,
                borderColor: 'divider'
            }}>
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
            <Typography variant="subtitle2" sx={{
                fontWeight: 700
            }}>{title}</Typography>
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
    badge: {
        label: string;
        tone: 'primary' | 'success' | 'warning' | 'error';
    };
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
                display: 'flex', alignItems: 'stretch',
                boxShadow: 0,
                transition: 'border-color 0.15s ease, background-color 0.15s ease',
                '&:hover': {
                    borderColor: 'primary.main',
                    bgcolor: (theme) => alpha(theme.palette.primary.main, 0.04),
                },
            }}
        >
            <ButtonBase
                onClick={onClick}
                onKeyDown={(event) => {
                    if (event.key === 'Enter' || event.key === ' ') {
                        event.preventDefault();
                        onClick();
                    }
                }}
                aria-label={`${name}: ${meta}`}
                sx={{
                    width: '100%',
                    minWidth: 0,
                    p: 1.25,
                    pr: showDetails ? 9 : 1.25,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'flex-start',
                    gap: 1.25,
                    borderRadius: 'inherit',
                    textAlign: 'left',
                    '&.Mui-focusVisible': {
                        outline: '2px solid',
                        outlineColor: 'primary.main',
                        outlineOffset: -2,
                    },
                }}
            >
                <Box sx={{width: 30, height: 30, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0}}>
                    {icon}
                </Box>
                <Box sx={{minWidth: 0, flex: 1}}>
                    <Typography
                        variant="body2"
                        title={name}
                        sx={{
                            fontWeight: 600,
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            display: '-webkit-box',
                            WebkitLineClamp: 2,
                            WebkitBoxOrient: 'vertical',
                            lineHeight: 1.3
                        }}>
                        {name}
                    </Typography>
                    <Typography
                        variant="caption"
                        noWrap
                        sx={{
                            color: "text.disabled",
                            display: 'block',
                            mt: 0.25,
                            fontSize: '0.68rem',
                            letterSpacing: '0.01em'
                        }}>
                        {meta}
                    </Typography>
                </Box>
            </ButtonBase>
            {showDetails && (
                <Stack
                    direction="row"
                    spacing={0.3}
                    sx={{
                        position: 'absolute',
                        right: 6,
                        bottom: 4,
                    }}
                >
                    {website && (
                        <IconButton
                            size="small"
                            href={website}
                            target="_blank"
                            rel="noopener noreferrer"
                            title="Official Website"
                            aria-label={`Open ${name} official website`}
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
                            aria-label={`Open ${name} API documentation`}
                        >
                            <Description fontSize="small"/>
                        </IconButton>
                    )}
                </Stack>
            )}
            <Typography
                component="span"
                sx={(theme) => ({
                    position: 'absolute', top: 6, right: 6,
                    fontSize: '0.6rem', fontWeight: 600, lineHeight: 1,
                    color: theme.palette[badge.tone].main,
                    px: 0.5, py: 0.25,
                    borderRadius: 0.5,
                    bgcolor: alpha(theme.palette[badge.tone].main, 0.1),
                    pointerEvents: 'none',
                })}
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
            gridTemplateColumns: {xs: '1fr', sm: '1fr 1fr', lg: 'repeat(3, 1fr)'},
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

    const needle = query.trim();
    const filteredKey = needle
        ? searchProviders(keyProviders, needle)
        : keyProviders;
    const filteredOAuth = needle
        ? oauthProviders.filter((p) => `${p.name} ${p.displayName} ${p.id}`.toLowerCase().includes(needle.toLowerCase()))
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

    const keyBadge = {label: 'Key', tone: 'primary'} as const;
    const oauthBadge = {label: 'OAuth', tone: 'success'} as const;
    const selfHostedBadge = {label: 'Self-hosted', tone: 'warning'} as const;
    const cnBadge = {label: 'CN', tone: 'error'} as const;
    const globalBadge = {label: 'Global', tone: 'primary'} as const;

    const protocolMeta = (p: UniqueProvider) =>
        [p.supportsOpenAI && 'OpenAI', p.supportsAnthropic && 'Anthropic'].filter(Boolean).join(' · ') || 'Custom API';

    const nothing = filteredKey.length === 0 && filteredOAuth.length === 0 && selfHostedProviders.length === 0 && !showCustom;

    return (
        <Box>
            {!hideOfficialInfo && (
                <Typography
                    variant="body2"
                    sx={{
                        color: "text.secondary",
                        mb: 1.5
                    }}>
                    Search for your provider below — most are pre-configured. Not listed? Pick <Box component="span" sx={{fontWeight: 600, color: 'text.primary'}}>Custom endpoint</Box> to enter any base URL yourself.
                </Typography>
            )}
            <TextField
                fullWidth
                size="small"
                placeholder="Search providers…"
                value={query}
                onChange={(e) => onQueryChange(e.target.value)}
                slotProps={{
                    htmlInput: {
                        'aria-label': 'Search providers',
                    },
                    input: {
                        startAdornment: (
                            <InputAdornment position="start"><Search fontSize="small"/></InputAdornment>
                        ),
                        sx: {
                            borderRadius: 1,
                            mb: 2,
                        },
                    }
                }}
            />
            <Box
                sx={{
                    pt: 1,
                    maxHeight: '70vh',
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
                                meta="Not listed? Bring your own URL"
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
                                <Stack
                                    direction="row"
                                    spacing={1}
                                    sx={{
                                        alignItems: "center",
                                        px: 0.5,
                                        mb: 1
                                    }}>
                                    <RegionBadge region="cn" size="medium" />
                                    <Typography variant="caption" sx={{
                                        color: "text.secondary"
                                    }}>
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
                                <Stack
                                    direction="row"
                                    spacing={1}
                                    sx={{
                                        alignItems: "center",
                                        px: 0.5,
                                        mb: 1
                                    }}>
                                    <RegionBadge region="global" size="medium" />
                                    <Typography variant="caption" sx={{
                                        color: "text.secondary"
                                    }}>
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
                    <Typography
                        variant="body2"
                        sx={{
                            color: "text.secondary",
                            textAlign: 'center',
                            py: 3
                        }}>
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
            aria-labelledby="connect-ai-dialog-title"
            maxWidth="sm"
            fullWidth
            scroll="paper"
            slotProps={{
                paper: {sx: {maxHeight: '88vh', display: 'flex', flexDirection: 'column', maxWidth: {lg: '900px'}}}
            }}
        >
            {/* Locked header: title and close button never scroll. */}
            <DialogTitle id="connect-ai-dialog-title" sx={{pb: 1, flexShrink: 0}}>
                <Stack
                    direction="row"
                    sx={{
                        alignItems: "center",
                        justifyContent: "space-between"
                    }}>
                    <Typography component="span" variant="h6">Connect AI</Typography>
                    <IconButton aria-label="Close Connect AI" onClick={onClose} size="small"><Close/></IconButton>
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

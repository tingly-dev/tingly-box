import {Add, Close, Key, Login, Search} from '@mui/icons-material';
import {
    Box,
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
    | {kind: 'custom'};

interface ConnectProviderDialogProps {
    open: boolean;
    onClose: () => void;
    onSelect: (selection: ConnectSelection) => void;
}

type Accent = 'custom' | 'oauth' | 'key';

const ACCENT: Record<Accent, string> = {
    custom: 'secondary.main',
    oauth: 'success.main',
    key: 'primary.main',
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
                    bgcolor: (theme) => alpha(theme.palette[accent === 'key' ? 'primary' : accent === 'oauth' ? 'success' : 'secondary'].main, 0.12),
                }}
            >
                {icon}
            </Box>
            <Typography variant="subtitle2" fontWeight={700}>{title}</Typography>
            {typeof count === 'number' && (
                <Chip label={count} size="small" sx={{height: 18, fontSize: '0.65rem', fontWeight: 600}}/>
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
}> = ({icon, name, meta, badge, onClick}) => (
    <Box
        onClick={onClick}
        sx={{
            position: 'relative',
            border: 1, borderColor: 'divider', borderRadius: 2,
            p: 1.25, display: 'flex', alignItems: 'center', gap: 1.25,
            cursor: 'pointer', transition: 'border-color 0.15s, box-shadow 0.15s',
            '&:hover': {borderColor: 'primary.main', boxShadow: 1},
        }}
    >
        <Box sx={{width: 30, height: 30, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0}}>
            {icon}
        </Box>
        <Box sx={{minWidth: 0, flex: 1}}>
            <Typography variant="body2" fontWeight={600} noWrap>{name}</Typography>
            <Typography variant="caption" color="text.secondary" noWrap sx={{display: 'block'}}>{meta}</Typography>
        </Box>
        <Chip
            label={badge.label}
            size="small"
            sx={{
                position: 'absolute', top: 6, right: 6, height: 18,
                fontSize: '0.62rem', fontWeight: 700, color: badge.color, bgcolor: badge.bg,
            }}
        />
    </Box>
);

const CardGrid: React.FC<{children: React.ReactNode; single?: boolean}> = ({children, single}) => (
    <Box sx={{
        display: 'grid',
        gridTemplateColumns: single ? '1fr' : {xs: '1fr', sm: '1fr 1fr'},
        gap: 1,
    }}>{children}</Box>
);

const ConnectProviderDialog: React.FC<ConnectProviderDialogProps> = ({open, onClose, onSelect}) => {
    const [query, setQuery] = useState('');
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
    const showCustom = !needle || 'custom endpoint'.includes(needle);

    const keyBadge = {label: 'Key', color: '#1967d2', bg: '#e8f0fe'};
    const oauthBadge = {label: 'OAuth', color: '#1e8e3e', bg: '#e6f4ea'};

    const protocolMeta = (p: UniqueProvider) =>
        [p.supportsOpenAI && 'OpenAI', p.supportsAnthropic && 'Anthropic'].filter(Boolean).join(' · ') || 'Custom API';

    const nothing = filteredKey.length === 0 && filteredOAuth.length === 0 && !showCustom;

    return (
        <Dialog
            open={open}
            onClose={onClose}
            maxWidth="sm"
            fullWidth
            scroll="paper"
            PaperProps={{sx: {maxHeight: '82vh', display: 'flex', flexDirection: 'column'}}}
        >
            {/* Locked header: title, description and search never scroll. */}
            <DialogTitle sx={{pb: 1, flexShrink: 0}}>
                <Stack direction="row" alignItems="center" justifyContent="space-between">
                    <Typography variant="h6">Connect AI</Typography>
                    <IconButton onClick={onClose} size="small"><Close/></IconButton>
                </Stack>
            </DialogTitle>
            <Box sx={{px: 3, pb: 1.5, flexShrink: 0}}>
                <Typography variant="body2" color="text.secondary" sx={{mb: 1.5}}>
                    Pick a provider. We&apos;ll ask only for what that provider needs.
                </Typography>
                <TextField
                    fullWidth
                    size="small"
                    placeholder="Search providers…"
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    InputProps={{
                        startAdornment: (
                            <InputAdornment position="start"><Search fontSize="small"/></InputAdornment>
                        ),
                    }}
                />
            </Box>
            <DialogContent
                dividers
                sx={{
                    pt: 1,
                    flex: 1,
                    overflowY: 'auto',
                    // Keep the scrollbar visible so it's obvious the list scrolls.
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
                        <CardGrid>
                            <ProviderCard
                                icon={<Add/>}
                                name="Custom endpoint"
                                meta="Any compatible API"
                                badge={keyBadge}
                                onClick={() => onSelect({kind: 'custom'})}
                            />
                        </CardGrid>
                    </>
                )}

                {filteredOAuth.length > 0 && (
                    <>
                        <SectionHeader icon={<Login fontSize="small"/>} title="OAuth sign-in" count={filteredOAuth.length} accent="oauth"/>
                        <CardGrid>
                            {filteredOAuth.map((p) => (
                                <ProviderCard
                                    key={`oauth-${p.id}`}
                                    icon={p.icon}
                                    name={p.name}
                                    meta="OAuth sign-in"
                                    badge={oauthBadge}
                                    onClick={() => onSelect({kind: 'oauth', providerId: p.id})}
                                />
                            ))}
                        </CardGrid>
                    </>
                )}

                {filteredKey.length > 0 && (
                    <>
                        <SectionHeader icon={<Key fontSize="small"/>} title="API key providers" count={filteredKey.length} accent="key"/>
                        <CardGrid single>
                            {filteredKey.map((p) => (
                                <ProviderCard
                                    key={`key-${p.id}`}
                                    icon={<ProviderIcon identifier={p.icon || p.id} size={26}/>}
                                    name={p.alias || p.name}
                                    meta={protocolMeta(p)}
                                    badge={keyBadge}
                                    onClick={() => onSelect({kind: 'key', provider: p})}
                                />
                            ))}
                        </CardGrid>
                    </>
                )}

                {nothing && (
                    <Typography variant="body2" color="text.secondary" sx={{textAlign: 'center', py: 3}}>
                        No providers match &ldquo;{query}&rdquo;.
                    </Typography>
                )}
            </DialogContent>
        </Dialog>
    );
};

export default ConnectProviderDialog;

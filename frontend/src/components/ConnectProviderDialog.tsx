import {Add, Close, Search} from '@mui/icons-material';
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
            border: 1,
            borderColor: 'divider',
            borderRadius: 2,
            p: 1.25,
            display: 'flex',
            alignItems: 'center',
            gap: 1.25,
            cursor: 'pointer',
            transition: 'border-color 0.15s, box-shadow 0.15s',
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
                position: 'absolute',
                top: 6,
                right: 6,
                height: 18,
                fontSize: '0.62rem',
                fontWeight: 700,
                color: badge.color,
                bgcolor: badge.bg,
            }}
        />
    </Box>
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
    const matchesKey = (p: UniqueProvider) => (p.alias || p.name).toLowerCase().includes(needle);
    const matchesOAuth = (p: OAuthProvider) => `${p.name} ${p.displayName}`.toLowerCase().includes(needle);

    const filteredKey = needle ? keyProviders.filter(matchesKey) : keyProviders;
    const filteredOAuth = needle ? oauthProviders.filter(matchesOAuth) : oauthProviders;
    const showCustom = !needle || 'custom endpoint'.includes(needle);

    const keyBadge = {label: 'Key', color: '#1967d2', bg: '#e8f0fe'};
    const oauthBadge = {label: 'OAuth', color: '#1e8e3e', bg: '#e6f4ea'};

    const protocolMeta = (p: UniqueProvider) =>
        [p.supportsOpenAI && 'OpenAI', p.supportsAnthropic && 'Anthropic'].filter(Boolean).join(' · ') || 'Custom API';

    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle>
                <Stack direction="row" alignItems="center" justifyContent="space-between">
                    <Typography variant="h6">Connect a Provider</Typography>
                    <IconButton onClick={onClose} size="small"><Close/></IconButton>
                </Stack>
            </DialogTitle>
            <DialogContent>
                <Typography variant="body2" color="text.secondary" sx={{mb: 2}}>
                    Pick a provider. We&apos;ll ask only for what that provider needs.
                </Typography>
                <TextField
                    fullWidth
                    size="small"
                    placeholder="Search providers…"
                    value={query}
                    onChange={(e) => setQuery(e.target.value)}
                    sx={{mb: 2}}
                    InputProps={{
                        startAdornment: (
                            <InputAdornment position="start"><Search fontSize="small"/></InputAdornment>
                        ),
                    }}
                />
                <Box sx={{display: 'grid', gridTemplateColumns: {xs: '1fr', sm: '1fr 1fr'}, gap: 1}}>
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
                    {showCustom && (
                        <ProviderCard
                            icon={<Add/>}
                            name="Custom endpoint"
                            meta="Any compatible API"
                            badge={keyBadge}
                            onClick={() => onSelect({kind: 'custom'})}
                        />
                    )}
                </Box>
                {filteredKey.length === 0 && filteredOAuth.length === 0 && !showCustom && (
                    <Typography variant="body2" color="text.secondary" sx={{textAlign: 'center', py: 3}}>
                        No providers match &ldquo;{query}&rdquo;.
                    </Typography>
                )}
            </DialogContent>
        </Dialog>
    );
};

export default ConnectProviderDialog;

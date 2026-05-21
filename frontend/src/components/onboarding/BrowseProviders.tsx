import {useMemo, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {
    Box,
    Card,
    CardActionArea,
    Chip,
    IconButton,
    InputAdornment,
    Stack,
    TextField,
    Typography,
    alpha,
} from '@mui/material';
import SearchIcon from '@mui/icons-material/Search';
import LanguageIcon from '@mui/icons-material/Language';
import DescriptionIcon from '@mui/icons-material/Description';
import AddIcon from '@mui/icons-material/Add';
import PublicIcon from '@mui/icons-material/Public';
import LocationOnIcon from '@mui/icons-material/LocationOn';
import ProviderIcon from '@/components/ProviderIcon';
import {useProviderTemplates, type UniqueProvider} from '@/services/serviceProviders';
import type {EnhancedProviderFormData} from '@/components/ProviderFormDialog';

interface BrowseProvidersProps {
    onPick: (prefill: EnhancedProviderFormData) => void;
}

// Empty form data for custom provider entry
const emptyForm = (): EnhancedProviderFormData => ({
    name: '',
    apiBase: '',
    apiStyle: undefined,
    token: '',
    enabled: true,
    noKeyRequired: false,
    proxyUrl: '',
});

const CARD_HEIGHT = 84;

const cardSx = {
    borderRadius: 1,
    height: CARD_HEIGHT,
    display: 'flex',
    flexDirection: 'column' as const,
    boxShadow: 'none',
    transition: 'border-color 0.16s ease, background-color 0.16s ease',
    '&:hover': {
        borderColor: 'primary.main',
        bgcolor: (theme) => alpha(theme.palette.primary.main, 0.04),
    },
};

const cardActionAreaSx = {
    height: CARD_HEIGHT,
    p: 1.5,
    display: 'flex',
    alignItems: 'center',
};

const gridSx = {
    display: 'grid',
    gap: 1.5,
    gridTemplateColumns: {
        xs: '1fr',
        sm: 'repeat(2, 1fr)',
        md: 'repeat(3, 1fr)',
        xl: 'repeat(4, 1fr)',
    },
};

interface SectionHeaderProps {
    icon: React.ReactNode;
    title: string;
    count?: number;
    accent: 'global' | 'cn' | 'custom';
}

const SectionHeader: React.FC<SectionHeaderProps> = ({icon, title, count, accent}) => {
    const accentColor =
        accent === 'cn' ? 'error.main' : accent === 'custom' ? 'secondary.main' : 'primary.main';
    const accentBg =
        accent === 'cn' ? 'error.50' : accent === 'custom' ? 'secondary.50' : 'primary.50';
    return (
        <Stack
            direction="row"
            alignItems="center"
            spacing={1.25}
            sx={{
                mt: 2.5,
                mb: 1.5,
                pb: 1,
                borderBottom: 1,
                borderColor: 'divider',
            }}
        >
            <Box
                sx={{
                    width: 28,
                    height: 28,
                    borderRadius: '50%',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    bgcolor: accentBg,
                    color: accentColor,
                }}
            >
                {icon}
            </Box>
            <Typography variant="subtitle1" fontWeight={700} sx={{color: 'text.primary'}}>
                {title}
            </Typography>
            {typeof count === 'number' && (
                <Chip
                    label={count}
                    size="small"
                    sx={{
                        height: 20,
                        fontSize: '0.7rem',
                        fontWeight: 600,
                        bgcolor: accentBg,
                        color: accentColor,
                        border: 'none',
                    }}
                />
            )}
        </Stack>
    );
};

const CustomProviderCard: React.FC<{onPick: () => void}> = ({onPick}) => {
    const {t} = useTranslation();
    return (
        <Card
            variant="outlined"
            sx={{
                ...cardSx,
                borderStyle: 'dashed',
                borderColor: 'primary.main',
                bgcolor: 'primary.50',
                '&:hover': {
                    bgcolor: (theme) => alpha(theme.palette.primary.main, 0.08),
                    borderColor: 'primary.dark',
                },
            }}
        >
            <CardActionArea onClick={onPick} sx={cardActionAreaSx}>
                <Stack direction="row" spacing={1.5} alignItems="center" width="100%">
                    <Box
                        sx={{
                            width: 32,
                            height: 32,
                            borderRadius: 1,
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            bgcolor: 'primary.main',
                            color: 'primary.contrastText',
                        }}
                    >
                        <AddIcon sx={{fontSize: 22}}/>
                    </Box>
                    <Box sx={{minWidth: 0, flex: 1}}>
                        <Typography variant="subtitle2" fontWeight={700} color="primary.main" noWrap>
                            {t('onboarding.browse.customProvider', {defaultValue: 'Custom Provider'})}
                        </Typography>
                        <Typography variant="caption" color="text.secondary" noWrap>
                            {t('onboarding.browse.customProviderHint', {defaultValue: 'Bring your own endpoint'})}
                        </Typography>
                    </Box>
                </Stack>
            </CardActionArea>
        </Card>
    );
};

interface ProviderCardProps {
    provider: UniqueProvider;
    onPick: (p: UniqueProvider) => void;
}

const ProviderCard: React.FC<ProviderCardProps> = ({provider: p, onPick}) => {
    return (
        <Card variant="outlined" sx={cardSx}>
            <CardActionArea onClick={() => onPick(p)} sx={cardActionAreaSx}>
                <Stack direction="row" spacing={1.5} alignItems="center" justifyContent="space-between" width="100%">
                    <Stack direction="row" spacing={1.5} alignItems="center" sx={{minWidth: 0, flex: 1}}>
                        <ProviderIcon identifier={p.icon || p.id} size={32}/>
                        <Box sx={{minWidth: 0, flex: 1}}>
                            <Typography
                                variant="subtitle2"
                                fontWeight={600}
                                sx={{
                                    overflow: 'hidden',
                                    textOverflow: 'ellipsis',
                                    display: '-webkit-box',
                                    WebkitLineClamp: 2,
                                    WebkitBoxOrient: 'vertical',
                                    lineHeight: 1.3,
                                }}
                                title={p.alias || p.name}
                            >
                                {p.alias || p.name}
                            </Typography>
                            <Stack direction="row" spacing={0.5} sx={{mt: 0.5}}>
                                {p.supportsOpenAI && (
                                    <Chip label="OpenAI" size="small" sx={{height: 18, fontSize: '0.65rem'}}/>
                                )}
                                {p.supportsAnthropic && (
                                    <Chip label="Anthropic" size="small" sx={{height: 18, fontSize: '0.65rem'}}/>
                                )}
                            </Stack>
                        </Box>
                    </Stack>
                    <Stack direction="row" spacing={0.3}>
                        {p.website && (
                            <IconButton
                                size="small"
                                href={p.website}
                                target="_blank"
                                rel="noopener noreferrer"
                                title="Official Website"
                                onClick={(e) => e.stopPropagation()}
                            >
                                <LanguageIcon fontSize="small"/>
                            </IconButton>
                        )}
                        {p.apiDoc && (
                            <IconButton
                                size="small"
                                href={p.apiDoc}
                                target="_blank"
                                rel="noopener noreferrer"
                                title="API Documentation"
                                onClick={(e) => e.stopPropagation()}
                            >
                                <DescriptionIcon fontSize="small"/>
                            </IconButton>
                        )}
                    </Stack>
                </Stack>
            </CardActionArea>
        </Card>
    );
};

const BrowseProviders: React.FC<BrowseProvidersProps> = ({onPick}) => {
    const {t} = useTranslation();
    const providers = useProviderTemplates();
    const [search, setSearch] = useState('');

    const filtered = useMemo(() => {
        const term = search.trim().toLowerCase();
        if (!term) return providers;
        return providers.filter(p => {
            const haystack = `${p.name} ${p.alias || ''}`.toLowerCase();
            return haystack.includes(term);
        });
    }, [providers, search]);

    const {globalProviders, chinaProviders} = useMemo(() => {
        const g: UniqueProvider[] = [];
        const c: UniqueProvider[] = [];
        for (const p of filtered) {
            if (p.region === 'cn') c.push(p);
            else g.push(p);
        }
        return {globalProviders: g, chinaProviders: c};
    }, [filtered]);

    const handlePick = (p: UniqueProvider) => {
        const apiStyle: 'openai' | 'anthropic' = p.supportsOpenAI ? 'openai' : 'anthropic';
        const apiBase = apiStyle === 'openai' ? p.baseUrlOpenAI || p.baseUrlAnthropic || '' : p.baseUrlAnthropic || p.baseUrlOpenAI || '';
        const protocols: ('openai' | 'anthropic')[] = [];
        if (p.supportsOpenAI) protocols.push('openai');
        if (p.supportsAnthropic) protocols.push('anthropic');
        onPick({
            name: p.alias || p.name,
            apiBase,
            apiStyle,
            token: '',
            enabled: true,
            protocols,
            providerBaseUrls: {
                openai: p.baseUrlOpenAI,
                anthropic: p.baseUrlAnthropic,
            },
        });
    };

    const isEmpty = filtered.length === 0;

    return (
        <Box>
            <Box sx={{mb: 1}}>
                <TextField
                    size="small"
                    fullWidth
                    placeholder={t('onboarding.browse.searchPlaceholder', {defaultValue: 'Search providers'})}
                    value={search}
                    onChange={e => setSearch(e.target.value)}
                    InputProps={{
                        startAdornment: (
                            <InputAdornment position="start">
                                <SearchIcon fontSize="small"/>
                            </InputAdornment>
                        ),
                        sx: {
                            borderRadius: 1,
                            bgcolor: 'background.paper',
                        },
                    }}
                />
            </Box>

            <SectionHeader
                icon={<AddIcon sx={{fontSize: 18}}/>}
                title={t('onboarding.browse.section.custom', {defaultValue: 'Custom'})}
                accent="custom"
            />
            <Box sx={gridSx}>
                <CustomProviderCard onPick={() => onPick(emptyForm())}/>
            </Box>

            {chinaProviders.length > 0 && (
                <>
                    <SectionHeader
                        icon={<LocationOnIcon sx={{fontSize: 18}}/>}
                        title={t('onboarding.browse.section.china', {defaultValue: 'China (Mainland)'})}
                        count={chinaProviders.length}
                        accent="cn"
                    />
                    <Box sx={gridSx}>
                        {chinaProviders.map(p => (
                            <ProviderCard key={p.id} provider={p} onPick={handlePick}/>
                        ))}
                    </Box>
                </>
            )}

            {globalProviders.length > 0 && (
                <>
                    <SectionHeader
                        icon={<PublicIcon sx={{fontSize: 18}}/>}
                        title={t('onboarding.browse.section.global', {defaultValue: 'Global'})}
                        count={globalProviders.length}
                        accent="global"
                    />
                    <Box sx={gridSx}>
                        {globalProviders.map(p => (
                            <ProviderCard key={p.id} provider={p} onPick={handlePick}/>
                        ))}
                    </Box>
                </>
            )}

            {isEmpty && (
                <Box sx={{py: 6, textAlign: 'center'}}>
                    <Typography variant="body2" color="text.secondary">
                        {t('onboarding.browse.empty', {defaultValue: 'No providers match your search.'})}
                    </Typography>
                </Box>
            )}
        </Box>
    );
};

export default BrowseProviders;

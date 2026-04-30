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
} from '@mui/material';
import SearchIcon from '@mui/icons-material/Search';
import LanguageIcon from '@mui/icons-material/Language';
import DescriptionIcon from '@mui/icons-material/Description';
import ProviderIcon from '@/components/ProviderIcon';
import {useProviderTemplates, type UniqueProvider} from '@/services/serviceProviders';
import type {EnhancedProviderFormData} from '@/components/ProviderFormDialog';

interface BrowseProvidersProps {
    onPick: (prefill: EnhancedProviderFormData) => void;
}

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

    return (
        <Box>
            <Box sx={{mb: 2}}>
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
                    }}
                />
            </Box>

            {filtered.length === 0 ? (
                <Box sx={{py: 6, textAlign: 'center'}}>
                    <Typography variant="body2" color="text.secondary">
                        {t('onboarding.browse.empty', {defaultValue: 'No providers match your filters.'})}
                    </Typography>
                </Box>
            ) : (
                <Box
                    sx={{
                        display: 'grid',
                        gap: 1.5,
                        gridTemplateColumns: {
                            xs: '1fr',
                            sm: 'repeat(2, 1fr)',
                            md: 'repeat(3, 1fr)',
                            xl: 'repeat(4, 1fr)',
                        },
                    }}
                >
                    {filtered.map(p => (
                        <Card key={p.id} variant="outlined" sx={{borderRadius: 2}}>
                            <CardActionArea onClick={() => handlePick(p)} sx={{p: 1.5}}>
                                <Stack direction="row" spacing={1.5} alignItems="center" justifyContent="space-between" width="100%">
                                    <Stack direction="row" spacing={1.5} alignItems="center">
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
                                                    <Chip
                                                        label="OpenAI"
                                                        size="small"
                                                        sx={{height: 18, fontSize: '0.65rem'}}
                                                    />
                                                )}
                                                {p.supportsAnthropic && (
                                                    <Chip
                                                        label="Anthropic"
                                                        size="small"
                                                        sx={{height: 18, fontSize: '0.65rem'}}
                                                    />
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
                                                <LanguageIcon fontSize="small" />
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
                                                <DescriptionIcon fontSize="small" />
                                            </IconButton>
                                        )}
                                    </Stack>
                                </Stack>
                            </CardActionArea>
                        </Card>
                    ))}
                </Box>
            )}
        </Box>
    );
};

export default BrowseProviders;

import { Box, Typography } from '@mui/material';
import UnifiedCard from './UnifiedCard';
import CardGrid, { CardGridItem } from './CardGrid';
import ProviderCard from './ProviderCard';

interface ProviderSelectionCardProps {
    providers: any[];
    defaults: any;
    providerModels: any;
    onSetDefault: (providerName: string) => Promise<void>;
    onFetchModels: (providerName: string) => Promise<void>;
}

const ProviderSelectionCard = ({
    providers,
    defaults,
    providerModels,
    onSetDefault,
    onFetchModels,
}: ProviderSelectionCardProps) => {
    return (
        <UnifiedCard
            title="Provider Selection"
            subtitle="Quick access to all configured providers"
            size="large"
        >
            <Box sx={{ maxHeight: 300, overflowY: 'auto' }}>
                {providers.length > 0 ? (
                    <CardGrid>
                        {providers.map((provider) => {
                            const isDefault = defaults.defaultProvider === provider.name;
                            return (
                                <CardGridItem xs={12} sm={6} key={provider.name}>
                                    <ProviderCard
                                        provider={provider}
                                        variant="simple"
                                        isDefault={isDefault}
                                        providerModels={providerModels}
                                        onSetDefault={onSetDefault}
                                        onFetchModels={onFetchModels}
                                    />
                                </CardGridItem>
                            );
                        })}
                    </CardGrid>
                ) : (
                    <Box textAlign="center" py={3}>
                        <Typography variant="body2" color="text.secondary">
                            No providers configured yet
                        </Typography>
                    </Box>
                )}
            </Box>
        </UnifiedCard>
    );
};

export default ProviderSelectionCard;

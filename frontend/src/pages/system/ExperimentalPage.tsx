import CardGrid from '@/components/CardGrid';
import { parseExperimentalFeature, sanitizeFeatureReturnPath } from '@/components/ExperimentalFeatureGate';
import GlobalExperimentalFeatures from '@/components/GlobalExperimentalFeatures';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router-dom';

// Card spans full width; only the content inside is capped so the feature
// list doesn't stretch uncomfortably wide on big screens.
const CONTENT_MAX_WIDTH = 720;

const ExperimentalPage = () => {
    const { t } = useTranslation();
    const [searchParams] = useSearchParams();
    const requestedFeature = parseExperimentalFeature(searchParams.get('feature'));
    const returnTo = sanitizeFeatureReturnPath(searchParams.get('returnTo'));

    return (
        <PageLayout loading={false}>
            <CardGrid>
                <UnifiedCard
                    contentMaxWidth={CONTENT_MAX_WIDTH}
                    title={t('system.experimentalFeatures.title')}
                    titleHeadingLevel={1}
                    size="full"
                >
                    <Stack spacing={1}>
                        <Typography
                            variant="body2"
                            sx={{
                                color: "text.secondary",
                                mb: 1
                            }}>
                            {t('system.experimentalFeatures.description')}
                        </Typography>
                        <GlobalExperimentalFeatures
                            requestedFeature={requestedFeature}
                            returnTo={returnTo}
                        />
                    </Stack>
                </UnifiedCard>
            </CardGrid>
        </PageLayout>
    );
};

export default ExperimentalPage;

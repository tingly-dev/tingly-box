import CardGrid from '@/components/CardGrid';
import GlobalExperimentalFeatures from '@/components/GlobalExperimentalFeatures';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';

const ExperimentalPage = () => {
    const { t } = useTranslation();

    return (
        <PageLayout loading={false}>
            <CardGrid>
                <UnifiedCard
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
                        <GlobalExperimentalFeatures />
                    </Stack>
                </UnifiedCard>
            </CardGrid>
        </PageLayout>
    );
};

export default ExperimentalPage;

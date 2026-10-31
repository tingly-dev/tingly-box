import { useTranslation } from 'react-i18next';
import PageHeader from '@/components/PageHeader';
import PageLayout from '@/components/PageLayout';

// Keep old bookmarks informative after removing the unused recording UI.
export default function RetiredRecordingsPage() {
    const { t } = useTranslation();
    return (
        <PageLayout loading={false}>
            <PageHeader
                title={t('prompt.recordingsRemoved.title')}
                subtitle={t('prompt.recordingsRemoved.description')}
            />
        </PageLayout>
    );
}

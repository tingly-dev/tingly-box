import { Box, Tab, Tabs } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router-dom';
import UnifiedCard from '@/components/UnifiedCard';
import ImagesPanel from './library/ImagesPanel';
import PiecesPanel from './library/PiecesPanel';
import { useLibrary } from './library/useLibrary';

type LibraryTab = 'prompts' | 'references';

// What the user keeps for image work beyond one session: prompt material
// (whole prompts, and the terms and phrases split out of them) and reference
// images. The playground saves into it and draws from it in place; this page
// is where it is browsed, split and tidied. See .design/image-library.md.
const ImageLibraryPage: React.FC = () => {
    const { t } = useTranslation();
    const { pieces, images, loaded } = useLibrary();
    const [searchParams, setSearchParams] = useSearchParams();
    const tab: LibraryTab = searchParams.get('tab') === 'references' ? 'references' : 'prompts';

    return (
        <UnifiedCard
            size="full"
            titleHeadingLevel={1}
            title={t('imageLibrary.title', { defaultValue: 'Image Library' })}
            subtitle={t('imageLibrary.subtitle', {
                defaultValue: 'Prompts, terms, phrases and reference images you keep. Stored in this browser.',
            })}
        >
            <Box sx={{ borderBottom: 1, borderColor: 'divider', mb: 2 }}>
                <Tabs
                    value={tab}
                    onChange={(_, value: LibraryTab) => setSearchParams(value === 'prompts' ? {} : { tab: value }, { replace: true })}
                >
                    <Tab
                        value="prompts"
                        label={t('imageLibrary.tabPrompts', { defaultValue: 'Prompts · {{count}}', count: pieces.length })}
                    />
                    <Tab
                        value="references"
                        label={t('imageLibrary.tabImages', { defaultValue: 'Reference images · {{count}}', count: images.length })}
                    />
                </Tabs>
            </Box>
            {tab === 'prompts'
                ? <PiecesPanel pieces={pieces} loaded={loaded} />
                : <ImagesPanel images={images} loaded={loaded} />}
        </UnifiedCard>
    );
};

export default ImageLibraryPage;

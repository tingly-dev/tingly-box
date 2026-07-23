import PlatformBotPage from './PlatformBotPage';
import { usePlatformGuide } from '@/constants/platformGuides';

// Every platform page (TelegramPage, FeishuPage, ...) is the same three
// lines: look up the localized guide for this platform and hand it to
// PlatformBotPage. Factored out so each page file is just its platform id.
export function createPlatformBotPage(platformId: string, fallbackName: string) {
    const Page = () => {
        const config = usePlatformGuide(platformId);
        return (
            <PlatformBotPage
                platformId={platformId}
                platformName={config?.name || fallbackName}
                platformGuide={config?.guide}
            />
        );
    };
    Page.displayName = `${fallbackName}Page`;
    return Page;
}

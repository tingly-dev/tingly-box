import type { ImageProfile } from '../profiles/imageProfileTypes';
import type { ImageAsset } from './assetTypes';

// Which profiles use an image. Identity is the image itself (its data URL):
// a reference picked from the library carries the asset's src unchanged.
export const profilesUsing = (asset: ImageAsset, profiles: ImageProfile[]): ImageProfile[] =>
    profiles.filter((profile) => profile.refs.some((ref) => ref.previewUrl === asset.src));

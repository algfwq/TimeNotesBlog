import type { AssetMeta, NotePage } from '../types';
import { assetDataUrl } from '../lib/files';

export function PageBackground({ page, assets }: { page: NotePage; assets: AssetMeta[] }) {
  const asset = assets.find((item) => item.id === page.backgroundAssetId);
  const src = assetDataUrl(asset);
  if (!src) {
    return null;
  }
  return (
    <img
      className="pointer-events-none absolute inset-0 h-full w-full"
      src={src}
      alt=""
      draggable={false}
      style={{
        objectFit: page.backgroundFit ?? 'cover',
        objectPosition: `${page.backgroundCropX ?? 50}% ${page.backgroundCropY ?? 50}%`,
      }}
    />
  );
}

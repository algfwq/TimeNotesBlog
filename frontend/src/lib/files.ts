import type { AssetMeta } from '../types';

export function assetDataUrl(asset?: Pick<AssetMeta, 'mimeType' | 'dataBase64' | 'dataUrl'> | null) {
  if (!asset) {
    return undefined;
  }
  if (asset.dataUrl) {
    return asset.dataUrl;
  }
  if (asset.dataBase64) {
    return `data:${asset.mimeType || 'application/octet-stream'};base64,${asset.dataBase64}`;
  }
  return undefined;
}

export function assetCoverDataUrl(asset?: Pick<AssetMeta, 'coverMimeType' | 'coverDataBase64' | 'coverDataUrl'> | null) {
  if (!asset) {
    return undefined;
  }
  if (asset.coverDataUrl) {
    return asset.coverDataUrl;
  }
  if (asset.coverDataBase64) {
    return `data:${asset.coverMimeType || 'image/jpeg'};base64,${asset.coverDataBase64}`;
  }
  return undefined;
}

export function assetPosterDataUrl(asset?: Pick<AssetMeta, 'posterDataBase64' | 'posterDataUrl'> | null) {
  if (!asset) {
    return undefined;
  }
  if (asset.posterDataUrl) {
    return asset.posterDataUrl;
  }
  if (asset.posterDataBase64) {
    return `data:image/jpeg;base64,${asset.posterDataBase64}`;
  }
  return undefined;
}

export function mergeAssetWithCache(asset?: AssetMeta, cached?: AssetMeta) {
  if (!asset) {
    return cached;
  }
  if (!cached) {
    return asset;
  }
  return {
    ...cached,
    ...asset,
    dataBase64: asset.dataBase64 ?? cached.dataBase64,
    dataUrl: asset.dataUrl ?? cached.dataUrl,
    coverDataBase64: asset.coverDataBase64 ?? cached.coverDataBase64,
    coverDataUrl: asset.coverDataUrl ?? cached.coverDataUrl,
  };
}

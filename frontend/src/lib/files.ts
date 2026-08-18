import type { AssetMeta } from '../types';

// 素材 dataUrl 来自上传的 .tnote 包，只接受 data: 协议，防止远程 URL 被自动请求成信标。
function safeInlineDataUrl(dataUrl: string | undefined) {
  return dataUrl && dataUrl.startsWith('data:') ? dataUrl : undefined;
}

export function assetDataUrl(asset?: Pick<AssetMeta, 'mimeType' | 'dataBase64' | 'dataUrl'> | null) {
  if (!asset) {
    return undefined;
  }
  const inline = safeInlineDataUrl(asset.dataUrl);
  if (inline) {
    return inline;
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
  const inline = safeInlineDataUrl(asset.coverDataUrl);
  if (inline) {
    return inline;
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
  const inline = safeInlineDataUrl(asset.posterDataUrl);
  if (inline) {
    return inline;
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

import type { AssetMeta } from '../types';

// 素材 URL 只接受本包解析出的 data: / blob:，拒绝 http(s) 以免文档里的远程地址被自动请求成信标。
// Blog 阅读器对音视频和 GLB 使用 Object URL（blob:），不能整文件转成 data URL。
function safeLocalResourceUrl(dataUrl: string | undefined) {
  if (!dataUrl) {
    return undefined;
  }
  if (dataUrl.startsWith('data:') || dataUrl.startsWith('blob:')) {
    return dataUrl;
  }
  return undefined;
}

export function assetDataUrl(asset?: Pick<AssetMeta, 'mimeType' | 'dataBase64' | 'dataUrl'> | null) {
  if (!asset) {
    return undefined;
  }
  const inline = safeLocalResourceUrl(asset.dataUrl);
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
  const inline = safeLocalResourceUrl(asset.coverDataUrl);
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
  const inline = safeLocalResourceUrl(asset.posterDataUrl);
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

import JSZip from 'jszip';
import type { AssetMeta, NoteDocument, NoteElement, NotePage, TemplateDef } from '../types';

export type LoadedTNote = {
  document: NoteDocument;
  objectUrls: string[];
};

function bytesToObjectUrl(mime: string, bytes: Uint8Array): string {
  const copy = new Uint8Array(bytes.byteLength);
  copy.set(bytes);
  const blob = new Blob([copy], {
    type: mime || 'application/octet-stream',
  });
  return URL.createObjectURL(blob);
}

function asString(value: unknown, fallback = '') {
  return typeof value === 'string' ? value : fallback;
}

function asNumber(value: unknown, fallback = 0) {
  const n = Number(value);
  return Number.isFinite(n) ? n : fallback;
}

function normalizePage(raw: Record<string, unknown>, index: number): NotePage {
  return {
    id: asString(raw.id, `page-${index + 1}`),
    title: asString(raw.title, `第 ${index + 1} 页`),
    width: asNumber(raw.width, 794),
    height: asNumber(raw.height, 1123),
    background: asString(raw.background, '#fffaf0'),
    backgroundAssetId: asString(raw.backgroundAssetId) || undefined,
    backgroundFit: raw.backgroundFit === 'contain' ? 'contain' : 'cover',
    backgroundCropX: raw.backgroundCropX === undefined ? 50 : asNumber(raw.backgroundCropX, 50),
    backgroundCropY: raw.backgroundCropY === undefined ? 50 : asNumber(raw.backgroundCropY, 50),
  };
}

function normalizeElement(raw: Record<string, unknown>, index: number): NoteElement {
  const pointsRaw = raw.points;
  let points: number[] | undefined;
  if (Array.isArray(pointsRaw)) {
    if (pointsRaw.length > 0 && typeof pointsRaw[0] === 'object' && pointsRaw[0] !== null) {
      points = [];
      for (const p of pointsRaw as Array<{ x?: number; y?: number }>) {
        points.push(asNumber(p.x, 0), asNumber(p.y, 0));
      }
    } else {
      points = pointsRaw.map((v) => asNumber(v, 0));
    }
  }
  return {
    id: asString(raw.id, `el-${index + 1}`),
    pageId: asString(raw.pageId),
    type: asString(raw.type, 'shape') as NoteElement['type'],
    x: asNumber(raw.x),
    y: asNumber(raw.y),
    width: asNumber(raw.width),
    height: asNumber(raw.height),
    rotation: asNumber(raw.rotation),
    zIndex: asNumber(raw.zIndex, index),
    content: raw.content === undefined ? undefined : String(raw.content),
    assetId: asString(raw.assetId) || undefined,
    style: (raw.style && typeof raw.style === 'object' ? raw.style : undefined) as NoteElement['style'],
    points,
  };
}

function normalizeAsset(raw: Record<string, unknown>): AssetMeta {
  return {
    id: asString(raw.id),
    name: asString(raw.name),
    hash: asString(raw.hash),
    mimeType: asString(raw.mimeType, 'application/octet-stream'),
    size: asNumber(raw.size),
    path: asString(raw.path),
    dataBase64: typeof raw.dataBase64 === 'string' ? raw.dataBase64 : undefined,
    dataUrl: typeof raw.dataUrl === 'string' ? raw.dataUrl : undefined,
    audioTitle: typeof raw.audioTitle === 'string' ? raw.audioTitle : undefined,
    audioArtist: typeof raw.audioArtist === 'string' ? raw.audioArtist : undefined,
    audioAlbum: typeof raw.audioAlbum === 'string' ? raw.audioAlbum : undefined,
    duration: raw.duration === undefined ? undefined : asNumber(raw.duration),
    coverMimeType: typeof raw.coverMimeType === 'string' ? raw.coverMimeType : undefined,
    coverDataBase64: typeof raw.coverDataBase64 === 'string' ? raw.coverDataBase64 : undefined,
    coverDataUrl: typeof raw.coverDataUrl === 'string' ? raw.coverDataUrl : undefined,
    videoWidth: raw.videoWidth === undefined ? undefined : asNumber(raw.videoWidth),
    videoHeight: raw.videoHeight === undefined ? undefined : asNumber(raw.videoHeight),
    posterDataBase64: typeof raw.posterDataBase64 === 'string' ? raw.posterDataBase64 : undefined,
    posterDataUrl: typeof raw.posterDataUrl === 'string' ? raw.posterDataUrl : undefined,
  };
}

async function hydrateGroup(zip: JSZip, items: AssetMeta[] | undefined, objectUrls: string[]): Promise<AssetMeta[]> {
  if (!items?.length) {
    return [];
  }
  const out: AssetMeta[] = [];
  for (const item of items) {
    const next = { ...item };
    if (item.path) {
      const file = zip.file(item.path);
      if (file) {
        const buf = await file.async('uint8array');
        const url = bytesToObjectUrl(item.mimeType || 'application/octet-stream', buf);
        objectUrls.push(url);
        next.dataUrl = url;
      }
    }
    if (next.coverDataBase64 && !next.coverDataUrl) {
      next.coverDataUrl = `data:${next.coverMimeType || 'image/jpeg'};base64,${next.coverDataBase64}`;
    }
    if (next.posterDataBase64 && !next.posterDataUrl) {
      next.posterDataUrl = `data:image/jpeg;base64,${next.posterDataBase64}`;
    }
    out.push(next);
  }
  return out;
}

export function releaseTNoteObjectUrls(urls: string[] | undefined) {
  for (const url of urls || []) {
    try {
      URL.revokeObjectURL(url);
    } catch {
      // ignore
    }
  }
}

export async function loadTNoteFromUrl(url: string): Promise<LoadedTNote> {
  const resp = await fetch(url);
  if (!resp.ok) {
    throw new Error(`download failed: ${resp.status}`);
  }
  const buf = await resp.arrayBuffer();
  const zip = await JSZip.loadAsync(buf);
  const docFile = zip.file('document.json');
  if (!docFile) {
    throw new Error('document.json missing');
  }
  const raw = JSON.parse(await docFile.async('string')) as Record<string, unknown>;
  const objectUrls: string[] = [];

  const pages = Array.isArray(raw.pages) ? raw.pages.map((p, i) => normalizePage((p || {}) as Record<string, unknown>, i)) : [];
  const elements = Array.isArray(raw.elements)
    ? raw.elements.map((e, i) => normalizeElement((e || {}) as Record<string, unknown>, i))
    : [];

  let assets = Array.isArray(raw.assets) ? raw.assets.map((a) => normalizeAsset((a || {}) as Record<string, unknown>)) : [];
  let stickers = Array.isArray(raw.stickers) ? raw.stickers.map((a) => normalizeAsset((a || {}) as Record<string, unknown>)) : [];
  let fonts = Array.isArray(raw.fonts) ? raw.fonts.map((a) => normalizeAsset((a || {}) as Record<string, unknown>)) : [];
  let audios = Array.isArray(raw.audios) ? raw.audios.map((a) => normalizeAsset((a || {}) as Record<string, unknown>)) : [];
  let videos = Array.isArray(raw.videos) ? raw.videos.map((a) => normalizeAsset((a || {}) as Record<string, unknown>)) : [];
  let models = Array.isArray(raw.models) ? raw.models.map((a) => normalizeAsset((a || {}) as Record<string, unknown>)) : [];

  assets = await hydrateGroup(zip, assets, objectUrls);
  stickers = await hydrateGroup(zip, stickers, objectUrls);
  fonts = await hydrateGroup(zip, fonts, objectUrls);
  audios = await hydrateGroup(zip, audios, objectUrls);
  videos = await hydrateGroup(zip, videos, objectUrls);
  models = await hydrateGroup(zip, models, objectUrls);

  const templates = Array.isArray(raw.templates) ? (raw.templates as TemplateDef[]) : [];
  const document: NoteDocument = {
    formatVersion: asNumber(raw.formatVersion, 1),
    title: asString(raw.title, '未命名手账'),
    createdAt: asString(raw.createdAt),
    updatedAt: asString(raw.updatedAt),
    pages,
    elements,
    assets,
    stickers,
    fonts,
    audios,
    videos,
    models,
    templates,
  };
  return { document, objectUrls };
}

export function assetMap(doc: NoteDocument): Record<string, AssetMeta> {
  const map: Record<string, AssetMeta> = {};
  for (const group of [doc.assets, doc.stickers, doc.fonts, doc.audios, doc.videos, doc.models]) {
    for (const a of group || []) {
      if (a.id) {
        map[a.id] = a;
      }
    }
  }
  return map;
}

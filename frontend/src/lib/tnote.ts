import JSZip from 'jszip';

export type AssetMeta = {
  id: string;
  name?: string;
  mimeType?: string;
  path?: string;
  dataBase64?: string;
  dataUrl?: string;
  [key: string]: unknown;
};

export type NoteDocument = {
  formatVersion?: number;
  title?: string;
  pages: Array<{
    id: string;
    name?: string;
    width: number;
    height: number;
    background?: string;
    backgroundAssetId?: string;
    [key: string]: unknown;
  }>;
  elements: Array<{
    id: string;
    pageId: string;
    type: string;
    x: number;
    y: number;
    width: number;
    height: number;
    rotation?: number;
    zIndex?: number;
    content?: string;
    style?: Record<string, unknown>;
    assetId?: string;
    points?: Array<{ x: number; y: number }>;
    [key: string]: unknown;
  }>;
  assets?: AssetMeta[];
  stickers?: AssetMeta[];
  fonts?: AssetMeta[];
  audios?: AssetMeta[];
  videos?: AssetMeta[];
  models?: AssetMeta[];
  [key: string]: unknown;
};

function toDataUrl(mime: string, bytes: Uint8Array): string {
  let binary = '';
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return `data:${mime || 'application/octet-stream'};base64,${btoa(binary)}`;
}

async function hydrateGroup(zip: JSZip, items: AssetMeta[] | undefined): Promise<AssetMeta[]> {
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
        next.dataUrl = toDataUrl(item.mimeType || 'application/octet-stream', buf);
      }
    }
    out.push(next);
  }
  return out;
}

export async function loadTNoteFromUrl(url: string): Promise<NoteDocument> {
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
  const doc = JSON.parse(await docFile.async('string')) as NoteDocument;
  doc.assets = await hydrateGroup(zip, doc.assets);
  doc.stickers = await hydrateGroup(zip, doc.stickers);
  doc.fonts = await hydrateGroup(zip, doc.fonts);
  doc.audios = await hydrateGroup(zip, doc.audios);
  doc.videos = await hydrateGroup(zip, doc.videos);
  doc.models = await hydrateGroup(zip, doc.models);
  return doc;
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

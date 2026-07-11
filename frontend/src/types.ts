export type ElementType = 'text' | 'code' | 'image' | 'sticker' | 'audio' | 'video' | 'model' | 'tape' | 'shape' | 'drawing';

export interface NotePage {
  id: string;
  title: string;
  width: number;
  height: number;
  background: string;
  backgroundAssetId?: string;
  backgroundFit?: 'cover' | 'contain';
  backgroundCropX?: number;
  backgroundCropY?: number;
}

export interface NoteElement {
  id: string;
  pageId: string;
  type: ElementType;
  x: number;
  y: number;
  width: number;
  height: number;
  rotation: number;
  zIndex: number;
  content?: string;
  assetId?: string;
  style?: Record<string, string | number | boolean>;
  points?: number[];
}

export interface AssetMeta {
  id: string;
  name: string;
  hash: string;
  mimeType: string;
  size: number;
  path: string;
  dataBase64?: string;
  dataUrl?: string;
  audioTitle?: string;
  audioArtist?: string;
  audioAlbum?: string;
  duration?: number;
  coverMimeType?: string;
  coverDataBase64?: string;
  coverDataUrl?: string;
  videoWidth?: number;
  videoHeight?: number;
  posterDataBase64?: string;
  posterDataUrl?: string;
}

export interface TemplateDef {
  id: string;
  name: string;
  description: string;
  page: NotePage;
  elements: NoteElement[];
}

export interface NoteDocument {
  formatVersion: number;
  title: string;
  createdAt: string;
  updatedAt: string;
  pages: NotePage[];
  elements: NoteElement[];
  assets: AssetMeta[];
  stickers: AssetMeta[];
  fonts: AssetMeta[];
  audios: AssetMeta[];
  videos: AssetMeta[];
  models: AssetMeta[];
  templates: TemplateDef[];
}

export type ResourceGroup = 'assets' | 'stickers' | 'fonts' | 'audios' | 'videos' | 'models';

export interface ResourceTransferProgress {
  key: string;
  group: ResourceGroup;
  assetId: string;
  name: string;
  receivedChunks: number;
  totalChunks: number;
  receivedBytes: number;
  totalBytes: number;
  progress: number;
}

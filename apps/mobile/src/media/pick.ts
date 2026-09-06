import * as DocumentPicker from 'expo-document-picker';
import { ImageManipulator, SaveFormat } from 'expo-image-manipulator';
import * as ImagePicker from 'expo-image-picker';

import type { MediaKind } from '@/api/types';

// Picking files, and getting them into a state worth uploading.
//
// A phone camera makes a 12-megapixel photo of four megabytes. Nobody
// looking at it in a chat bubble can tell it from one a quarter of the
// size, and on an Indian mobile connection the difference is the wait.
// So pictures are shrunk here, before anything is sent — which also drops
// the location and camera details the original carried, because the copy
// is re-encoded and those are not copied with it.

// The long side a picture is reduced to. Still more than any phone screen
// has, so a photo stays sharp when it is opened full-screen.
const MAX_PIXELS = 2048;

// JPEG quality for the shrunk copy: the usual point where a photograph
// stops getting visibly better and keeps getting bigger.
const QUALITY = 0.8;

// How many files one message may carry, matching the hub.
export const MAX_FILES = 10;

// A file this device has picked and is about to send.
export interface PickedFile {
  uri: string;
  name: string;
  mimeType: string;
  kind: MediaKind;
  byteSize: number;
  width?: number;
  height?: number;
  // For a recording: how long it runs and what it looked like, measured
  // while it was being made. See src/media/record.ts.
  durationMs?: number;
  waveform?: number[];
}

export function kindOf(mimeType: string): MediaKind {
  if (mimeType.startsWith('image/')) return 'image';
  if (mimeType.startsWith('video/')) return 'video';
  if (mimeType.startsWith('audio/')) return 'audio';
  return 'file';
}

// pickPhotos opens the phone's own gallery. Returns nothing when it was
// dismissed, which is not an error.
export async function pickPhotos(): Promise<PickedFile[]> {
  const result = await ImagePicker.launchImageLibraryAsync({
    mediaTypes: ['images', 'videos'],
    allowsMultipleSelection: true,
    selectionLimit: MAX_FILES,
    quality: QUALITY,
  });
  if (result.canceled) return [];
  return Promise.all(result.assets.map(fromAsset));
}

// takePhoto opens the camera. The permission is asked for at the moment it
// is needed, which is the only moment it makes sense to a person.
export async function takePhoto(): Promise<PickedFile[]> {
  const permission = await ImagePicker.requestCameraPermissionsAsync();
  if (!permission.granted) return [];
  const result = await ImagePicker.launchCameraAsync({ mediaTypes: ['images'], quality: QUALITY });
  if (result.canceled) return [];
  return Promise.all(result.assets.map(fromAsset));
}

// pickDocuments opens the system file picker: anything at all.
export async function pickDocuments(): Promise<PickedFile[]> {
  const result = await DocumentPicker.getDocumentAsync({
    multiple: true,
    copyToCacheDirectory: true,
  });
  if (result.canceled) return [];
  return result.assets.slice(0, MAX_FILES).map((a) => ({
    uri: a.uri,
    name: a.name || 'file',
    mimeType: a.mimeType || 'application/octet-stream',
    kind: kindOf(a.mimeType || ''),
    byteSize: a.size ?? 0,
  }));
}

async function fromAsset(asset: ImagePicker.ImagePickerAsset): Promise<PickedFile> {
  const mimeType = asset.mimeType || (asset.type === 'video' ? 'video/mp4' : 'image/jpeg');
  const file: PickedFile = {
    uri: asset.uri,
    name: asset.fileName || defaultName(mimeType),
    mimeType,
    kind: kindOf(mimeType),
    byteSize: asset.fileSize ?? 0,
    width: asset.width || undefined,
    height: asset.height || undefined,
  };
  return file.kind === 'image' ? shrink(file) : file;
}

// shrink reduces a picture that is larger than any screen will show it.
// A failure here is not worth refusing to send over: the original is
// perfectly good, only larger.
async function shrink(file: PickedFile): Promise<PickedFile> {
  const { width = 0, height = 0 } = file;
  if (width <= MAX_PIXELS && height <= MAX_PIXELS) return file;
  try {
    const size =
      width >= height
        ? { width: MAX_PIXELS, height: Math.round((height * MAX_PIXELS) / width) }
        : { height: MAX_PIXELS, width: Math.round((width * MAX_PIXELS) / height) };
    const image = await ImageManipulator.manipulate(file.uri).resize(size).renderAsync();
    const saved = await image.saveAsync({ compress: QUALITY, format: SaveFormat.JPEG });
    return {
      ...file,
      uri: saved.uri,
      mimeType: 'image/jpeg',
      name: file.name.replace(/\.(png|heic|heif|webp)$/i, '.jpg'),
      width: saved.width,
      height: saved.height,
      // The new size is unknown until it is read; the hub is the one that
      // counts bytes anyway.
      byteSize: 0,
    };
  } catch {
    return file;
  }
}

function defaultName(mimeType: string): string {
  const stamp = new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-');
  const ext = mimeType.split('/')[1]?.split('+')[0] ?? 'bin';
  return `cuckoo-${stamp}.${ext}`;
}

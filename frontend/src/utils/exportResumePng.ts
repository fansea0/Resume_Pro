import html2canvas from 'html2canvas';

const EXPORT_SCALE = 2;
const PHOTO_SELECTOR = 'img.template-resume-photo';

interface PhotoPatch {
  element: HTMLImageElement;
  src: string;
  width: string;
  height: string;
  objectFit: string;
}

function filenameFromTitle(title: string) {
  const normalized = title.trim().replace(/\s+/g, '-').replace(/[\\/:*?"<>|]/g, '');
  return `${normalized || '简历'}.png`;
}

function canvasToBlob(canvas: HTMLCanvasElement): Promise<Blob> {
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob) resolve(blob);
      else reject(new Error('无法生成 PNG 文件。'));
    }, 'image/png');
  });
}

/**
 * html2canvas 1.4.1 ignores `object-fit`, so a round cover-cropped avatar is
 * drawn stretched into its box. Crop the photo ourselves with the same cover
 * rule and hand html2canvas a data URL that already matches the box ratio.
 */
async function coverCropPhoto(photo: HTMLImageElement): Promise<string | null> {
  const rect = photo.getBoundingClientRect();
  const boxWidth = rect.width;
  const boxHeight = rect.height;
  const naturalWidth = photo.naturalWidth;
  const naturalHeight = photo.naturalHeight;
  if (!(boxWidth > 0) || !(boxHeight > 0) || !(naturalWidth > 0) || !(naturalHeight > 0)) return null;

  const coverScale = Math.max(boxWidth / naturalWidth, boxHeight / naturalHeight);
  const cropWidth = boxWidth / coverScale;
  const cropHeight = boxHeight / coverScale;
  const sourceX = (naturalWidth - cropWidth) / 2;
  const sourceY = (naturalHeight - cropHeight) / 2;

  const canvas = document.createElement('canvas');
  canvas.width = Math.max(1, Math.round(boxWidth * EXPORT_SCALE));
  canvas.height = Math.max(1, Math.round(boxHeight * EXPORT_SCALE));
  const context = canvas.getContext('2d');
  if (!context) return null;
  try {
    context.drawImage(
      photo,
      sourceX,
      sourceY,
      cropWidth,
      cropHeight,
      0,
      0,
      canvas.width,
      canvas.height,
    );
    return canvas.toDataURL('image/png');
  } catch {
    // A cross-origin photo without CORS headers taints the canvas; keep the original.
    return null;
  }
}

/** Swap every avatar photo for a pre-cropped copy and return the restore records. */
async function applyPhotoCoverCrop(root: HTMLElement): Promise<PhotoPatch[]> {
  const photos = Array.from(root.querySelectorAll<HTMLImageElement>(PHOTO_SELECTOR));
  const patches: PhotoPatch[] = [];
  for (const photo of photos) {
    if (!photo.complete) {
      try {
        await photo.decode();
      } catch {
        continue;
      }
    }
    const cropped = await coverCropPhoto(photo);
    if (!cropped) continue;
    const rect = photo.getBoundingClientRect();
    patches.push({
      element: photo,
      src: photo.getAttribute('src') ?? '',
      width: photo.style.width,
      height: photo.style.height,
      objectFit: photo.style.objectFit,
    });
    photo.src = cropped;
    photo.style.width = `${rect.width}px`;
    photo.style.height = `${rect.height}px`;
    // The crop already matches the box ratio, so a plain stretch is pixel-exact.
    photo.style.objectFit = 'fill';
    if (typeof photo.decode === 'function') {
      try {
        await photo.decode();
      } catch {
        // html2canvas falls back to the intrinsic bitmap when decoding races.
      }
    }
  }
  return patches;
}

function restorePhotos(patches: PhotoPatch[]) {
  for (const patch of patches) {
    if (patch.src) patch.element.setAttribute('src', patch.src);
    patch.element.style.width = patch.width;
    patch.element.style.height = patch.height;
    patch.element.style.objectFit = patch.objectFit;
  }
}

export async function exportResumePng(element: HTMLElement, title: string): Promise<void> {
  const patches = await applyPhotoCoverCrop(element);
  let canvas: HTMLCanvasElement;
  try {
    canvas = await html2canvas(element, {
      backgroundColor: '#ffffff',
      scale: EXPORT_SCALE,
      useCORS: true,
    });
  } finally {
    restorePhotos(patches);
  }
  const blob = await canvasToBlob(canvas);
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.download = filenameFromTitle(title);
  link.href = url;
  document.body.append(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}
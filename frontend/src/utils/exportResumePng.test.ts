import { beforeEach, describe, expect, it, vi } from 'vitest';
import html2canvas from 'html2canvas';
import { exportResumePng } from './exportResumePng';

vi.mock('html2canvas', () => ({ default: vi.fn() }));

const drawImage = vi.fn();

function stubCanvas() {
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({ drawImage } as unknown as CanvasRenderingContext2D);
  vi.spyOn(HTMLCanvasElement.prototype, 'toDataURL').mockReturnValue('data:image/png;base64,cropped');
}

function avatarImage() {
  const img = document.createElement('img');
  img.className = 'template-mint-canvas__avatar template-resume-photo';
  img.src = 'https://cdn.example.com/avatar.jpg';
  Object.defineProperty(img, 'complete', { value: true, configurable: true });
  Object.defineProperty(img, 'naturalWidth', { value: 800, configurable: true });
  Object.defineProperty(img, 'naturalHeight', { value: 400, configurable: true });
  img.getBoundingClientRect = () => ({ width: 64, height: 64, x: 0, y: 0, top: 0, left: 0, right: 64, bottom: 64, toJSON: () => ({}) }) as DOMRect;
  img.decode = vi.fn().mockResolvedValue(undefined);
  return img;
}

function exportableCanvas() {
  const canvas = document.createElement('canvas');
  canvas.toBlob = (callback) => callback(new Blob(['png'], { type: 'image/png' }));
  return canvas;
}

describe('exportResumePng', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    drawImage.mockClear();
    URL.createObjectURL = vi.fn(() => 'blob:resume') as typeof URL.createObjectURL;
    URL.revokeObjectURL = vi.fn();
  });

  it('renders at scale 2 and downloads a title-based PNG', async () => {
    const element = document.createElement('article');
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined);
    vi.mocked(html2canvas).mockResolvedValueOnce(exportableCanvas());

    await exportResumePng(element, '我的 产品 简历');

    expect(html2canvas).toHaveBeenCalledWith(element, expect.objectContaining({
      backgroundColor: '#ffffff',
      scale: 2,
      useCORS: true,
    }));
    expect(click).toHaveBeenCalledOnce();
    expect(document.querySelector('a[download="我的-产品-简历.png"]')).toBeNull();
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:resume');
  });

  it('cover-crops the avatar before capture so the face is never stretched', async () => {
    stubCanvas();
    const element = document.createElement('article');
    const img = avatarImage();
    element.append(img);
    let capturedSrc = '';
    let capturedWidth = '';
    vi.mocked(html2canvas).mockImplementation(async () => {
      capturedSrc = img.getAttribute('src') ?? '';
      capturedWidth = img.style.width;
      return exportableCanvas();
    });
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined);

    await exportResumePng(element, '简历');

    // A 800x400 photo in a 64x64 box must be cropped to the centred square
    // (200..600 on x) and drawn into a square canvas, not squashed.
    expect(drawImage).toHaveBeenCalledWith(img, 200, 0, 400, 400, 0, 0, 128, 128);
    expect(capturedSrc).toBe('data:image/png;base64,cropped');
    expect(capturedWidth).toBe('64px');
    // The live preview keeps its original photo and styles after exporting.
    expect(img.getAttribute('src')).toBe('https://cdn.example.com/avatar.jpg');
    expect(img.style.width).toBe('');
    expect(img.style.objectFit).toBe('');
  });

  it('leaves the photo untouched when the canvas is unavailable or tainted', async () => {
    const element = document.createElement('article');
    const img = avatarImage();
    element.append(img);
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
    let capturedSrc = '';
    vi.mocked(html2canvas).mockImplementation(async () => {
      capturedSrc = img.getAttribute('src') ?? '';
      return exportableCanvas();
    });
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined);

    await exportResumePng(element, '简历');

    expect(capturedSrc).toBe('https://cdn.example.com/avatar.jpg');
    expect(img.style.width).toBe('');
  });
});
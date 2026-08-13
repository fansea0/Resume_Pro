import { beforeEach, describe, expect, it, vi } from 'vitest';
import html2canvas from 'html2canvas';
import { exportResumePng } from './exportResumePng';

vi.mock('html2canvas', () => ({ default: vi.fn() }));

describe('exportResumePng', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    URL.createObjectURL = vi.fn(() => 'blob:resume') as typeof URL.createObjectURL;
    URL.revokeObjectURL = vi.fn();
  });

  it('renders at scale 2 and downloads a title-based PNG', async () => {
    const element = document.createElement('article');
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined);
    const canvas = document.createElement('canvas');
    canvas.toBlob = (callback) => callback(new Blob(['png'], { type: 'image/png' }));
    vi.mocked(html2canvas).mockResolvedValueOnce(canvas);

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
});

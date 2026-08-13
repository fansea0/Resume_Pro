import html2canvas from 'html2canvas';

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

export async function exportResumePng(element: HTMLElement, title: string): Promise<void> {
  const canvas = await html2canvas(element, {
    backgroundColor: '#ffffff',
    scale: 2,
    useCORS: true,
  });
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

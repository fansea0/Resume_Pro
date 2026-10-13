import { forwardRef } from 'react';
import { getTemplate } from '../templates/registry';
import type { ResumeDocument } from '../types/resume';
import type { LocateHandler } from './ResumeLocate';

interface ResumePreviewProps {
  document: ResumeDocument;
  /** 标题点击回调；缺省时模板只渲染标题文本（`?preview=` 封面图路径即依赖这一点）。 */
  onLocate?: LocateHandler;
}

export const ResumePreview = forwardRef<HTMLDivElement, ResumePreviewProps>(function ResumePreview({ document, onLocate }, ref) {
  const { Renderer } = getTemplate(document.templateId);

  return (
    <section className="resume-preview" aria-label="简历预览" data-testid="resume-preview">
      <div ref={ref}><Renderer document={document} onLocate={onLocate} /></div>
    </section>
  );
});

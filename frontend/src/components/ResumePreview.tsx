import { forwardRef } from 'react';
import { getTemplate } from '../templates/registry';
import type { ResumeDocument } from '../types/resume';

interface ResumePreviewProps {
  document: ResumeDocument;
}

export const ResumePreview = forwardRef<HTMLDivElement, ResumePreviewProps>(function ResumePreview({ document }, ref) {
  const { Renderer } = getTemplate(document.templateId);

  return (
    <section className="resume-preview" aria-label="简历预览" data-testid="resume-preview">
      <p className="resume-preview__template" data-testid="active-template">{document.templateId}</p>
      <div ref={ref}><Renderer document={document} /></div>
    </section>
  );
});

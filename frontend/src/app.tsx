import { useState } from 'react';
import { ResumeEditor } from './components/ResumeEditor';
import { ResumePreview } from './components/ResumePreview';
import { TemplatePicker } from './components/TemplatePicker';
import { createEmptyResumeDocument } from './types/resume';

export default function App() {
  const [resume, setResume] = useState(createEmptyResumeDocument);

  return (
    <main className="app-shell">
      <h1>简历生成器</h1>
      <p>为互联网求职打造专业、美观的简历。</p>
      <div className="resume-workspace">
        <aside className="resume-workspace__editor">
          <TemplatePicker onSelect={(templateId) => setResume((current) => ({ ...current, templateId }))} selectedTemplateId={resume.templateId} />
          <ResumeEditor document={resume} onChange={setResume} />
        </aside>
        <ResumePreview document={resume} />
      </div>
    </main>
  );
}

import { useState } from 'react';
import { ResumeEditor } from './components/ResumeEditor';
import { ResumeLibrary } from './components/ResumeLibrary';
import { ResumePreview } from './components/ResumePreview';
import { TemplatePicker } from './components/TemplatePicker';
import { createSampleResumeDocument } from './types/resume';

export default function App() {
  const [resume, setResume] = useState(createSampleResumeDocument);

  return (
    <main className="app-shell">
      <h1>简历生成器</h1>
      <p>为互联网求职打造专业、美观的简历。</p>
      <div className="resume-workspace">
        <aside className="resume-workspace__editor">
          <ResumeLibrary document={resume} onChange={setResume} onRestoreSample={() => setResume(createSampleResumeDocument())} onSelect={setResume} />
          <TemplatePicker onSelect={(templateId) => setResume((current) => ({ ...current, templateId }))} selectedTemplateId={resume.templateId} />
          <ResumeEditor document={resume} onChange={setResume} />
        </aside>
        <ResumePreview document={resume} />
      </div>
    </main>
  );
}

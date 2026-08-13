import { useState } from 'react';
import { ResumeEditor } from './components/ResumeEditor';
import { ResumeLibrary } from './components/ResumeLibrary';
import { ResumePreview } from './components/ResumePreview';
import { TemplateGallery } from './components/TemplateGallery';
import { templateRegistry } from './templates/registry';
import { createSampleResumeDocument } from './types/resume';

export default function App() {
  const [resume, setResume] = useState(createSampleResumeDocument);
  const [view, setView] = useState<'gallery' | 'editor'>('gallery');

  const selectTemplate = (templateId: typeof resume.templateId) => {
    setResume((current) => ({ ...current, templateId }));
    setView('editor');
  };
  const currentTemplate = templateRegistry.find((template) => template.id === resume.templateId);

  return (
    <main className="app-shell">
      <h1>简历生成器</h1>
      <p>为互联网求职打造专业、美观的简历。</p>
      {view === 'gallery' ? (
        <TemplateGallery onSelect={selectTemplate} />
      ) : (
        <div className="resume-workspace">
          <aside className="resume-workspace__editor">
            <div className="editor-navigation">
              <button onClick={() => setView('gallery')} type="button">返回模板库</button>
              <span>当前模板：{currentTemplate?.name ?? resume.templateId}</span>
            </div>
            <ResumeLibrary document={resume} onChange={setResume} onRestoreSample={() => setResume(createSampleResumeDocument())} onSelect={setResume} />
            <ResumeEditor document={resume} onChange={setResume} />
          </aside>
          <ResumePreview document={resume} />
        </div>
      )}
    </main>
  );
}

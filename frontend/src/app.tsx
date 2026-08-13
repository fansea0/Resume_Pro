import { createEmptyResumeDocument } from './types/resume';

export default function App() {
  const resume = createEmptyResumeDocument();

  return (
    <main className="app-shell">
      <h1>简历生成器</h1>
      <p>为互联网求职打造专业、美观的简历。</p>
      <p>当前模板：{resume.templateId}</p>
    </main>
  );
}

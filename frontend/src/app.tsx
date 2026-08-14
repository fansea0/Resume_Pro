import { useEffect, useRef, useState } from 'react';
import { getCurrentUser, logout, type AuthUser } from './api/auth';
import { onUnauthorized } from './api/client';
import { AuthScreen } from './components/AuthScreen';
import { ResumeEditor } from './components/ResumeEditor';
import { ResumeDrafts } from './components/ResumeDrafts';
import { ResumeLibrary } from './components/ResumeLibrary';
import { ResumePreview } from './components/ResumePreview';
import { TemplateGallery } from './components/TemplateGallery';
import { documentFromStored, type StoredResume } from './api/resumes';
import { templateRegistry } from './templates/registry';
import { createSampleResumeDocument } from './types/resume';

export default function App() {
  const [resume, setResume] = useState(createSampleResumeDocument);
  const [view, setView] = useState<'gallery' | 'drafts' | 'editor'>('gallery');
  const [authState, setAuthState] = useState<'loading' | 'authenticated' | 'unauthenticated'>('loading');
  const [user, setUser] = useState<AuthUser | null>(null);
  const previewRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let active = true;
    getCurrentUser()
      .then((currentUser) => {
        if (!active) return;
        setUser(currentUser);
        setAuthState('authenticated');
      })
      .catch(() => {
        if (active) setAuthState('unauthenticated');
      });
    const removeUnauthorizedListener = onUnauthorized(() => {
      if (!active) return;
      setUser(null);
      setAuthState('unauthenticated');
      setView('gallery');
    });
    return () => {
      active = false;
      removeUnauthorizedListener();
    };
  }, []);

  const selectTemplate = (templateId: typeof resume.templateId) => {
    setResume((current) => ({ ...current, templateId }));
    setView('editor');
  };

  const selectDraft = (draft: StoredResume) => {
    try {
      setResume(documentFromStored(draft));
      setView('editor');
    } catch {
      setView('drafts');
    }
  };
  const currentTemplate = templateRegistry.find((template) => template.id === resume.templateId);

  const handleAuthenticated = (nextUser: AuthUser) => {
    setUser(nextUser);
    setAuthState('authenticated');
    setView('gallery');
  };

  const handleLogout = async () => {
    try {
      await logout();
    } finally {
      setUser(null);
      setAuthState('unauthenticated');
      setView('gallery');
    }
  };

  if (authState === 'loading') {
    return <main className="app-shell"><p aria-live="polite">正在验证登录状态…</p></main>;
  }

  if (authState === 'unauthenticated') {
    return <main className="app-shell"><AuthScreen onAuthenticated={handleAuthenticated} /></main>;
  }

  return (
    <main className="app-shell">
      <header className="app-header">
        <div><h1>简历生成器</h1><p>为互联网求职打造专业、美观的简历。</p></div>
        <div className="app-header__actions">
          <nav aria-label="主要导航" className="app-navigation">
            <button aria-current={view === 'gallery' ? 'page' : undefined} onClick={() => setView('gallery')} type="button">模板主页</button>
            <button aria-current={view === 'drafts' ? 'page' : undefined} onClick={() => setView('drafts')} type="button">我的草稿</button>
          </nav>
          <div className="account-menu"><span className="account-menu__email">{user?.email}</span><button onClick={handleLogout} type="button">退出登录</button></div>
        </div>
      </header>
      {view === 'gallery' ? (
        <TemplateGallery onSelect={selectTemplate} />
      ) : view === 'drafts' ? (
        <ResumeDrafts onCreate={() => { setResume(createSampleResumeDocument()); setView('gallery'); }} onSelect={selectDraft} />
      ) : (
        <div className="resume-workspace">
          <aside className="resume-workspace__editor">
            <div className="editor-navigation">
              <div>
                <button onClick={() => setView('gallery')} type="button">返回模板主页</button>
                <button className="editor-navigation__secondary" onClick={() => setView('drafts')} type="button">我的草稿</button>
              </div>
              <span>当前模板：{currentTemplate?.name ?? resume.templateId}</span>
            </div>
            <ResumeLibrary document={resume} onChange={setResume} onRestoreSample={() => setResume(createSampleResumeDocument())} onSelect={setResume} previewRef={previewRef} />
            <ResumeEditor document={resume} onChange={setResume} />
          </aside>
          <ResumePreview document={resume} ref={previewRef} />
        </div>
      )}
    </main>
  );
}

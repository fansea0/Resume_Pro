import { useCallback, useEffect, useRef, useState } from 'react';
import { getCurrentUser, logout, type AuthUser } from './api/auth';
import { onUnauthorized } from './api/client';
import { AuthScreen } from './components/AuthScreen';
import { ResumeEditor } from './components/ResumeEditor';
import { ResumeDrafts } from './components/ResumeDrafts';
import { ResumeLibrary } from './components/ResumeLibrary';
import { ResumePreview } from './components/ResumePreview';
import { TemplateGallery } from './components/TemplateGallery';
import { TemplateAdmin } from './components/TemplateAdmin';
import { TargetJobsPage } from './pages/TargetJobsPage';
import { documentFromStored, type StoredResume } from './api/resumes';
import { getTemplateStatus } from './api/templates';
import { templateRegistry } from './templates/registry';
import { createSampleResumeDocument, type TemplateId } from './types/resume';

export default function App() {
  // Dev/preview mode: `?preview=<template-id>` renders a single template with
  // the fixed sample document so headless browsers can snapshot the cover PNG
  // for `frontend/public/template-previews/`. No auth, no network, no user data.
  // Expose the registry to headless preview scripts (see
  // `frontend/scripts/generate-template-previews.mjs`) so they can iterate ids
  // without rendering each template. Set once per page load.
  if (typeof window !== 'undefined') {
    (window as unknown as { __TEMPLATE_IDS__?: string[] }).__TEMPLATE_IDS__ = templateRegistry.map((t) => t.id);
  }

  if (typeof window !== 'undefined') {
    const params = new URLSearchParams(window.location.search);
    const previewId = params.get('preview') as TemplateId | null;
    if (previewId) {
      const template = templateRegistry.find((entry) => entry.id === previewId);
      if (template) {
        const sample = { ...createSampleResumeDocument(), templateId: previewId };
        const Renderer = template.Renderer;
        return (
          <main className="app-shell app-shell--preview">
            <Renderer document={sample} />
          </main>
        );
      }
    }
  }

  const [resume, setResume] = useState(createSampleResumeDocument);
  const [view, setView] = useState<'gallery' | 'drafts' | 'editor' | 'admin' | 'targetJobs'>(() => (
    typeof window !== 'undefined' && window.location.pathname === '/target-jobs' ? 'targetJobs' : 'gallery'
  ));
  const [selectedResumeId, setSelectedResumeId] = useState<number | null>(null);
  const [draftReadError, setDraftReadError] = useState(false);
  const [authState, setAuthState] = useState<'loading' | 'authenticated' | 'unauthenticated'>('loading');
  const [user, setUser] = useState<AuthUser | null>(null);
  const [disabledTemplateIds, setDisabledTemplateIds] = useState<typeof resume.templateId[]>([]);
  const previewRef = useRef<HTMLDivElement>(null);
  const isMounted = useRef(false);
  const templateStatusRequestVersion = useRef(0);

  const loadTemplateStatus = useCallback(async () => {
    const requestVersion = ++templateStatusRequestVersion.current;
    try {
      const status = await getTemplateStatus();
      if (isMounted.current && requestVersion === templateStatusRequestVersion.current) {
        setDisabledTemplateIds(status.disabledTemplateIds);
      }
      return isMounted.current && requestVersion === templateStatusRequestVersion.current;
    } catch {
      if (isMounted.current && requestVersion === templateStatusRequestVersion.current) {
        setDisabledTemplateIds([]);
      }
      return isMounted.current && requestVersion === templateStatusRequestVersion.current;
    }
  }, []);

  useEffect(() => {
    let active = true;
    isMounted.current = true;
    getCurrentUser()
      .then(async (currentUser) => {
        if (!active) return;
        if (!(await loadTemplateStatus()) || !active) return;
        setUser(currentUser);
        setAuthState('authenticated');
      })
      .catch(() => {
        if (active) setAuthState('unauthenticated');
      });
    const removeUnauthorizedListener = onUnauthorized(() => {
      if (!active) return;
      templateStatusRequestVersion.current += 1;
      setDisabledTemplateIds([]);
      setUser(null);
      setAuthState('unauthenticated');
      setView('gallery');
      setSelectedResumeId(null);
    });
    return () => {
      active = false;
      isMounted.current = false;
      templateStatusRequestVersion.current += 1;
      removeUnauthorizedListener();
    };
  }, [loadTemplateStatus]);

  useEffect(() => {
    const handlePopState = () => setView(window.location.pathname === '/target-jobs' ? 'targetJobs' : 'gallery');
    window.addEventListener('popstate', handlePopState);
    return () => window.removeEventListener('popstate', handlePopState);
  }, []);

  const navigateTo = (nextView: 'gallery' | 'targetJobs') => {
    const path = nextView === 'targetJobs' ? '/target-jobs' : '/';
    if (window.location.pathname !== path) window.history.pushState({}, '', path);
    setDraftReadError(false);
    setView(nextView);
  };

  const selectTemplate = (templateId: typeof resume.templateId) => {
    setResume((current) => ({ ...current, templateId }));
    setSelectedResumeId(null);
    setView('editor');
  };

  const selectDraft = (draft: StoredResume) => {
    try {
      setResume(documentFromStored(draft));
      setSelectedResumeId(draft.id);
      setDraftReadError(false);
      setView('editor');
    } catch {
      setDraftReadError(true);
      setView('drafts');
    }
  };
  const currentTemplate = templateRegistry.find((template) => template.id === resume.templateId);

  const handleAuthenticated = async (nextUser: AuthUser) => {
    if (!(await loadTemplateStatus()) || !isMounted.current) return;
    setUser(nextUser);
    setAuthState('authenticated');
    setView('gallery');
  };

  const handleLogout = async () => {
    try {
      await logout();
    } finally {
      templateStatusRequestVersion.current += 1;
      setDisabledTemplateIds([]);
      setUser(null);
      setAuthState('unauthenticated');
      setView('gallery');
      setSelectedResumeId(null);
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
        <div className="app-header__brand"><h1>简历生成器</h1><p>专业表达，从清晰开始。</p></div>
        <div className="app-header__actions">
          <nav aria-label="主要导航" className="app-navigation">
            <button aria-current={view === 'gallery' ? 'page' : undefined} onClick={() => navigateTo('gallery')} type="button">模板主页</button>
            <button aria-current={view === 'drafts' ? 'page' : undefined} onClick={() => setView('drafts')} type="button">我的草稿</button>
            <button aria-current={view === 'targetJobs' ? 'page' : undefined} onClick={() => navigateTo('targetJobs')} type="button">目标岗位</button>
            {user?.isAdmin && <button aria-current={view === 'admin' ? 'page' : undefined} onClick={() => setView('admin')} type="button">后台管理</button>}
          </nav>
          <div className="account-menu"><span className="account-menu__email">{user?.email}</span><button onClick={handleLogout} type="button">退出登录</button></div>
        </div>
      </header>
      {view === 'gallery' ? (
        <TemplateGallery disabledTemplateIds={disabledTemplateIds} onSelect={selectTemplate} />
      ) : view === 'targetJobs' ? (
        <TargetJobsPage />
      ) : view === 'drafts' ? (
        <>
          {draftReadError && <p className="resume-library__error" role="alert">无法读取该简历内容。</p>}
          <ResumeDrafts onCreate={() => { setResume(createSampleResumeDocument()); setSelectedResumeId(null); setDraftReadError(false); setView('gallery'); }} onSelect={selectDraft} />
        </>
      ) : view === 'admin' && user?.isAdmin ? (
        <TemplateAdmin onDisabledTemplateIdsChange={setDisabledTemplateIds} />
      ) : (
        <div className="resume-workspace">
          <aside className="resume-workspace__editor">
            <div className="editor-navigation">
              <div>
                <button onClick={() => { setDraftReadError(false); setView('gallery'); }} type="button">返回模板主页</button>
                <button className="editor-navigation__secondary" onClick={() => setView('drafts')} type="button">我的草稿</button>
              </div>
              <span>当前模板：{currentTemplate?.name ?? resume.templateId}</span>
            </div>
            <ResumeLibrary document={resume} selectedId={selectedResumeId} onSelectedIdChange={setSelectedResumeId} onChange={setResume} onRestoreSample={() => setResume(createSampleResumeDocument())} onSelect={setResume} previewRef={previewRef} />
            <ResumeEditor document={resume} resumeId={selectedResumeId ?? undefined} onChange={setResume} />
          </aside>
          <ResumePreview document={resume} ref={previewRef} />
        </div>
      )}
    </main>
  );
}

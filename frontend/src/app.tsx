import { useCallback, useEffect, useRef, useState } from 'react';
import { getCurrentUser, logout, type AuthUser } from './api/auth';
import { onUnauthorized } from './api/client';
import { AuthScreen } from './components/AuthScreen';
import { ResumeEditor } from './components/ResumeEditor';
import { ResumeDrafts } from './components/ResumeDrafts';
import { ResumeLibrary } from './components/ResumeLibrary';
import { ResumePreview } from './components/ResumePreview';
import type { LocateRequest, LocateTarget } from './components/ResumeLocate';
import { TemplateGallery } from './components/TemplateGallery';
import { TemplateAdmin } from './components/TemplateAdmin';
import { TargetJobsPage } from './pages/TargetJobsPage';
import type { ResumeAutosaveHandle } from './hooks/useResumeAutosave';
import { documentFromStored, type StoredResume } from './api/resumes';
import { getTemplateStatus } from './api/templates';
import { templateRegistry } from './templates/registry';
import { createSampleResumeDocument, type TemplateId } from './types/resume';

type AppView = 'gallery' | 'drafts' | 'editor' | 'admin' | 'targetJobs';

function viewFromHistory(pathname: string, state: unknown): AppView {
  if (pathname === '/target-jobs') return 'targetJobs';
  if (state && typeof state === 'object' && 'appView' in state) {
    const candidate = (state as { appView?: unknown }).appView;
    if (candidate === 'gallery' || candidate === 'drafts' || candidate === 'editor' || candidate === 'admin') {
      return candidate;
    }
  }
  return 'gallery';
}

function pathForView(view: AppView) {
  return view === 'targetJobs' ? '/target-jobs' : '/';
}

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
  const [view, setView] = useState<AppView>(() => (
    typeof window !== 'undefined' ? viewFromHistory(window.location.pathname, window.history.state) : 'gallery'
  ));
  const [selectedResumeId, setSelectedResumeId] = useState<number | null>(null);
  const [draftReadError, setDraftReadError] = useState(false);
  const [authState, setAuthState] = useState<'loading' | 'authenticated' | 'unauthenticated'>('loading');
  const [user, setUser] = useState<AuthUser | null>(null);
  const [disabledTemplateIds, setDisabledTemplateIds] = useState<typeof resume.templateId[]>([]);
  const [targetJobsModalOpen, setTargetJobsModalOpen] = useState(false);
  const [templateReplacementMode, setTemplateReplacementMode] = useState(false);
  // 定位请求存在 state 里：无关重渲染拿到的是同一个对象引用，useLocateTarget 的 effect 不会重放。
  // 若改成每次渲染新建对象，用户每敲一个字编辑器都会被拽回被定位的区块。
  const [locateRequest, setLocateRequest] = useState<LocateRequest>();
  const locateNonce = useRef(0);
  const previewRef = useRef<HTMLDivElement>(null);
  const resumeAutosaveRef = useRef<ResumeAutosaveHandle | null>(null);
  const isMounted = useRef(false);
  const sessionEpoch = useRef(0);
  const templateStatusRequestVersion = useRef(0);

  // nonce 自增：连点同一目标时对象内容相同，靠引用变化让 effect 重放。
  const handleLocate = useCallback((target: LocateTarget) => {
    setLocateRequest({ ...target, nonce: ++locateNonce.current });
  }, []);

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

  const resetTenantState = useCallback(() => {
    sessionEpoch.current += 1;
    const nextView: AppView = window.location.pathname === '/target-jobs' ? 'targetJobs' : 'gallery';
    setResume(createSampleResumeDocument());
    setSelectedResumeId(null);
    setDraftReadError(false);
    setTargetJobsModalOpen(false);
    setView(nextView);
    window.history.replaceState({ appView: nextView }, '', pathForView(nextView));
  }, []);

  useEffect(() => {
    let active = true;
    isMounted.current = true;
    getCurrentUser()
      .then(async (currentUser) => {
        if (!active) return;
        if (!(await loadTemplateStatus()) || !active) return;
        resetTenantState();
        setUser(currentUser);
        setAuthState('authenticated');
      })
      .catch(() => {
        if (active) {
          resetTenantState();
          setAuthState('unauthenticated');
        }
      });
    const removeUnauthorizedListener = onUnauthorized(() => {
      if (!active) return;
      templateStatusRequestVersion.current += 1;
      setDisabledTemplateIds([]);
      setUser(null);
      setAuthState('unauthenticated');
      resetTenantState();
    });
    return () => {
      active = false;
      isMounted.current = false;
      templateStatusRequestVersion.current += 1;
      removeUnauthorizedListener();
    };
  }, [loadTemplateStatus, resetTenantState]);

  useEffect(() => {
    const handlePopState = (event: PopStateEvent) => {
      setDraftReadError(false);
      setView(viewFromHistory(window.location.pathname, event.state));
    };
    window.addEventListener('popstate', handlePopState);
    return () => window.removeEventListener('popstate', handlePopState);
  }, []);

  const navigateToView = (nextView: AppView) => {
    const path = pathForView(nextView);
    if (window.location.pathname !== path || view !== nextView) {
      window.history.pushState({ appView: nextView }, '', path);
    }
    setView(nextView);
  };

  const selectTemplate = (templateId: typeof resume.templateId) => {
    if (templateReplacementMode) {
      setResume((current) => ({ ...current, templateId }));
      setTemplateReplacementMode(false);
      navigateToView('editor');
      return;
    }
    setResume((current) => ({ ...current, templateId }));
    setSelectedResumeId(null);
    navigateToView('editor');
  };

  const selectDraft = (draft: StoredResume) => {
    try {
      setResume(documentFromStored(draft));
      setSelectedResumeId(draft.id);
      setDraftReadError(false);
      navigateToView('editor');
    } catch {
      setDraftReadError(true);
      navigateToView('drafts');
    }
  };
  const currentTemplate = templateRegistry.find((template) => template.id === resume.templateId);
  const renderedSessionEpoch = sessionEpoch.current;
  const changeResumeForSession = (nextResume: typeof resume) => {
    if (sessionEpoch.current === renderedSessionEpoch) setResume(nextResume);
  };
  const changeSelectedResumeForSession = (id: number | null) => {
    if (sessionEpoch.current === renderedSessionEpoch) setSelectedResumeId(id);
  };

  const handleAuthenticated = async (nextUser: AuthUser) => {
    if (!(await loadTemplateStatus()) || !isMounted.current) return;
    resetTenantState();
    setUser(nextUser);
    setAuthState('authenticated');
  };

  const handleLogout = async () => {
    try {
      await logout();
    } finally {
      templateStatusRequestVersion.current += 1;
      setDisabledTemplateIds([]);
      setUser(null);
      setAuthState('unauthenticated');
      resetTenantState();
    }
  };

  if (authState === 'loading') {
    return <main className="app-shell"><p aria-live="polite">正在验证登录状态…</p></main>;
  }

  if (authState === 'unauthenticated') {
    return <main className="app-shell"><AuthScreen onAuthenticated={handleAuthenticated} /></main>;
  }

  return (
    <main {...(targetJobsModalOpen ? { 'aria-hidden': true, inert: '' } : {})} className="app-shell">
      <header className="app-header">
        <div className="app-header__brand"><h1>简历生成器</h1><p>专业表达，从清晰开始。</p></div>
        <div className="app-header__actions">
          <nav aria-label="主要导航" className="app-navigation">
            <button aria-current={view === 'gallery' ? 'page' : undefined} onClick={() => { setDraftReadError(false); navigateToView('gallery'); }} type="button">模板主页</button>
            <button aria-current={view === 'drafts' ? 'page' : undefined} onClick={() => { setDraftReadError(false); navigateToView('drafts'); }} type="button">我的草稿</button>
            <button aria-current={view === 'targetJobs' ? 'page' : undefined} onClick={() => { setDraftReadError(false); navigateToView('targetJobs'); }} type="button">目标岗位</button>
            {user?.isAdmin && <button aria-current={view === 'admin' ? 'page' : undefined} onClick={() => { setDraftReadError(false); navigateToView('admin'); }} type="button">后台管理</button>}
          </nav>
          <div className="account-menu"><span className="account-menu__email">{user?.email}</span><button onClick={handleLogout} type="button">退出登录</button></div>
        </div>
      </header>
      {view === 'gallery' ? (
        <TemplateGallery currentTemplateId={resume.templateId} disabledTemplateIds={disabledTemplateIds} onCancelReplacement={() => { setTemplateReplacementMode(false); navigateToView('editor'); }} onSelect={selectTemplate} replacementMode={templateReplacementMode} />
      ) : view === 'targetJobs' ? (
        <TargetJobsPage onModalOpenChange={setTargetJobsModalOpen} />
      ) : view === 'drafts' ? (
        <>
          {draftReadError && <p className="resume-library__error" role="alert">无法读取该简历内容。</p>}
          <ResumeDrafts onCreate={() => { setResume(createSampleResumeDocument()); setSelectedResumeId(null); setDraftReadError(false); navigateToView('gallery'); }} onSelect={selectDraft} />
        </>
      ) : view === 'admin' && user?.isAdmin ? (
        <TemplateAdmin onDisabledTemplateIdsChange={setDisabledTemplateIds} />
      ) : (
        <div className="resume-workspace" key={renderedSessionEpoch}>
          <aside className="resume-workspace__editor">
            <div className="editor-navigation">
              <div>
                <button onClick={() => { setDraftReadError(false); navigateToView('gallery'); }} type="button">返回模板主页</button>
                <button className="editor-navigation__secondary" onClick={() => { setTemplateReplacementMode(true); navigateToView('gallery'); }} type="button">更换模板</button>
                <button className="editor-navigation__secondary" onClick={() => { setDraftReadError(false); navigateToView('drafts'); }} type="button">我的草稿</button>
              </div>
              <span>当前模板：{currentTemplate?.name ?? resume.templateId}</span>
            </div>
            <ResumeLibrary autosaveHandleRef={resumeAutosaveRef} document={resume} selectedId={selectedResumeId} onSelectedIdChange={changeSelectedResumeForSession} onChange={changeResumeForSession} onRestoreSample={() => changeResumeForSession(createSampleResumeDocument())} onSelect={changeResumeForSession} previewRef={previewRef} />
            <ResumeEditor document={resume} locateRequest={locateRequest} resumeId={selectedResumeId ?? undefined} onBeforeAiRewrite={(snapshot) => resumeAutosaveRef.current?.flush(snapshot) ?? Promise.resolve(false)} onChange={changeResumeForSession} />
          </aside>
          <ResumePreview document={resume} onLocate={handleLocate} ref={previewRef} />
        </div>
      )}
    </main>
  );
}

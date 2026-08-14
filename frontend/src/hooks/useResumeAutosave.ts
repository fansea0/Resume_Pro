import { useCallback, useEffect, useRef, useState } from 'react';
import { createResume, updateResume, type StoredResume } from '../api/resumes';
import type { ResumeDocument } from '../types/resume';

export type ResumeAutosaveStatus = 'idle' | 'saving' | 'saved' | 'error';

interface UseResumeAutosaveOptions {
  document: ResumeDocument;
  selectedId: number | null;
  onSaved: (id: number, resume: StoredResume) => void;
  enabled?: boolean;
}

export function useResumeAutosave({ document, selectedId, onSaved, enabled = true }: UseResumeAutosaveOptions) {
  const [status, setStatus] = useState<ResumeAutosaveStatus>('idle');
  const [error, setError] = useState<string | null>(null);
  const mounted = useRef(true);
  const documentRef = useRef(document);
  const idRef = useRef<number | null>(selectedId);
  const version = useRef(0);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const inFlight = useRef(false);
  const queuedAfterFlight = useRef(false);
  const queuedImmediately = useRef(false);
  const onSavedRef = useRef(onSaved);

  documentRef.current = document;
  onSavedRef.current = onSaved;

  useEffect(() => { idRef.current = selectedId; }, [selectedId]);

  const save = useCallback(async (immediate = false, forceNew = false) => {
    if (!enabled) return;
    if (inFlight.current) {
      queuedAfterFlight.current = true;
      queuedImmediately.current ||= immediate;
      return;
    }
    inFlight.current = true;
    const requestVersion = version.current;
    const requestDocument = documentRef.current;
    const currentId = forceNew ? null : idRef.current;
    if (mounted.current) { setStatus('saving'); setError(null); }
    try {
      const saved = currentId === null
        ? await createResume(requestDocument)
        : await updateResume(currentId, requestDocument);
      if (currentId === null) idRef.current = saved.id;
      if (mounted.current && requestVersion === version.current) {
        onSavedRef.current(saved.id, saved);
        setStatus('saved');
      }
    } catch {
      if (mounted.current && requestVersion === version.current) {
        setStatus('error');
        setError('保存失败，请稍后重试。');
      }
    } finally {
      inFlight.current = false;
      if (queuedAfterFlight.current && mounted.current) {
        const runImmediately = queuedImmediately.current;
        queuedAfterFlight.current = false;
        queuedImmediately.current = false;
        if (runImmediately) void save(true);
      }
    }
  }, [enabled]);

  useEffect(() => {
    version.current += 1;
    if (timer.current) clearTimeout(timer.current);
    if (!enabled) return;
    timer.current = setTimeout(() => { void save(); }, 800);
    return () => {
      if (timer.current) clearTimeout(timer.current);
      timer.current = null;
    };
  }, [document, enabled, save]);

  useEffect(() => () => {
    mounted.current = false;
    if (timer.current) clearTimeout(timer.current);
  }, []);

  const saveNow = useCallback(async ({ newDraft = false }: { newDraft?: boolean } = {}) => {
    if (timer.current) { clearTimeout(timer.current); timer.current = null; }
    if (newDraft) idRef.current = null;
    await save(true, newDraft);
  }, [save]);

  return { status, error, saveNow };
}

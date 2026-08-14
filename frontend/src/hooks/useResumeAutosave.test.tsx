import { act, renderHook } from '@testing-library/react';
import { StrictMode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { createResume, updateResume, type StoredResume } from '../api/resumes';
import { useResumeAutosave } from './useResumeAutosave';
import { createEmptyResumeDocument } from '../types/resume';

vi.mock('../api/resumes', () => ({ createResume: vi.fn(), updateResume: vi.fn() }));

const stored = (id: number, title: string): StoredResume => ({ id, title, templateId: 'minimal', content: '{}', createdAt: '', updatedAt: '' });

describe('useResumeAutosave', () => {
  afterEach(() => { vi.useRealTimers(); vi.resetAllMocks(); });

  it('creates after 800ms, retains its id, then updates that same draft', async () => {
    vi.useFakeTimers();
    const onSaved = vi.fn();
    const first = createEmptyResumeDocument();
    vi.mocked(createResume).mockResolvedValueOnce(stored(11, first.title));
    vi.mocked(updateResume).mockResolvedValueOnce(stored(11, '第二版'));
    const { rerender, result } = renderHook(({ document, selectedId }) => useResumeAutosave({ document, selectedId, onSaved }), { initialProps: { document: first, selectedId: null as number | null } });

    await act(async () => { await vi.advanceTimersByTimeAsync(800); });
    expect(createResume).toHaveBeenCalledWith(first);
    expect(onSaved).toHaveBeenCalledWith(11, expect.objectContaining({ id: 11 }));

    const second = { ...first, title: '第二版' };
    rerender({ document: second, selectedId: 11 });
    await act(async () => { await vi.advanceTimersByTimeAsync(800); });
    expect(updateResume).toHaveBeenCalledWith(11, second);
    expect(result.current.status).toBe('saved');
  });

  it('saves immediately on demand and reports failure without changing the document', async () => {
    vi.useFakeTimers();
    const document = createEmptyResumeDocument();
    vi.mocked(createResume).mockRejectedValueOnce(new Error('offline'));
    const { result } = renderHook(() => useResumeAutosave({ document, selectedId: null, onSaved: vi.fn() }));

    await act(async () => { await result.current.saveNow(); });
    expect(createResume).toHaveBeenCalledWith(document);
    expect(result.current.status).toBe('error');
    expect(result.current.error).toBe('保存失败，请稍后重试。');
  });

  it('reports dirty while waiting for the debounce', () => {
    vi.useFakeTimers();
    const { result } = renderHook(() => useResumeAutosave({ document: createEmptyResumeDocument(), selectedId: null, onSaved: vi.fn() }));
    expect(result.current.status).toBe('dirty');
  });

  it('does not save after unmount and ignores an old request completion after a newer edit', async () => {
    vi.useFakeTimers();
    const first = createEmptyResumeDocument();
    let resolveFirst!: (value: StoredResume) => void;
    vi.mocked(createResume).mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve; }));
    const { rerender, result, unmount } = renderHook(({ document }) => useResumeAutosave({ document, selectedId: null, onSaved: vi.fn() }), { initialProps: { document: first } });

    await act(async () => { await vi.advanceTimersByTimeAsync(800); });
    rerender({ document: { ...first, title: '更新中' } });
    await act(async () => { resolveFirst(stored(12, first.title)); });
    expect(result.current.status).not.toBe('saved');

    unmount();
    await act(async () => { await vi.advanceTimersByTimeAsync(800); });
    expect(createResume).toHaveBeenCalledTimes(1);
  });

  it('persists edit B when its debounce completes while save A is still in flight', async () => {
    vi.useFakeTimers();
    const first = createEmptyResumeDocument();
    let resolveFirst!: (value: StoredResume) => void;
    vi.mocked(createResume).mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve; }));
    vi.mocked(updateResume).mockResolvedValueOnce(stored(21, 'B'));
    const { rerender } = renderHook(({ document, selectedId }) => useResumeAutosave({ document, selectedId, onSaved: vi.fn() }), { initialProps: { document: first, selectedId: null as number | null } });

    await act(async () => { await vi.advanceTimersByTimeAsync(800); });
    const second = { ...first, title: 'B' };
    rerender({ document: second, selectedId: null });
    await act(async () => { await vi.advanceTimersByTimeAsync(800); });
    await act(async () => { resolveFirst(stored(21, first.title)); await vi.runAllTimersAsync(); });

    expect(updateResume).toHaveBeenCalledWith(21, second);
  });

  it('invalidates pending and in-flight autosaves when a selected draft is deleted', async () => {
    vi.useFakeTimers();
    const document = createEmptyResumeDocument();
    let resolveUpdate!: (value: StoredResume) => void;
    vi.mocked(updateResume).mockImplementationOnce(() => new Promise((resolve) => { resolveUpdate = resolve; }));
    const onSaved = vi.fn();
    const { result } = renderHook(() => useResumeAutosave({ document, selectedId: 31, onSaved }));

    await act(async () => { await vi.advanceTimersByTimeAsync(800); });
    act(() => result.current.cancel());
    await act(async () => { resolveUpdate(stored(31, document.title)); await vi.advanceTimersByTimeAsync(800); });

    expect(onSaved).not.toHaveBeenCalled();
    expect(createResume).not.toHaveBeenCalled();
  });

  it('does not let a stale create change the selected draft after a switch', async () => {
    vi.useFakeTimers();
    const first = createEmptyResumeDocument();
    let resolveCreate!: (value: StoredResume) => void;
    vi.mocked(createResume).mockImplementationOnce(() => new Promise((resolve) => { resolveCreate = resolve; }));
    vi.mocked(updateResume).mockResolvedValueOnce(stored(42, '新草稿'));
    const onSaved = vi.fn();
    const { rerender, result } = renderHook(({ document, selectedId }) => useResumeAutosave({ document, selectedId, onSaved }), { initialProps: { document: first, selectedId: null as number | null } });

    await act(async () => { await vi.advanceTimersByTimeAsync(800); });
    act(() => result.current.cancel());
    const next = { ...first, title: '新草稿' };
    rerender({ document: next, selectedId: 42 });
    await act(async () => { resolveCreate(stored(99, first.title)); await vi.advanceTimersByTimeAsync(800); });

    expect(onSaved).toHaveBeenLastCalledWith(42, expect.objectContaining({ id: 42 }));
    expect(updateResume).toHaveBeenCalledWith(42, next);
  });

  it('remains active after the StrictMode effect rehearsal', async () => {
    vi.useFakeTimers();
    const document = createEmptyResumeDocument();
    vi.mocked(createResume).mockResolvedValueOnce(stored(61, document.title));
    renderHook(() => useResumeAutosave({ document, selectedId: null, onSaved: vi.fn() }), { wrapper: StrictMode });

    await act(async () => { await vi.advanceTimersByTimeAsync(800); });
    expect(createResume).toHaveBeenCalledWith(document);
  });
});

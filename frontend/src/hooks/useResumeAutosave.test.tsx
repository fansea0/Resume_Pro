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
    const firstEdited = { ...first, title: '第一版' };
    rerender({ document: firstEdited, selectedId: null });

    await act(async () => { await vi.advanceTimersByTimeAsync(800); });
    expect(createResume).toHaveBeenCalledWith(firstEdited);
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

  it('does not save merely by opening a new template editor', async () => {
    vi.useFakeTimers();
    const document = createEmptyResumeDocument();
    const { result } = renderHook(() => useResumeAutosave({ document, selectedId: null, onSaved: vi.fn() }));
    await act(async () => { await vi.advanceTimersByTimeAsync(801); });
    expect(createResume).not.toHaveBeenCalled();
    expect(result.current.status).toBe('idle');
  });

  it('does not update merely by opening an existing draft', async () => {
    vi.useFakeTimers();
    const document = createEmptyResumeDocument();
    renderHook(() => useResumeAutosave({ document, selectedId: 17, onSaved: vi.fn() }));
    await act(async () => { await vi.advanceTimersByTimeAsync(801); });
    expect(updateResume).not.toHaveBeenCalled();
  });

  it('reports dirty while waiting for the debounce after an edit', () => {
    vi.useFakeTimers();
    const first = createEmptyResumeDocument();
    const { rerender, result } = renderHook(({ document }) => useResumeAutosave({ document, selectedId: null, onSaved: vi.fn() }), { initialProps: { document: first } });
    rerender({ document: { ...first, title: '编辑后' } });
    expect(result.current.status).toBe('dirty');
  });

  it('does not save after unmount and ignores an old request completion after a newer edit', async () => {
    vi.useFakeTimers();
    const first = createEmptyResumeDocument();
    let resolveFirst!: (value: StoredResume) => void;
    vi.mocked(createResume).mockImplementationOnce(() => new Promise((resolve) => { resolveFirst = resolve; }));
    const { rerender, result, unmount } = renderHook(({ document }) => useResumeAutosave({ document, selectedId: null, onSaved: vi.fn() }), { initialProps: { document: first } });
    rerender({ document: { ...first, title: '首次编辑' } });
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
    rerender({ document: { ...first, title: 'A' }, selectedId: null });
    await act(async () => { await vi.advanceTimersByTimeAsync(800); });
    const second = { ...first, title: 'B' };
    rerender({ document: second, selectedId: null });
    await act(async () => { await vi.advanceTimersByTimeAsync(800); });
    await act(async () => { resolveFirst(stored(21, first.title)); await vi.runAllTimersAsync(); });

    expect(updateResume).toHaveBeenCalledWith(21, second);
  });

  it('flushes the exact requested snapshot only after an older save finishes', async () => {
    vi.useFakeTimers();
    const first = createEmptyResumeDocument();
    const older = { ...first, title: '旧快照' };
    const exact = { ...first, title: 'AI 请求前快照', targetJobId: 7 };
    let resolveOlder!: (value: StoredResume) => void;
    vi.mocked(updateResume)
      .mockImplementationOnce(() => new Promise((resolve) => { resolveOlder = resolve; }))
      .mockResolvedValueOnce(stored(71, exact.title));
    const { rerender, result } = renderHook(
      ({ document }) => useResumeAutosave({ document, selectedId: 71, onSaved: vi.fn() }),
      { initialProps: { document: first } },
    );
    rerender({ document: older });
    await act(async () => { await vi.advanceTimersByTimeAsync(800); });

    const flush = (result.current as unknown as {
      flush?: (snapshot: typeof exact) => Promise<boolean>;
    }).flush;
    expect(flush).toBeTypeOf('function');
    let flushResult: boolean | undefined;
    let flushing!: Promise<boolean | undefined>;
    act(() => { flushing = flush?.(exact) ?? Promise.resolve(undefined); });
    await act(async () => undefined);
    expect(updateResume).toHaveBeenCalledTimes(1);

    resolveOlder(stored(71, older.title));
    await act(async () => { flushResult = await flushing; });

    expect(vi.mocked(updateResume).mock.calls).toEqual([
      [71, older],
      [71, exact],
    ]);
    expect(flushResult).toBe(true);
  });

  it('reports a failed exact flush to its caller', async () => {
    const document = createEmptyResumeDocument();
    vi.mocked(updateResume).mockRejectedValueOnce(new Error('offline'));
    const { result } = renderHook(() => useResumeAutosave({ document, selectedId: 7, onSaved: vi.fn() }));
    const flush = (result.current as unknown as {
      flush?: (snapshot: typeof document) => Promise<boolean>;
    }).flush;

    expect(flush).toBeTypeOf('function');
    let flushed: boolean | undefined;
    await act(async () => { flushed = await flush?.(document); });
    expect(flushed).toBe(false);
    expect(result.current.error).toBe('保存失败，请稍后重试。');
  });

  it('invalidates pending and in-flight autosaves when a selected draft is deleted', async () => {
    vi.useFakeTimers();
    const document = createEmptyResumeDocument();
    let resolveUpdate!: (value: StoredResume) => void;
    vi.mocked(updateResume).mockImplementationOnce(() => new Promise((resolve) => { resolveUpdate = resolve; }));
    const onSaved = vi.fn();
    const { result, rerender } = renderHook(({ document }) => useResumeAutosave({ document, selectedId: 31, onSaved }), { initialProps: { document } });
    rerender({ document: { ...document, title: '已编辑' } });

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
    rerender({ document: { ...first, title: '创建中' }, selectedId: null });
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
    const { rerender } = renderHook(({ current }) => useResumeAutosave({ document: current, selectedId: null, onSaved: vi.fn() }), { initialProps: { current: document }, wrapper: StrictMode });
    rerender({ current: { ...document, title: 'StrictMode 编辑' } });

    await act(async () => { await vi.advanceTimersByTimeAsync(800); });
    expect(createResume).toHaveBeenCalledWith(expect.objectContaining({ title: 'StrictMode 编辑' }));
  });
});

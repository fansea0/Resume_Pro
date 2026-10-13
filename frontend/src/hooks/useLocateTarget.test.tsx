import { act, cleanup, render } from '@testing-library/react';
import { useRef } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useLocateTarget, HIGHLIGHT_DURATION_MS } from './useLocateTarget';
import type { LocateRequest } from '../components/ResumeLocate';

function Panel({ request, sectionId = 'section-1' }: { request?: LocateRequest; sectionId?: string }) {
  const ref = useRef<HTMLDivElement>(null);
  useLocateTarget(request, ref);
  return (
    <div ref={ref}>
      {/* 刻意不加 open：验证 hook 会主动展开折叠的区块 */}
      <details data-editor-section-id={sectionId}><summary>项目经历</summary>
        <div className="editor-card" data-editor-item-id="item-7">条目</div>
      </details>
    </div>
  );
}

let scrollIntoView: ReturnType<typeof vi.fn>;
const originalMatchMedia = window.matchMedia;

beforeEach(() => {
  vi.useFakeTimers();
  scrollIntoView = vi.fn();
  Element.prototype.scrollIntoView = scrollIntoView;
});

afterEach(() => {
  vi.useRealTimers();
  delete (Element.prototype as unknown as { scrollIntoView?: unknown }).scrollIntoView;
  window.matchMedia = originalMatchMedia;
  cleanup();
});

/** jsdom 与部分旧环境没有 matchMedia：缺失时 hook 必须按"不减弱动效"处理。 */
function removeMatchMedia() {
  Reflect.deleteProperty(window, 'matchMedia');
}

function stubMatchMedia(matches: boolean) {
  window.matchMedia = vi.fn((query: string) => ({
    matches,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
}

describe('useLocateTarget', () => {
  it('定位到区块：滚动到区块根节点并加高亮', () => {
    const { container } = render(<Panel request={{ sectionId: 'section-1', nonce: 1 }} />);

    expect(scrollIntoView).toHaveBeenCalledWith({ behavior: 'smooth', block: 'start' });
    expect(container.querySelector('details')?.classList.contains('is-located')).toBe(true);
  });

  it('目标区块折叠时先展开再滚动', () => {
    const { container } = render(<Panel request={{ sectionId: 'section-1', nonce: 1 }} />);

    const details = container.querySelector('details') as HTMLDetailsElement;
    expect(details.open).toBe(true);
    expect(scrollIntoView).toHaveBeenCalled();
  });

  it('定位到条目时命中条目节点而不是区块节点', () => {
    const { container } = render(<Panel request={{ sectionId: 'section-1', itemId: 'item-7', nonce: 1 }} />);

    const card = container.querySelector('[data-editor-item-id="item-7"]');
    expect(card?.classList.contains('is-located')).toBe(true);
    expect(container.querySelector('details')?.classList.contains('is-located')).toBe(false);
  });

  it('1600ms 后移除高亮', () => {
    const { container } = render(<Panel request={{ sectionId: 'section-1', nonce: 1 }} />);

    const details = container.querySelector('details') as HTMLElement;
    expect(details.classList.contains('is-located')).toBe(true);
    act(() => { vi.advanceTimersByTime(HIGHLIGHT_DURATION_MS - 100); });
    expect(details.classList.contains('is-located')).toBe(true);
    act(() => { vi.advanceTimersByTime(200); });
    expect(details.classList.contains('is-located')).toBe(false);
  });

  it('目标不存在时静默不报错、不滚动', () => {
    render(<Panel request={{ sectionId: 'missing', nonce: 1 }} />);

    expect(scrollIntoView).not.toHaveBeenCalled();
  });

  it('request 为 undefined 时不做任何事', () => {
    const { container } = render(<Panel />);

    expect(scrollIntoView).not.toHaveBeenCalled();
    expect(container.querySelector('details')?.classList.contains('is-located')).toBe(false);
  });

  it('nonce 变化时同一目标可以重放高亮', () => {
    const { container, rerender } = render(<Panel request={{ sectionId: 'section-1', nonce: 1 }} />);
    const details = container.querySelector('details') as HTMLElement;
    act(() => { vi.advanceTimersByTime(HIGHLIGHT_DURATION_MS + 10); });
    expect(details.classList.contains('is-located')).toBe(false);

    rerender(<Panel request={{ sectionId: 'section-1', nonce: 2 }} />);

    expect(details.classList.contains('is-located')).toBe(true);
    expect(scrollIntoView).toHaveBeenCalledTimes(2);
  });

  it('连续定位时旧的定时器不会提前抹掉新高亮', () => {
    const { container, rerender } = render(<Panel request={{ sectionId: 'section-1', nonce: 1 }} />);
    const details = container.querySelector('details') as HTMLElement;

    // 第一次请求的定时器在 t=1600ms 到期，这里故意在其之前发起第二次请求
    act(() => { vi.advanceTimersByTime(500); });
    rerender(<Panel request={{ sectionId: 'section-1', nonce: 2 }} />);
    expect(details.classList.contains('is-located')).toBe(true);

    // t≈1700ms：第一次请求的定时器若未被清理，会在此抹掉第二次请求的高亮
    act(() => { vi.advanceTimersByTime(1200); });
    expect(details.classList.contains('is-located')).toBe(true);
    // 第二次请求自己的 1600ms 从 t=500 起算，应到 t≈2100ms 之后才移除
    act(() => { vi.advanceTimersByTime(500); });
    expect(details.classList.contains('is-located')).toBe(false);
  });

  it('itemId 指向不存在的条目时回退到区块锚点', () => {
    const { container } = render(<Panel request={{ sectionId: 'section-1', itemId: 'item-404', nonce: 1 }} />);

    expect(container.querySelector('details')?.classList.contains('is-located')).toBe(true);
    expect(container.querySelector('[data-editor-item-id="item-7"]')?.classList.contains('is-located')).toBe(false);
  });

  it('id 含引号与方括号时仍按属性取值命中，不受选择器语法影响', () => {
    const hostile = 'x"] .is-located';
    const { container } = render(<Panel request={{ sectionId: hostile, nonce: 1 }} sectionId={hostile} />);

    expect(container.querySelector('details')?.classList.contains('is-located')).toBe(true);
  });

  it('scrollIntoView 不存在时静默降级，不抛异常', () => {
    delete (Element.prototype as unknown as { scrollIntoView?: unknown }).scrollIntoView;
    expect('scrollIntoView' in Element.prototype).toBe(false);

    const { container } = render(<Panel request={{ sectionId: 'section-1', nonce: 1 }} />);

    expect(container.querySelector('details')?.classList.contains('is-located')).toBe(true);
  });

  it('matchMedia 不存在时按 smooth 滚动', () => {
    removeMatchMedia();
    expect(typeof window.matchMedia).toBe('undefined');

    render(<Panel request={{ sectionId: 'section-1', nonce: 1 }} />);

    expect(scrollIntoView).toHaveBeenCalledWith({ behavior: 'smooth', block: 'start' });
  });

  it('用户偏好减弱动效时按 auto 滚动', () => {
    stubMatchMedia(true);

    render(<Panel request={{ sectionId: 'section-1', nonce: 1 }} />);

    expect(window.matchMedia).toHaveBeenCalledWith('(prefers-reduced-motion: reduce)');
    expect(scrollIntoView).toHaveBeenCalledWith({ behavior: 'auto', block: 'start' });
  });
});
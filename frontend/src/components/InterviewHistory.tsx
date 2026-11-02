import { useCallback, useEffect, useRef, useState } from 'react';

import { abortInterview, listInterviews, type Interview } from '../api/interviews';

const STATUS_LABEL = { active: '进行中', completed: '已完成', aborted: '已放弃' } as const;
const GRADE_CLASS: Record<NonNullable<Interview['overallGrade']>, string> = {
  C: 'c', 'B-': 'bm', B: 'b', 'B+': 'bp', A: 'a', S: 's',
};

interface InterviewHistoryProps {
  onOpen: (interview: Interview) => void;
  onStart: () => void;
}

function formatDate(value: string) {
  return new Date(value).toLocaleString();
}

export function InterviewHistory({ onOpen, onStart }: InterviewHistoryProps) {
  const [interviews, setInterviews] = useState<Interview[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [abortingId, setAbortingId] = useState<number | null>(null);
  const mounted = useRef(false);
  const requestVersion = useRef(0);

  const load = useCallback(async () => {
    const version = ++requestVersion.current;
    try {
      const rows = await listInterviews();
      if (mounted.current && version === requestVersion.current) setInterviews(rows);
    } catch {
      if (mounted.current && version === requestVersion.current) setLoadError(true);
    }
  }, []);

  useEffect(() => {
    mounted.current = true;
    setLoading(true);
    void load().finally(() => {
      if (mounted.current) setLoading(false);
    });
    return () => {
      mounted.current = false;
      requestVersion.current += 1;
    };
  }, [load]);

  const active = interviews.find((interview) => interview.status === 'active');

  const handleAbort = async (interview: Interview) => {
    if (abortingId) return;
    if (!window.confirm(`确定放弃面试 #${interview.id} 吗？已答题目与评分会保留，可回看，但不会生成总评。`)) return;
    setAbortingId(interview.id);
    try {
      const aborted = await abortInterview(interview.id);
      if (mounted.current) setInterviews((rows) => rows.map((row) => (row.id === aborted.id ? aborted : row)));
    } catch {
      // 放弃失败保持原状态，用户可重试。
    } finally {
      if (mounted.current) setAbortingId(null);
    }
  };

  return (
    <section aria-label="AI 面试记录" className="interview-history">
      {active ? (
        <div className="interview-history__banner">
          <div>
            <b>面试 #{active.id} — 进行中</b>
            <p>同一时间仅能进行一场面试；继续作答或先放弃当前面试。</p>
          </div>
          <div className="interview-history__banner-actions">
            <button onClick={() => onOpen(active)} type="button">继续面试</button>
            <button disabled={abortingId === active.id} onClick={() => void handleAbort(active)} type="button">
              {abortingId === active.id ? '放弃中…' : '放弃'}
            </button>
          </div>
        </div>
      ) : null}

      <div className="interview-card">
        <h3 className="interview-card__title">历史面试</h3>
        {loading ? (
          <p aria-live="polite" className="interview-review__status" role="status">正在载入面试记录…</p>
        ) : loadError ? (
          <p className="interview-review__status" role="alert">无法载入面试记录，请稍后重试。</p>
        ) : interviews.length === 0 ? (
          <div className="interview-history__empty">
            <p>还没有面试记录。</p>
            <span>在简历编辑器的目标岗位卡片上点击「开始 AI 面试」，即可生成一场针对 JD 的模拟面试。</span>
            <button onClick={onStart} type="button">去目标岗位页开始</button>
          </div>
        ) : (
          <ul className="interview-history__list">
            {interviews.filter((interview) => interview.status !== 'active' || interview.id !== active?.id).map((interview) => (
              <li aria-label={`面试 #${interview.id}`} key={interview.id}>
                <div className="interview-history__info">
                  <b>面试 #{interview.id}</b>
                  <span>
                    {formatDate(interview.completedAt ?? interview.updatedAt)}
                    {interview.status === 'aborted' ? ' · 已答数据可回看' : ''}
                  </span>
                </div>
                <span className={`interview-status interview-status--${interview.status}`}>{STATUS_LABEL[interview.status]}</span>
                {interview.overallGrade ? (
                  <span className={`interview-grade interview-grade--${GRADE_CLASS[interview.overallGrade]}`}>{interview.overallGrade}</span>
                ) : (
                  <span className="interview-history__none">未生成总评</span>
                )}
                <div className="interview-history__actions">
                  <button onClick={() => onOpen(interview)} type="button">
                    {interview.status === 'completed' ? '查看报告' : '回看'}
                  </button>
                </div>
              </li>
            ))}
          </ul>
        )}
      </div>
    </section>
  );
}

import { useEffect, useRef, useState } from 'react';

import {
  getInterviewQuestions,
  type Interview,
  type InterviewGrade,
  type InterviewQuestion,
} from '../api/interviews';

const GRADE_CLASS: Record<InterviewGrade, string> = {
  C: 'c', 'B-': 'bm', B: 'b', 'B+': 'bp', A: 'a', S: 's',
};
const GRADE_ORDER: InterviewGrade[] = ['S', 'A', 'B+', 'B', 'B-', 'C'];
const LABEL_NAMES = { '0': '八股', '1': '算法', '2': '项目' } as const;

interface InterviewReviewProps {
  interview: Interview;
  onBack?: () => void;
}

function QuestionBody({ question }: { question: InterviewQuestion }) {
  const skipped = question.grade === 'C' && !question.userAnswer;
  const extra = question.extra;
  return (
    <div className="interview-review__body">
      {skipped ? (
        <p className="interview-review__skip">已跳过</p>
      ) : question.userAnswer ? (
        <div className="interview-bubble">
          <p className="interview-bubble__label">我的回答</p>
          <p className="interview-bubble__text">{question.userAnswer}</p>
        </div>
      ) : null}
      {question.standardAnswer ? (
        <p className="interview-review__answer"><b>标准答案</b>{question.standardAnswer}</p>
      ) : (
        <p className="interview-review__answer"><b>标准答案</b><span>项目题无标准答案，按综合表现评级</span></p>
      )}
      {extra?.mustPoints.length ? (
        <p className="interview-points">
          <span className="interview-points__label">必答点</span>
          {extra.mustPoints.map((point) => <span className="interview-point" key={point}>{point}</span>)}
        </p>
      ) : null}
      {extra?.bonusPoints.length ? (
        <p className="interview-points">
          <span className="interview-points__label">加分点</span>
          {extra.bonusPoints.map((point) => <span className="interview-point interview-point--bonus" key={point}>{point}</span>)}
        </p>
      ) : null}
    </div>
  );
}

export function InterviewReview({ interview, onBack }: InterviewReviewProps) {
  const [questions, setQuestions] = useState<InterviewQuestion[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const mounted = useRef(false);
  const requestVersion = useRef(0);

  useEffect(() => {
    mounted.current = true;
    const version = ++requestVersion.current;
    getInterviewQuestions(interview.id)
      .then((rows) => {
        if (mounted.current && version === requestVersion.current) setQuestions(rows);
      })
      .catch(() => {
        if (mounted.current && version === requestVersion.current) setLoadError(true);
      })
      .finally(() => {
        if (mounted.current) setLoading(false);
      });
    return () => {
      mounted.current = false;
      requestVersion.current += 1;
    };
  }, [interview.id]);

  const mains = questions.filter((question) => !question.parentId);
  const followUpsByParent = new Map<number, InterviewQuestion[]>();
  for (const question of questions) {
    if (!question.parentId) continue;
    const siblings = followUpsByParent.get(question.parentId) ?? [];
    siblings.push(question);
    followUpsByParent.set(question.parentId, siblings);
  }
  const distribution = new Map<InterviewGrade, number>();
  for (const grade of GRADE_ORDER) distribution.set(grade, 0);
  for (const question of mains) {
    if (question.grade) distribution.set(question.grade, (distribution.get(question.grade) ?? 0) + 1);
  }
  const followUpUsed = questions.filter((question) => question.parentId).length;

  return (
    <section aria-label="AI 面试报告" className="interview-review">
      <div className="interview-hero">
        <div className="interview-hero__grade">
          <span className="interview-hero__letter">{interview.overallGrade ?? '—'}</span>
          <div>
            <b>{interview.overallGrade ? '总评等级' : interview.status === 'aborted' ? '面试已放弃，未生成总评' : '总评等级'}</b>
            <span>{new Date(interview.completedAt ?? interview.updatedAt).toLocaleString()} 完成 · 12 题制 · 追问 {followUpUsed} 次</span>
          </div>
        </div>
        {onBack ? <button onClick={onBack} type="button">返回列表</button> : null}
      </div>

      <div className="interview-card">
        <h3 className="interview-card__title">整体评价</h3>
        <p className="interview-card__review">{interview.overallReview ?? '本场面试未生成整体评价。'}</p>
      </div>

      <div className="interview-card">
        <h3 className="interview-card__title">等级分布 <span>主问题最终等级 · 等级只升不降</span></h3>
        <div className="interview-dist" role="img" aria-label={GRADE_ORDER.map((grade) => `${grade} ${distribution.get(grade) ?? 0} 道`).join('，')}>
          {GRADE_ORDER.map((grade) => {
            const count = distribution.get(grade) ?? 0;
            return count > 0 ? (
              <i className={`interview-dist__seg interview-dist__seg--${GRADE_CLASS[grade]}`} key={grade} style={{ flexGrow: count }}>{count}</i>
            ) : null;
          })}
        </div>
        <p className="interview-dist__legend">
          {GRADE_ORDER.map((grade) => (
            <span key={grade}>
              <i aria-hidden="true" className={`interview-dist__dot interview-dist__seg--${GRADE_CLASS[grade]}`} />
              {grade} × {distribution.get(grade) ?? 0}
            </span>
          ))}
        </p>
        <p className="interview-dist__note">总评算法：中位数定基础档（偶数题取下档）＋ 比例门槛定高档（S 严格过半，A / B+ 至少一半）。只输出等级档位，无 0-100 分。</p>
      </div>

      <div className="interview-card">
        <h3 className="interview-card__title">逐题明细 <span>点击展开回答、标准答案、必答 / 加分点与追问链</span></h3>
        {loading ? (
          <p aria-live="polite" className="interview-review__status" role="status">正在载入逐题明细…</p>
        ) : loadError ? (
          <p className="interview-review__status" role="alert">无法载入逐题明细，请稍后重试。</p>
        ) : (
          <ol className="interview-review__list">
            {mains.map((main, index) => (
              <li key={main.id}>
                <details className="interview-review__row">
                  <summary>
                    <span className="interview-review__index">{String(index + 1).padStart(2, '0')}</span>
                    <span className={`interview-chip interview-chip--${main.label === '0' ? 'octopus' : main.label === '1' ? 'algorithm' : 'project'}`}>{LABEL_NAMES[main.label]}</span>
                    <span className="interview-review__question">{main.question}</span>
                    {(followUpsByParent.get(main.id)?.length ?? 0) > 0 ? (
                      <span className="interview-follow-count">追问 ×{followUpsByParent.get(main.id)!.length}</span>
                    ) : null}
                    {main.grade ? (
                      <span className={`interview-grade interview-grade--${GRADE_CLASS[main.grade]}`}>{main.grade}</span>
                    ) : null}
                  </summary>
                  <QuestionBody question={main} />
                  {(followUpsByParent.get(main.id) ?? []).map((followUp) => (
                    <div className="interview-follow" key={followUp.id}>
                      <p className="interview-follow__head">
                        <span className="interview-follow__chip">追问 {followUp.followUpRound}</span>
                        {followUp.grade ? (
                          <span className={`interview-grade interview-grade--${GRADE_CLASS[followUp.grade]}`}>{followUp.grade}</span>
                        ) : null}
                      </p>
                      <p className="interview-msg__question">{followUp.question}</p>
                      <QuestionBody question={followUp} />
                    </div>
                  ))}
                </details>
              </li>
            ))}
          </ol>
        )}
      </div>
    </section>
  );
}

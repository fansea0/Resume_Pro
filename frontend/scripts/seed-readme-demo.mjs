#!/usr/bin/env node
/**
 * Seeds the README demo tenant: one Go-backend target job and one resume bound
 * to it, so the screenshots show a coherent candidate (resume matches the
 * Go 八股/项目 questions the interview generator produces).
 *
 * Usage: node scripts/seed-readme-demo.mjs
 * Requires the API on http://127.0.0.1:8080.
 */

const API = process.env.README_API || 'http://127.0.0.1:8080';
const EMAIL = process.env.README_SHOT_EMAIL || 'readme-shot@example.com';
const PASSWORD = process.env.README_SHOT_PASSWORD || 'Readme-Shot-2026';

const JD = [
  '负责公司游戏运营后台与 AI 客服平台的服务端开发。',
  '',
  '要求：',
  '- 5 年以上 Go 后端经验，熟悉高并发服务设计、MySQL 分库分表、Redis 缓存与限流降级；',
  '- 熟悉分布式事务、消息队列与线上稳定性治理，有大促/故障演练经验；',
  '- 有 AI 应用落地经验（意图识别、RAG 知识库、模型推理服务）者优先；',
  '- 具备良好的系统设计能力与技术方案输出能力。',
].join('\n');

const RESUME = {
  schemaVersion: 2,
  title: 'Go 后端工程师简历',
  templateId: 'tech',
  profile: {
    fullName: '陈知远',
    headline: 'Go 后端工程师 · 高并发平台与 AI 应用落地',
    email: 'chenzhiyuan@example.com',
    phone: '138-0000-0000',
    location: '武汉',
    website: 'github.com/example',
    avatarUrl: '',
    avatarStorageKey: '',
    showAvatar: false,
  },
  sections: [
    {
      id: 'summary',
      type: 'summary',
      title: '个人总结',
      visible: true,
      content: {
        markdown:
          '5 年 Go 后端开发经验，专注高并发业务中台与 AI 应用落地。主导过游戏运营后台从单体到服务化拆分、日均千万级请求的稳定性治理，AI 客服从 0 到 1 建设并使客服人力下降 70%。熟悉 Go、MySQL、Redis、Kafka 与 LLM 应用工程化。',
      },
    },
    {
      id: 'work',
      type: 'work',
      title: '工作经历',
      visible: true,
      content: {
        items: [
          {
            id: 'work-1',
            company: '微派网络科技',
            role: '高级后端工程师',
            startDate: '2021.06',
            endDate: '至今',
            location: '武汉',
            achievements: '主导运营后台服务化改造，接口 P99 从 320ms 降至 90ms；AI 客服上线后人工客服人力下降 70%',
            highlightsMarkdown:
              '- **服务化拆分**：把单体运营后台按资源变更、道具发放、战绩统计拆成独立服务，发布互相阻塞问题下降 80%。\n- **稳定性治理**：Redis Cluster 分片 + 本地二级缓存防热点，MySQL 按业务分库、流水表分表；全链路引入 Trace 与 Trace-based 告警定位耗时毛刺。\n- **AI 客服 0→1**：意图识别用微调模型 + 规则兜底，知识库走向量检索召回，转人工携带上下文摘要，重复问题自动应答覆盖 68%。',
            showLocation: true,
            showAchievements: true,
          },
        ],
      },
    },
    {
      id: 'project',
      type: 'project',
      title: '项目经历',
      visible: true,
      content: {
        items: [
          {
            id: 'project-1',
            name: 'AI 智能客服平台',
            role: '技术负责人',
            startDate: '2023.03',
            endDate: '2024.09',
            location: '武汉',
            link: '',
            introduction: '面向游戏玩家与运营团队的智能客服平台，覆盖意图识别、知识库检索与工单流转。',
            techStack: 'Go / Redis / Milvus / Kafka / Kubernetes',
            achievements: '自动应答覆盖率 68%，客服人力下降 70%，知识库自动分类准确率 91%',
            highlightsMarkdown:
              '- **意图识别**：微调模型 + 规则兜底的混合路由，低置信度问题转人工，避免答非所问。\n- **知识库治理**：文档自动分类 + 向量召回，召回准确率从 54% 提升到 83%。\n- **稳定性**：全链路 Trace 与 Trace-based 告警，线上问题定位时长从小时级降到分钟级。',
            showLocation: true,
            showLink: false,
            showIntroduction: true,
            showTechStack: true,
            showAchievements: true,
          },
          {
            id: 'project-2',
            name: '游戏运营后台重构',
            role: '后端负责人',
            startDate: '2021.09',
            endDate: '2022.12',
            location: '武汉',
            link: '',
            introduction: '面向内部运营的资源变更、道具发放与战绩统计后台重构项目。',
            techStack: 'Go / MySQL / Redis / Kafka',
            achievements: '日均千万级请求稳定运行，发布回滚时间从 30 分钟缩短到 3 分钟',
            highlightsMarkdown:
              '- **资源变更追踪**：所有运营改动落审计流水，支持按人按时间回溯。\n- **道具发放幂等**：基于业务唯一键 + 幂等表，重复发放问题归零。\n- **容量与限流**：令牌桶限流 + 分级降级，大促期间核心接口可用性保持 99.99%。',
            showLocation: true,
            showLink: false,
            showIntroduction: true,
            showTechStack: true,
            showAchievements: true,
          },
        ],
      },
    },
    {
      id: 'skills',
      type: 'skills',
      title: '专业技能',
      visible: true,
      content: {
        items: [
          { id: 'skill-1', name: 'Go', note: '并发模型、GORM、gRPC、性能剖析与压测调优' },
          { id: 'skill-2', name: 'MySQL / Redis', note: '分库分表、慢查询治理、缓存一致性与热点 key 应对' },
          { id: 'skill-3', name: '分布式与中间件', note: 'Kafka、幂等设计、分布式事务、服务治理与可观测性' },
          { id: 'skill-4', name: 'AI 应用工程', note: '意图识别、RAG 知识库、模型推理服务接入与效果评估' },
        ],
      },
    },
    {
      id: 'education',
      type: 'education',
      title: '教育经历',
      visible: true,
      content: {
        items: [
          {
            id: 'edu-1',
            school: '武汉大学',
            startDate: '2016.09',
            endDate: '2020.06',
            degree: '本科',
            college: '计算机学院',
            major: '软件工程',
          },
        ],
      },
    },
  ],
};

let sessionCookie = '';

async function call(path, options = {}) {
  const response = await fetch(API + path, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(sessionCookie ? { Cookie: sessionCookie } : {}),
      ...(options.headers || {}),
    },
  });
  const setCookie = response.headers.getSetCookie ? response.headers.getSetCookie() : [];
  for (const raw of setCookie) {
    const pair = raw.split(';')[0];
    if (pair.startsWith('resume_session=')) sessionCookie = pair;
  }
  const text = await response.text();
  if (!response.ok) throw new Error(path + ' -> ' + response.status + ' ' + text.slice(0, 300));
  return text ? JSON.parse(text) : null;
}

async function login() {
  try {
    return await call('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email: EMAIL, password: PASSWORD }),
    });
  } catch {
    return await call('/api/auth/register', {
      method: 'POST',
      body: JSON.stringify({ email: EMAIL, password: PASSWORD }),
    });
  }
}

const main = async () => {
  const user = await login();
  console.log('user:', user.user.email);

  const existing = await call('/api/target-jobs');
  let targetJob = existing.find((job) => job.company === '微派网络科技');
  if (!targetJob) {
    targetJob = await call('/api/target-jobs', {
      method: 'POST',
      body: JSON.stringify({ company: '微派网络科技', position: '高级后端工程师', requirements: JD }),
    });
    console.log('created target job:', targetJob.id);
  } else {
    console.log('reusing target job:', targetJob.id);
  }

  const content = { ...RESUME, targetJobId: targetJob.id };
  const created = await call('/api/resumes', {
    method: 'POST',
    body: JSON.stringify({ title: content.title, templateId: content.templateId, content }),
  });
  console.log('created resume:', created.id);
};

main().catch((err) => {
  console.error('FAILED:', err.message);
  process.exit(1);
});

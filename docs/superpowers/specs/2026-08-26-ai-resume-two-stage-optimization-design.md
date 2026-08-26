# AI 简历两阶段优化设计

> 日期：2026-08-26  
> 状态：已确认，待实施计划  
> 范围：现有章节级 AI 润色（`summary`、`work`、`internship`、`project`）

## 目标

将当前的单阶段 AI 润色升级为单次调用中的两阶段编排：

1. **JD 对齐**：根据已绑定的目标 JD，在用户既有事实范围内突出相关关键词、能力信号和成果优先级，提升岗位匹配度与 ATS 可读性。
2. **表达调优**：对 JD 对齐后的候选文本按 S1–S8 进行安全优化，并返回最终可采纳文本、命中问题及处理说明。

用户仍以单个章节发起润色并手动采纳。系统不得编造数字、职责、技能、离职原因或其他事实。

## 非目标

- 不支持整份简历一键优化。
- 不增加第二次 LLM 调用，不向用户暴露阶段一中间文本。
- 不自动将 S1 的量化占位示例写入简历。
- 不把分析、解释或进度持久化到简历正文或数据库。
- 不允许 AI 改写公司、职位、日期、项目名称、链接等既有冻结字段。

## 现有约束

- 请求仍由 `POST /api/ai/resumes/:id/section/rewrite` 发起；浏览器仅提交章节 id、类型和可选指令。
- handler 从落库简历取当前章节，并按简历绑定的目标岗位读取 JD；AI service 保持不访问数据库。
- 现有 rewrite schema 是 section schema 的严格子集，采纳时按 item id 和字段白名单合并。
- 支持的章节类型为 `summary`、`work`、`internship`、`project`。

## 服务端编排

### 输入上下文

handler 为 service 构造以下内部上下文：

```text
sectionType
currentContent       // 当前章节 JSON，仅含可读的当前内容
targetJob            // company / position / requirements，可为空
rewriteableFields    // 按章节的字段白名单
frozenFacts          // 作为事实边界，不提供输出位
```

章节内容始终来自数据库。没有绑定 JD 时请求仍成功：跳过岗位特定术语对齐，继续执行通用表达调优，并在分析中明确说明。

### 单次模型调用的两阶段工作流

`RewriteSection` 仍只调用模型一次。Prompt 明确要求模型在同一受约束任务内依次完成：

1. 从 JD 提取优先关键词、职责和能力信号；只使用原章节可支撑的内容进行对齐，形成不对外返回的中间候选文本。
2. 以候选文本为输入检查 S1–S8，产出最终 `rewrite` 和结构化分析。

服务端不请求、不存储、也不向客户端转发模型原始逐字推理。对用户展示的是稳定、可审阅的阶段进度和结构化分析摘要。

## 输出契约

SSE 的 `suggestion` 事件返回：

```json
{
  "rewrite": {
    "items": [
      {
        "id": "work-1",
        "achievements": "主导……",
        "highlightsMarkdown": "- 推动……"
      }
    ]
  },
  "analysis": {
    "jdAlignment": {
      "status": "applied",
      "matchedKeywords": ["用户增长", "跨团队协作"],
      "excludedKeywords": ["SQL"]
    },
    "matchedIssues": [
      {
        "code": "S2",
        "label": "动词贫瘠",
        "target": {"itemId": "work-1", "field": "highlightsMarkdown"},
        "resolution": "将弱动词替换为具体动作动词，并前置行动。",
        "status": "fixed"
      }
    ],
    "evidenceRequests": [
      {
        "code": "S1",
        "target": {"itemId": "work-1", "field": "achievements"},
        "reason": "当前未说明覆盖范围或转化效果。",
        "exampleWithPlaceholder": "主导用户增长活动，覆盖 XX 名用户，推动转化率提升 XX%。"
      }
    ],
    "warnings": [
      {
        "code": "S7",
        "resolution": "已依据既有跨行业项目经验重组叙事；未推测转行或空窗原因。"
      }
    ]
  }
}
```

`rewrite` 复用原有窄 schema，是唯一可以被前端采纳的内容。`analysis` 只用于解释本次生成，不参与简历合并。

所有分析 target 都必须属于当前章节、当前 item 且指向可改写字段。`summary` 使用其可改字段作为 target；item 型章节使用 `itemId + field`。

## S1–S8 策略规则

| 策略 | 触发条件 | 进入 `rewrite` | 可展示结果 |
| --- | --- | --- | --- |
| S1 量化缺失 | 有规模或结果语义，缺少可靠数字 | 否 | 带 `XX` 的补充示例 |
| S2 动词贫瘠 | 出现“负责、参与、协助”等弱动词 | 是 | 动词替换说明 |
| S3 职责堆砌 | 有职责但缺少个人行动或结果 | 仅可由原文安全重组时 | SAR 重组说明或证据提示 |
| S4 成果模糊 | 有行动但价值未表达 | 仅可由原文事实推出时 | 商业价值翻译说明 |
| S5 关键词缺失 | JD 关键词被原经历支撑但未突出 | 是 | 注入关键词与依据 |
| S6 句式平庸 | 冗余、松散或主谓不清 | 是 | 结构化表达说明 |
| S7 劣势暴露 | 空窗、频繁跳槽或转行线索 | 仅重组已有事实 | 叙事重构说明，不推测原因 |
| S8 冗长啰嗦 | 信息重复、修饰词过多、句子过长 | 是 | 压缩说明 |

S1 仅出现于 `evidenceRequests`。其中 `XX`、`待补充`、推测性措辞及任何未验证数值均禁止出现在 `rewrite`。

## SSE 进度与可解释过程

新增受控的 `progress` 事件，不转发模型 reasoning token：

```text
context_loaded
→ jd_alignment_started
→ jd_alignment_completed
→ optimization_started
→ optimization_completed
→ suggestion
→ done
```

事件包含稳定的短消息与可选摘要；例如 `jd_alignment_completed` 可返回实际使用的关键词，以及仅在影响结果时返回的未使用关键词。事件不得包含模型自由文本推理。

生成完成后，前端按顺序展示：JD 对齐摘要、命中问题与修复方式、待补充量化信息、最终字段 diff。没有 JD 时显示未执行 JD 对齐；没有安全改写时明确保留原文。

## 校验、失败与降级

1. **rewrite 校验**：维持严格 JSON schema、条目 id 对齐和字段白名单；拒绝冻结字段、占位符、推测事实或不合法结构。
2. **analysis 校验**：验证 code、target、`fixed` 与实际 diff 的一致性；S1 示例只能在 `evidenceRequests` 中。
3. **重试**：输出不符合契约时，进行一次仅修复 JSON 契约的模型重试。
4. **降级**：若 `rewrite` 安全有效但 `analysis` 不完整，返回可采纳改写并附带“未能生成完整分析说明”的受控提示。若 `rewrite` 不安全或无效，整次请求失败，不返回可采纳内容。
5. **取消与重复生成**：沿用现有 abort/version 机制；前端只以当前请求版本的事件和 suggestion 为准。

## 前端采纳语义

“采纳”只合并 `rewrite` 到内存中的当前章节，再走既有自动保存流程。分析、进度和 S1 量化示例仅在 AI 建议面板存活，不写入简历正文或数据库。

`XX` 示例在视觉和语义上标记为“待补充数据”，不提供采纳入口。用户可以手动依据示例回到编辑器填写真实数字。

## 测试

- 后端 schema：四类章节不能返回冻结字段；`rewrite` 中不能含 `XX`。
- 后端编排：有/无 JD、S1–S8 的触发与不触发、S7 不编造解释、S5 不注入无事实依据关键词。
- 后端一致性：每个 `fixed` 诊断有真实字段 diff；分析 target 不越界；S1 不泄漏到 rewrite。
- SSE：进度顺序、模型 reasoning 不透传、analysis 降级和失败事件。
- 前端：进度、分析摘要、量化示例不可采纳、采纳只合并 rewrite、无 JD/无安全改写状态。
- 回归：现有章节润色、重新生成、取消、错误状态与无障碍交互。

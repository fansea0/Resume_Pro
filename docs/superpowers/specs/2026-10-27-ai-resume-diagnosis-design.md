# AI 简历诊断与逐条润色设计（v2）

> 日期：2026-10-27
> 状态：待实施
> 范围：章节级 AI 润色（`summary`、`work`、`internship`、`project`），单章节输入
> 变更：v1 的「模型输出锚点」改为「模型输出改写全文 + 服务端 diff 出片段」

## 背景

现有实现（`2026-08-26-ai-resume-two-stage-optimization-design.md`）是「章节级、字段级、整体采纳」：模型直出整字段改写，`analysis.matchedIssues` 只能定位到 `itemId + field`，`resolution` 是一句套话，前端只能整体采纳或整体拒绝。

产品诉求是：展示「有哪些问题 → 修改后的内容 → 推荐怎么改 → 为什么这么改」，并支持一键应用。两者的最小可寻址单位不同，必须改造数据契约。

## 已确认的决策

1. **一次调用同时返回诊断与润色**：不新增体检报告入口，不做第二次模型调用。
2. **片段级逐条应用 + 全部应用**。
3. **诊断维度**：保留 S1–S8，新增 S10（信息密度）；**不做 S9（一致性 / 错别字 / 日期统一）**。
4. **单章节输入**，不做跨章节联合诊断。
5. **保留 summary 润色**，与其它章节共用同一套建议卡片交互。
6. **建议片段由服务端 diff 产出**，模型不写锚点。
7. **模型凭空新增的内容一律拒绝**；每条建议带服务端派生的严重度并按严重度排序。

## 为什么不用「模型写锚点」

若模型自己输出 `anchor`（原文子串），它写歪一点（多一个字、换个标点）服务端就判定锚点不唯一或找不到，这条建议被静默丢弃——用户看到「AI 说有问题但面板里没有」。改成服务端 diff 后：

- 锚点由服务端算出，必然是原文精确子串；
- 模型失败模式从「建议消失」变成「建议变多」，后者由用户取舍，不丢工作；
- 模型任务更简单：只需把字段改好，不必精确摘录原文。

## 模型输出契约

模型输出「每个目标字段的改写全文」，服务端 diff 出片段建议。

```json
{
  "edits": [
    {
      "target": { "itemId": "work-1", "field": "highlightsMarkdown" },
      "newText": "- 主导智能内容助手的产品规划与迭代，梳理创作、审核、分发全链路。\n- 通过访谈、漏斗分析定位新手流失点，首周留存率提升 18%。",
      "issues": [
        {
          "code": "S2",
          "label": "弱动词",
          "issue": "用「负责」开头，没有体现你的主导范围",
          "rationale": "招聘方通常只读前 3 秒，需要一眼看到动作强度与负责范围",
          "quote": "主导智能内容助手的产品规划与迭代"
        }
      ]
    }
  ],
  "analysis": {
    "jdAlignment": {
      "status": "applied",
      "matchedKeywords": ["用户增长"],
      "excludedKeywords": ["SQL"],
      "missingKeywords": ["A/B 实验"]
    },
    "evidenceRequests": [
      {
        "code": "S1",
        "target": { "itemId": "work-1", "field": "achievements" },
        "reason": "这条只写了职责，没有说明覆盖范围或结果",
        "exampleWithPlaceholder": "主导用户增长活动，覆盖 XX 名用户，转化率提升 XX%"
      }
    ],
    "warnings": [{ "code": "S7", "resolution": "已基于既有事实重组叙事，未推测离职原因" }]
  }
}
```

约束：

- `edits[].target.field`：`summary` 为 `markdown`（无 `itemId`）；`work`/`internship` 为 `achievements` / `highlightsMarkdown`；`project` 另含 `introduction`。`itemId` 必须命中当前章节已有条目。
- `issues[].code` 枚举：`S2 S3 S4 S5 S6 S8 S10`。
- `issues[].quote` 必须是 `newText` 中的一段连续文字；服务端用它把原因挂到具体片段。
- `newText` 不得包含 `XX` / `待补充`，不得引入原文没有的数字。

## 服务端处理

### 1. diff 出片段

对每个 edit：

1. 取该字段原文 `original` 与模型给的 `newText`；
2. 剥掉公共前后缀后，用 LCS 计算差异，切成 hunk 列表；
3. 相邻的「删除 + 新增」合并为「替换」，间隔 ≤ 4 个未变字符的改动也合并，避免碎建议；
4. 每个 hunk 是一条候选建议：`anchor` = 原文片段，`replacement` = 改后片段，`start`/`end` = 原文中的 rune 偏移；
5. **只有插入、没有改写**（模型纯粹在原文后面追加内容）→ 整个 edit 被拒绝，提示「模型只追加了原文没有的内容，该字段的建议已忽略」。混合改写里的插入片段视为改写的一部分，正常采纳；
6. **纯删除**（原文有、改后无）：作为建议，`replacement` 为空串，客户端删除该片段；
7. 替换型 hunk 是主要形态。

> 为什么不是「出现插入就拒绝」：字符级 diff 里任何改写都会产生插入片段（例：`old` → `valid single-call optimization` 会被切成「o→va」+ 插入「i」+ 插入「 rest」）。只有「全部 hunk 都是插入」才说明模型在凭空追加。

### 2. 把原因挂到片段

对每条 issue：在 `newText` 中定位 `quote`（必须唯一命中），找出包含它的 hunk，把 `issue` / `rationale` / `code` 挂到该 hunk。定位不到或命中多处 → 该 issue 不挂载（片段仍展示，只是没有原因文案）。

### 3. 安全校验（逐条降级）

单条建议不合法时丢弃该条并记录原因，不影响其它建议；整体 JSON 契约不合法时沿用现有「一次契约修复重试 → 失败」路径。

- **数字守恒**：`replacement` 中出现的每个数字必须在该字段原文中已存在，否则丢弃（原因：可能引入未验证数据）；
- **占位符拦截**：`replacement` 出现 `XX` 或 `待补充` 直接丢弃；
- **删除含数字的片段**：删除文本里带数字时丢弃（避免悄悄抹掉数据）；
- **空改动**：`anchor` 与 `replacement` 相同则丢弃；
- **原文残留**：diff 出的 `anchor` 必须是原文精确子串（由 diff 保证，无需额外校验）。

### 4. 派生整字段改写

服务端按 `start` 偏移**从后往前**（start 降序）把 `replacement` 替换进字段原文，得到最终文本并组装 `rewrite`。倒序替换保证前面片段的偏移不受影响，客户端用同一算法即可得到逐字一致的结果：

- `rewrite` 结构与现有契约一致：`summary` 为 `{markdown}`；条目型为 `{items:[{id, achievements, highlightsMarkdown[, introduction]}]}`，只包含发生变化的条目；
- 因此「全部应用」的结果与服务端派生结果逐字一致，不存在两份真源。

### 5. 严重度（服务端派生，不给模型负担）

| 严重度 | 检查码 |
| --- | --- |
| 高 | S2 S3 S4 |
| 中 | S5 S6 S8 |
| 低 | S10 |

面板按严重度降序、组内保持服务端顺序展示。

### 6. 密度指标

服务端对每个条目的可改写字段计算确定性指标，与模型输出无关：

| 字段 | 上限 |
| --- | --- |
| `markdown`（summary） | 150 字 / 4 行 |
| `achievements` | 60 字 / 2 行 |
| `introduction` | 60 字 / 1 行 |
| 单条 bullet | 40 字 |
| 每条目 bullet 条数 | 5 |

输出 `analysis.density: [{itemId, field, chars, lines, bullets, maxBulletChars, overLimit}]`，用于维度总览展示客观指标。

## 客户端契约

`suggestion` 事件的 `newContent`：

```json
{
  "rewrite": { "items": [ /* 服务端派生后的最终态 */ ] },
  "analysis": {
    "jdAlignment": { "status": "applied", "matchedKeywords": [], "excludedKeywords": [], "missingKeywords": [] },
    "suggestions": [
      {
        "id": "sg-1",
        "code": "S2",
        "label": "弱动词",
        "severity": "high",
        "target": { "itemId": "work-1", "field": "highlightsMarkdown" },
        "issue": "用「负责」开头，没有体现你的主导范围",
        "anchor": "负责智能内容助手的产品规划与迭代",
        "replacement": "主导智能内容助手的产品规划与迭代",
        "rationale": "招聘方通常只读前 3 秒，需要一眼看到动作强度与负责范围",
        "start": 2,
        "end": 18,
        "status": "applied",
        "rejectionReason": ""
      }
    ],
    "evidenceRequests": [],
    "warnings": [],
    "density": []
  }
}
```

- `suggestions[].status` 为 `applied` 或 `rejected`；`rejected` 时 `rejectionReason` 必填，客户端折叠展示为「已跳过 N 条建议」。
- `suggestions[].severity` 为 `high` / `medium` / `low`。
- `suggestions[].start` / `end` 是原文该字段中的 rune 偏移，`anchor` 是这两点之间的精确切片。纯插入的建议 `start == end`、`anchor` 为空串。

## 前端交互

面板自上而下：

1. **维度总览**：固定维度列表，四态展示——通过 / N 处建议 / 待补数据 / 未检查。不使用 0-100 分。
   - 动作与结构（S2 S3 S6）、结果与量化（S4 + S1 待补数据）、JD 关键词（S5，未绑定岗位时未检查）、信息密度（S10 + `density` 超限条目数）。
2. **JD 对齐**：已覆盖关键词、建议覆盖（`missingKeywords`，说明本节没有事实支撑，AI 不能替你补）、未纳入关键词；未绑定岗位时显示未检查。
3. **建议卡片**（只渲染 `status === 'applied'`，按严重度降序）：严重度 + 检查码 + 类型徽标 → 定位（第几条经历 · 字段中文名）→ 原文片段（删除线）→ 建议内容（新增样式）→ `issue` → `rationale` → 密度客观指标（如有）→「应用 / 撤销」。
4. **待补充数据**：`evidenceRequests`，明确标注不会写入简历。
5. **提示**：`warnings`。
6. `rejected` 建议折叠为「已跳过 N 条建议」，展开可见 `rejectionReason`。

底部操作：`全部应用`（勾选所有可用建议）、`应用到简历`（提交草稿，走既有自动保存）、`重新生成`、`关闭`。

**草稿模型**：客户端保存「每个字段被勾选的 suggestion id 集合」；字段文本 = 从原文出发，把被勾选的建议按 `start` 降序依次 splice 进码点数组。撤销即移除 id 并重算，不做反向字符串替换，因此永远可逆，且「全部应用」与服务端 `rewrite` 逐字一致。

**提交**：草稿非空时提交等价 `rewrite`（缺省字段表示不改），复用既有 `mergeRewrite` 与自动保存。分析、密度指标、跳过原因只存在于面板，不写入简历正文或数据库。

## 模型调用链上的配套改动

- **契约修复重试**：模型输出不合约时，追加一条「上一次输出不符合 schema」的纠正消息再调一次（最多 1 次）。这是链路上唯一可能出现的第二次模型调用。
- **输出截断识别**：模型返回的 JSON 不完整（多半是被 `AI_MAX_TOKENS` 截断）时，错误码为 `TRUNCATED_OUTPUT`，与契约违规区分开。
- **模型参数**：`AI_MAX_TOKENS` 默认 2048 → 4096（v2 改为回写整段文本，输出量更大）；`AI_REQUEST_TIMEOUT` 默认 30s → 60s。
- **可观测性**：`CallResult` 增加 `SuggestionsApplied` / `SuggestionsRejected`，日志能直接看到「模型提了 N 条、采纳 M 条、丢弃 K 条」。

## 不做

- 不预测真实页数，只做密度代理指标。
- 不做独立体检报告入口，不做第二次模型调用。
- 不做跨章节一致性检查，不做 S9。
- 不让模型新增数字或删除既有事实；模型凭空新增的内容一律拒绝。

## 测试

- 后端 diff：公共前后缀剥离、纯替换 / 纯删除 / 纯新增三类 hunk、新增内容被拒绝。
- 后端挂载：`quote` 唯一命中才挂原因，命中不到或多处命中时不挂。
- 后端校验：数字守恒拦截新数字、占位符拦截、空改动丢弃。
- 后端派生：`rewrite` 等于按顺序应用建议后的结果；无有效建议时 `rewrite` 等于原文。
- 后端严重度：按码派生高 / 中 / 低。
- 后端密度：超限条目被标记，未超限不标记。
- SSE：进度顺序不变，`suggestion` 事件携带新契约。
- 前端：逐条应用只改目标字段；撤销可逆；全部应用结果与服务端 `rewrite` 一致；`XX` 示例不可采纳；无 JD、无安全改写、错误状态。
- 回归：现有章节润色、重新生成、取消、无障碍交互。

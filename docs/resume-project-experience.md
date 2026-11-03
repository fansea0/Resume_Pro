# 项目经历（简历用）

## Resume Pro | AI 简历优化与模拟面试平台

**技术栈：** Go、Gin、CloudWeGo Eino、SQLite、Redis、SSE、OpenAI 兼容 API（DeepSeek / MiniMax）、React、TypeScript、Vite、JWT

**项目介绍：**

面向求职场景的本地优先简历平台，基于 CloudWeGo Eino Agent 编排实现目标岗位（JD）对齐的简历章节润色与多轮 AI 模拟面试，覆盖「优化表达 → 投递 → 面试 → 总评」闭环。

**主要工作：**

* 构建基于 Eino Graph 的 AI 模拟面试 Agent，编排提问、评分、追问决策、总评、收尾五个节点，以 StatefulInterrupt 实现跨请求挂起与恢复，Redis checkpoint 持久化会话游标与追问预算，支持服务重启后继续面试。
* 设计 Responses API 续聊机制，用 response_id 游标替代全量上下文重建，并透传 reasoning_effort 修复推理模型空转烧满 token 预算导致的生题超时，将生题耗时从 171s 失败优化至 15s 成功。
* 实现服务端 diff 驱动的逐条改写建议机制：模型只产出结构化 edits，服务端以 LCS diff 计算变更锚点并做数字接地校验（新增数字必须已在原文出现），杜绝模型编造业绩数据，违规建议单条拒绝而非整批失败。
* 建立 JSON Schema 强约束输出契约与契约修复重试机制，输出不合法时携带校验错误回灌重试，并区分截断与结构违规错误码；配套确定性信息密度度量（字符数、行数、要点数预算）指导模型压缩表达。
* 以 SSE 实现流式 AI 交互，按 progress / suggestion / error / done 分级推送事件并保证终态原子投递；配套 Redis SET NX EX 并发锁避免同一面试重复处理，多租户数据隔离与 AI 调用 Hook（耗时、Token、重试次数）落日志。

**规模数据：** 270 次提交 · 75 个测试文件（Go 37 / 前端 38）· 13 套简历模板 · 37 个后端模块 · 170+ 源文件

---

## 变体（按投递岗位调整侧重）

### 侧重 AI / Agent 工程

保留全部 5 条，重点强化第 1、2、3 条（Agent 编排、推理模型适配、输出可信度），可把项目名改为：

**Resume Pro | 基于 Eino Graph 的 AI 简历优化与面试 Agent**

### 侧重 Go 后端工程

**技术栈** 前置加：多租户隔离、SQLite、Redis 分布式锁、SSE 长连接、JWT 鉴权

主要工作改为：

* 设计多租户简历数据模型与资源归属校验，所有简历 / 目标岗位 / 面试读写按 tenant_id 隔离，跨账号访问统一返回未找到。
* 以 SQLite + 迁移脚本管理简历、目标岗位、面试与追问树结构，回答回写与追问落库置于事务内保证一致性。
* 引入 Redis SET NX EX 并发锁与 7 天 TTL checkpoint 存储，面试进行中拒绝并发回答（409），checkpoint 缺失时映射为会话过期（410）并保留历史答案可回看。
* 以 SSE 实现 AI 交互长连接，设计 progress / suggestion / error / done 四级事件契约，缓冲通道保留两个槽位保证终态事件不被慢消费者阻塞。
* 落地 AI 调用可观测 Hook，记录租户、操作类型、耗时、输入输出 Token 与重试次数；接入 JWT + HttpOnly Cookie 鉴权与开发环境免密登录端点。

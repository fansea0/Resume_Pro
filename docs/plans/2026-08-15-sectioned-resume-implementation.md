# 区块化简历实施计划

> **给实现者：** 全程采用测试驱动开发。先编写失败测试，确认其失败后，再编写最小实现。

**目标：** 将简历实现为可持久化、可排序、可隐藏的类型化区块集合；任何模板都能渲染全部区块，且不减少用户可填写的字段。

**架构：** `ResumeDocument` v2 结构持有语义区块和区块顺序。`documentFromStored` 读取标准 v2 内容，并在内存中迁移旧的固定数组内容。编辑器编辑 v2 区块；共享渲染器把可见区块转为语义标记，各模板提供自己的展示组件与 CSS。

**技术栈：** React 18、TypeScript、Vitest、Testing Library，以及 `react-markdown` + `remark-gfm`（不支持原始 HTML）。

---

### 任务 1：定义并创建 v2 区块化简历文档

**文件：**

- 修改：`frontend/src/types/resume.ts`
- 修改：`frontend/src/types/resume.test.ts`

**步骤 1：编写失败测试**

为 `createEmptyResumeDocument()` 添加测试，断言它返回 `schemaVersion: 2` 和 `sections: []`；为 `createSampleResumeDocument()` 添加测试，断言它含有全部八种区块类型、稳定的唯一 ID，并且默认 `visible: true`。

**步骤 2：运行测试并确认失败**

命令：`cd frontend && npm test -- src/types/resume.test.ts`

预期：失败，因为尚不存在 `schemaVersion` 和 `sections`。

**步骤 3：编写最小实现**

使用下述可辨识联合类型取代旧的顶层内容数组；`ResumeProfile` 仅保留联系信息字段（个人总结移入区块）。

```ts
type ResumeSection = SummarySection | ExperienceSection | ProjectSection |
  EducationSection | SkillsSection | CampusSection | HonoursSection;
// 每个变体都继承 { id, type, title, visible }，并持有 { items } 或
// { markdown }。工作与实习使用同一种 ExperienceSection 形状。
```

为八种类型增加区块工厂函数。所有字符串、显示开关和条目数组都必须初始化，保证编辑时不依赖 `undefined`。

**步骤 4：运行测试并确认通过**

命令：`cd frontend && npm test -- src/types/resume.test.ts`

预期：通过。

**步骤 5：提交**

`git add frontend/src/types/resume.ts frontend/src/types/resume.test.ts && git commit -m "feat: define sectioned resume document"`

### 任务 2：校验并迁移已存储内容

**文件：**

- 修改：`frontend/src/api/resumes.ts`
- 修改：`frontend/src/api/resumes.test.ts`

**步骤 1：编写失败测试**

添加三类测试：恢复字段完整的 v2 文档；恢复每一种 v2 区块类型；恢复一份旧格式文档并断言生成顺序为个人总结、工作经历、项目经历、教育经历、专业技能。还要断言格式错误的变体会被拒绝。

**步骤 2：运行测试并确认失败**

命令：`cd frontend && npm test -- src/api/resumes.test.ts`

预期：失败，因为现有校验器只能识别固定数组。

**步骤 3：编写最小实现**

为每种 v2 区块变体实现严格的运行时守卫，覆盖所有必填字符串和布尔显示开关。新增 `migrateLegacyDocument()`：识别现有的 profile/experiences/projects/education/skills 结构，映射到 v2 工厂，将 `profile.summary` 移入个人总结区块，将旧的 highlights 转为 Markdown 列表文本；无效旧数据返回 `null`。`documentFromStored()` 必须优先使用有效 v2 内容，否则尝试迁移，最后以 API 外层的 title/templateId 覆盖文档对应字段。

**步骤 4：运行测试并确认通过**

命令：`cd frontend && npm test -- src/api/resumes.test.ts`

预期：通过。

**步骤 5：提交**

`git add frontend/src/api/resumes.ts frontend/src/api/resumes.test.ts && git commit -m "feat: migrate legacy resume content"`

### 任务 3：增加安全的共享 Markdown 渲染器

**文件：**

- 修改：`frontend/package.json`
- 修改：`frontend/package-lock.json`
- 新建：`frontend/src/components/ResumeMarkdown.tsx`
- 新建：`frontend/src/components/ResumeMarkdown.test.tsx`

**步骤 1：编写失败测试**

测试粗体、斜体、链接、换行、无序列表和有序列表；测试 HTML 标签作为文本展示，不会被执行或作为原始标记插入。

**步骤 2：运行测试并确认失败**

命令：`cd frontend && npm test -- src/components/ResumeMarkdown.test.tsx`

预期：失败，因为组件尚不存在。

**步骤 3：编写最小实现**

安装 `react-markdown` 和 `remark-gfm`。实现仅使用 GFM 插件的 `ResumeMarkdown`；不要安装或配置 `rehype-raw`。为生成的段落和列表提供稳定的语义类名，供模板 CSS 使用。

**步骤 4：运行测试并确认通过**

命令：`cd frontend && npm test -- src/components/ResumeMarkdown.test.tsx`

预期：通过。

**步骤 5：提交**

`git add frontend/package.json frontend/package-lock.json frontend/src/components/ResumeMarkdown.tsx frontend/src/components/ResumeMarkdown.test.tsx && git commit -m "feat: render safe resume markdown"`

### 任务 4：构建区块编辑器

**文件：**

- 修改：`frontend/src/components/ResumeEditor.tsx`
- 修改：`frontend/src/components/ResumeEditor.test.tsx`
- 修改：`frontend/src/styles/global.css`（仅在必要时；保留用户已有改动）

**步骤 1：编写失败测试**

增加测试，覆盖：添加工作与实习区块、移动区块、切换区块显示状态、编辑工作条目的地点/业绩贡献显示状态、添加包含全部四个可选显示开关的项目、添加校园经历条目与荣誉奖项、更新 Markdown 内容。

**步骤 2：运行测试并确认失败**

命令：`cd frontend && npm test -- src/components/ResumeEditor.test.tsx`

预期：失败，因为旧编辑器只能编辑固定数组。

**步骤 3：编写最小实现**

用可复用的区块卡片控件取代固定表单：按类型添加、编辑标题、上移/下移、显示/隐藏和删除。根据类型分发到不同的内容表单。工作和实习复用 `ExperienceItemsEditor`；新增项目、教育、技能、校园、荣誉和总结表单。基于区块和条目 ID 实现不可变更新；被隐藏的可选字段仍可编辑且会保留，只是不在预览中展示。

**步骤 4：运行测试并确认通过**

命令：`cd frontend && npm test -- src/components/ResumeEditor.test.tsx`

预期：通过。

**步骤 5：提交**

`git add frontend/src/components/ResumeEditor.tsx frontend/src/components/ResumeEditor.test.tsx && git commit -m "feat: edit ordered resume sections"`

### 任务 5：提供共享的语义区块渲染

**文件：**

- 新建：`frontend/src/templates/ResumeSections.tsx`
- 新建：`frontend/src/templates/ResumeSections.test.tsx`

**步骤 1：编写失败测试**

渲染一份故意使用非标准顺序、且包含全部八种区块的文档。断言可见标题和内容严格遵循数组顺序，隐藏区块和隐藏可选字段不出现。

**步骤 2：运行测试并确认失败**

命令：`cd frontend && npm test -- src/templates/ResumeSections.test.tsx`

预期：失败，因为不存在共享的语义渲染器。

**步骤 3：编写最小实现**

将 `ResumeSections` 实现为按顺序处理可辨识联合类型的分支渲染器。它接受由模板拥有的标题、条目、元信息、角色和荣誉奖项容器。个人总结、列表说明和校园经历使用 `ResumeMarkdown`。绝不硬编码区块顺序；只由 `section.visible` 和字段显示开关决定是否渲染内容。

**步骤 4：运行测试并确认通过**

命令：`cd frontend && npm test -- src/templates/ResumeSections.test.tsx`

预期：通过。

**步骤 5：提交**

`git add frontend/src/templates/ResumeSections.tsx frontend/src/templates/ResumeSections.test.tsx && git commit -m "feat: render semantic resume sections"`

### 任务 6：将全部模板迁移至共享渲染器

**文件：**

- 修改：`frontend/src/templates/minimal.tsx`
- 修改：`frontend/src/templates/business.tsx`
- 修改：`frontend/src/templates/creative.tsx`
- 修改：`frontend/src/templates/tech.tsx`
- 修改：`frontend/src/templates/editorial.tsx`
- 修改：`frontend/src/templates/redline.tsx`
- 修改：`frontend/src/templates/violet.tsx`
- 修改：`frontend/src/templates/azure-chronicle.tsx`
- 修改：`frontend/src/templates/registry.test.tsx`
- 修改：`frontend/src/templates/azure-chronicle.test.tsx`
- 修改：`frontend/src/templates/templates.css`（仅在必要时；保留用户已有改动）

**步骤 1：编写失败测试**

扩展模板注册表测试，使用包含全部八种类型且已重新排序的 v2 固定数据。对每个模板断言：每一种可见类型都有代表性条目被渲染、荣誉奖项以紧凑形式展示、首尾标题遵循指定顺序。

**步骤 2：运行测试并确认失败**

命令：`cd frontend && npm test -- src/templates/registry.test.tsx src/templates/azure-chronicle.test.tsx`

预期：失败，因为模板仍在解构已移除的旧数组。

**步骤 3：编写最小实现**

保留每个既有模板的页头和视觉风格。使用按该模板现有类名与布局配置的 `ResumeSections`，替代硬编码的正文区块。仅为 Markdown 与各模板荣誉奖项布局添加必要 CSS；不得修改无关的既有 CSS。

**步骤 4：运行测试并确认通过**

命令：`cd frontend && npm test -- src/templates/registry.test.tsx src/templates/azure-chronicle.test.tsx`

预期：通过。

**步骤 5：提交**

`git add frontend/src/templates && git commit -m "feat: render sections in every resume template"`

### 任务 7：执行回归验证

**文件：**

- 仅修改修复已确认回归问题所必需的文件。

**步骤 1：运行前端测试套件**

命令：`cd frontend && npm test -- --run`

预期：通过。

**步骤 2：运行生产构建**

命令：`cd frontend && npm run build`

预期：通过，且没有 TypeScript 错误。

**步骤 3：人工检查每个模板**

启动前端，打开一份包含所有区块的 v2 示例，重新排序两个区块，隐藏一个区块和一个可选字段，再检查全部八个预览。确认样式会随模板改变，而数据可用性和顺序不受模板影响。

**步骤 4：提交回归修复（如有）**

`git add <verified-files> && git commit -m "fix: verify sectioned resume rendering"`

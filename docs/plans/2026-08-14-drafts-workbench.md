# 草稿工作台实施计划

> **执行方式：** TDD；每项先写失败测试、实现最小改动、运行绿色验证并提交。

## Task 1：工作台结构与空状态

**文件：**
- 修改：`frontend/src/components/ResumeDrafts.tsx`
- 修改：`frontend/src/components/ResumeDrafts.test.tsx`

1. 为空状态的行动面板、引导步骤和草稿工作台语义写失败 RTL 测试。
2. 运行 `npm test -- --run src/components/ResumeDrafts.test.tsx`，确认失败。
3. 用语义化结构实现紧凑空状态；已有草稿使用可读的项目结构与元数据。
4. 重新运行测试并提交。

## Task 2：视觉系统与响应式细节

**文件：**
- 修改：`frontend/src/styles/global.css`
- 必要时修改：`frontend/src/app.test.tsx`

1. 为工作台所需 class 和关键文案写失败断言。
2. 实现颜色、字阶、索引线、卡片、按钮、焦点和窄屏规则；添加 reduced-motion 保护。
3. 运行草稿/App 测试及 `npm run build`。
4. 浏览器视觉检查空状态和至少一个草稿项目。

## Task 3：最终复核

1. 运行 `npm test -- --run`、`npm run build` 与 `git diff --check`。
2. 审查草稿加载、错误重试、选择和创建行为无回归。

# 区块化简历设计说明

## 目标

将现有固定的简历数组重构为稳定的区块化文档格式。简历的内容与展示顺序属于数据本身；模板只负责这些内容的视觉呈现。

## 产品原则

- 数据库以区块化 JSON 作为简历内容的唯一标准格式。
- 每个模板都支持所有区块类型，并按文档定义的顺序渲染可见区块。
- 模板可以自行决定字体、间距、背景、分栏和区块的紧凑排法（尤其是荣誉奖项），但不得删减文档模型提供的数据能力。
- 区块可排序、可隐藏；需求中明确标记为可选的字段具有独立显示开关。
- 已保存的旧草稿通过一次性的内存迁移继续可读；下一次保存后写回新的标准区块格式。

## 标准数据结构

`ResumeDocument` 仍是保存在 `content` 中的前端文档表示。API 外层和数据库列仍独立保存 `title`、`templateId`，以便列表展示；`content` 内部也保留两者，使文档本身完整、自包含。

```ts
type ResumeDocument = {
  schemaVersion: 2;
  title: string;
  templateId: TemplateId;
  profile: ResumeProfile;
  sections: ResumeSection[];
};

type ResumeSection = {
  id: string;
  type: ResumeSectionType;
  title: string;
  visible: boolean;
  content: SectionContent;
};
```

`ResumeSectionType` 是封闭的可辨识联合类型：`summary`、`work`、`internship`、`project`、`education`、`skills`、`campus`、`honours`。`title` 默认使用对应的中文小标题，也保存在文档中，允许用户定制显示标题，同时不影响区块的语义类型。

各区块的内容如下：

| 类型 | 内容 |
| --- | --- |
| `summary` | `{ markdown: string }` |
| `work`、`internship` | `{ items: ExperienceItem[] }`：公司、角色、时间范围、地点、业绩贡献和 Markdown 列表说明。`showLocation`、`showAchievements` 控制可选字段。 |
| `project` | `{ items: ProjectItem[] }`：项目名称、角色、时间范围、地点、链接、项目介绍、项目成绩和 Markdown 列表说明。`showLocation`、`showLink`、`showIntroduction`、`showAchievements` 控制可选字段。 |
| `education` | `{ items: EducationItem[] }`：院校、时间范围、学历层次、学院和专业。 |
| `skills` | `{ items: SkillItem[] }`：技能名称和备注。 |
| `campus` | `{ items: CampusItem[] }`：部门或活动名称、担任角色和 Markdown 列表说明。 |
| `honours` | `{ items: HonourItem[] }`：仅存放简短的奖项名称。 |

日期持续以展示字符串保存，让用户可直接填写 `2025.08`、`至今` 或其他本地化格式，无需额外格式转换。

## 渲染架构

新增一个共享的语义区块渲染器和一个安全的轻量 Markdown 渲染器。每个模板提供自身的展示组件（区块标题、条目容器、元信息位置、荣誉奖项布局），再调用共享渲染器输出区块内容。渲染器会先过滤 `visible === true` 的区块，再严格按 `sections` 数组顺序遍历；绝不依赖固定的小标题顺序。

Markdown 只支持段落/换行、斜体、粗体、链接、无序列表和有序列表；不渲染原始 HTML、图片、表格或任意嵌入内容。列表顺序等于用户文本中的原始顺序；切换有序/无序列表属于文本编辑，不引入额外的列表排序模型。

## 编辑与迁移

编辑器改为区块卡片列表。用户可添加任意受支持的区块、上移/下移、显示/隐藏区块、编辑小标题或删除区块。不同类型使用对应的条目编辑器；工作经历和实习经历复用同一套经历条目编辑器。明确可选的字段提供独立显示开关。

`documentFromStored` 优先校验 v2 文档；若存储对象是有效的旧格式，则将 `profile.summary`、`experiences`、`projects`、`education`、`skills` 转换成默认的 v2 区块，并保留原有 ID 与顺序。格式错误的内容会安全失败，不能凭猜测修复。

## 测试策略

- 单元测试 v2 文档创建及旧格式迁移。
- 单元测试所有区块变体的已保存内容校验。
- 覆盖编辑器中的区块添加、排序、隐藏、可选字段显示以及全部条目类型。
- 测试安全 Markdown 输出和支持的列表语法。
- 用含所有区块的文档渲染全部已注册模板；断言隐藏区块不出现、可见区块遵循指定顺序。

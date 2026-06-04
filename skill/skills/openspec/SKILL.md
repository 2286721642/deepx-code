---
name: openspec
description: 规格驱动开发(SDD)。实现任何功能或改动时使用——写代码前先就规格达成一致:在 openspec/changes/<id>/ 写 proposal、spec deltas、tasks(复杂时加 design),对齐后再严格按规格实现,完成后把 delta 归档进基线 specs/。
license: MIT
source: https://github.com/Fission-AI/OpenSpec
---

# OpenSpec — 规格驱动开发(SDD)

提炼自官方 OpenSpec(https://github.com/Fission-AI/OpenSpec)的约定、文件格式与流程。
核心:**"Agree before you build"** —— 改动的行为规格写清楚、与用户对齐后,再写实现代码。

## 理念(不是僵硬的瀑布)

- **fluid not rigid**:不设阶段闸门,可按合理顺序产出 artifacts。
- **iterative not waterfall**:边做边学,规格可迭代细化。
- **easy not complex**:轻量、少仪式。
- **brownfield-first**:多数是改既有系统,所以用 **delta(增/改/删)** 描述"对现有行为的改动",而非从零描述整个系统。

## 目录两区

```
openspec/
  specs/                          # 真相基线:系统【当前】如何工作(按 capability 分)
    <capability>/spec.md
  changes/                        # 提案:对基线的【改动】,每个改动一个文件夹
    <change-id>/
      proposal.md                 # 为什么 + 改什么
      design.md                   # 可选,仅复杂改动写(技术方案/取舍)
      tasks.md                    # 实现清单
      specs/<capability>/spec.md  # 该改动对规格的 DELTA(ADDED/MODIFIED/REMOVED)
```

`specs/` 是源头真相;`changes/<id>/` 是尚未合并的改动。归档时 change 的 delta 合并进 `specs/`。

## 文件格式(照官方,别自由发挥)

### proposal.md
```
## Why
<动机:为什么要这个改动>

## What Changes
### 1. <子改动>
<说明>
### 2. <子改动>
...
```

### tasks.md
```
## 1. <分组>
- [ ] 1.1 <小而可验证的任务>
- [ ] 1.2 ...
## 2. <分组>
- [ ] 2.1 ...
```

### specs/<capability>/spec.md —— **关键:delta 格式**
按 `ADDED` / `MODIFIED` / `REMOVED` / `RENAMED` 标 delta;每条 Requirement 用 **SHALL**,且**至少含一个 Scenario**:
```
## ADDED Requirements
### Requirement: <名称>
<The system SHALL ...>

#### Scenario: <场景名>
- **WHEN** <条件>
- **THEN** <预期>
- **AND** <附加预期>

## MODIFIED Requirements
### Requirement: <已存在的需求名>
<改后的 SHALL 描述 + 场景>

## REMOVED Requirements
### Requirement: <要删除的需求名>
```
**绝不**用 `README.md` 或自由 bullet 代替——必须是 `spec.md`,且用上面的 `Requirement` + `Scenario`(WHEN/THEN)结构。

### design.md(可选)
仅当改动复杂、需要记录架构 / 关键决策 / 取舍时才写;简单改动跳过。

## 工作流

1. **Propose**:在 `openspec/changes/<change-id>/` 建文件夹,写 `proposal.md` +(需要时)`design.md` + `tasks.md` + `specs/<capability>/spec.md`(delta)。分成"短到能读完"的小块给用户看,迭代到对齐。
2. **Implement**:对齐后按 `tasks.md` 逐项实现,严格遵循 spec delta;每完成勾掉。若现实迫使偏离规格,**先停下更新规格、重新对齐**,别默默跑偏。
3. **Archive**:改动完成并验证后,把它的 spec delta **应用(apply)**进 `openspec/specs/` 基线,并把 change 文件夹移入归档(如 `openspec/changes/archive/`)。
   - **应用语义**:`ADDED` → 把该 Requirement 加入基线;`MODIFIED` → 用新内容替换基线里同名 Requirement;`REMOVED` → 从基线删除该 Requirement。
   - **关键**:`## ADDED/MODIFIED/REMOVED` 是**只属于 change 里那份 delta** 的标记。基线 `specs/<capability>/spec.md` 是**当前真相**,只保留**当前的 Requirements / Scenario**,**不要带这些 delta 标记**(用 `## Requirements` 或直接列 `### Requirement:` 即可)。换句话说:归档是"把 delta 应用进基线",不是"把 delta 整份拷进基线"。

## 铁律

- 改动的**行为规格(Requirement/Scenario)写出来、与用户对齐之前,不要写实现代码**。被要求"直接写"也先出 proposal + spec。
- specs 用 `spec.md` + 官方 `Requirement`/`Scenario` 格式,**不是 README、不是自由条目**。
- delta 思维:改既有系统时,specs 写的是"对现有行为的增/改/删",不是从零罗列。

> 说明:deepx 无内置 `openspec` CLI;本 skill 把官方 OpenSpec 的目录结构、文件格式与流程落到普通文件操作上(在仓库里建 `openspec/` 目录与上述文件)。需要原生 CLI 体验(`openspec init/propose/archive`、校验)可另装官方工具。
